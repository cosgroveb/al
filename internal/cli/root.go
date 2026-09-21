package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/cosgroveb/al/internal/anylist"
	"github.com/spf13/cobra"
)

var Version = "dev"

type options struct {
	configPath string
	anylist    anylist.Options
	getenv     func(string) string
}

type app struct {
	in           io.Reader
	out          io.Writer
	errOut       io.Writer
	opts         options
	json         bool
	data         any
	config       Config
	configLoaded bool
}

// Run executes one command and returns its process exit status.
func Run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) int {
	return newApp(in, out, errOut, options{}).run(ctx, args)
}

func newApp(in io.Reader, out, errOut io.Writer, opts options) *app {
	if opts.getenv == nil {
		opts.getenv = os.Getenv
	}
	return &app{in: in, out: out, errOut: errOut, opts: opts}
}

func (a *app) run(ctx context.Context, args []string) int {
	root := a.command()
	var commandOutput bytes.Buffer
	root.SetIn(a.in)
	root.SetOut(&commandOutput)
	root.SetErr(&commandOutput)
	root.SetArgs(args)
	command, err := root.ExecuteContextC(ctx)
	if err != nil {
		a.json = requestedJSON(command, args)
	}
	var diagnostic *anylist.Error
	if err != nil && !errors.As(err, &diagnostic) {
		diagnostic = inputError(err.Error())
	}
	if err == nil && a.data == nil && commandOutput.Len() > 0 {
		a.data = map[string]any{"text": commandOutput.String()}
	}
	if err := a.render(diagnostic); err != nil {
		_, _ = fmt.Fprintln(a.errOut, "cannot write command output")
		return 1
	}
	if diagnostic == nil {
		return 0
	}
	if diagnostic.Code == "input" {
		return 2
	}
	return 1
}

func (a *app) command() *cobra.Command {
	root := &cobra.Command{
		Use:   "al",
		Short: "Manage AnyList lists and items",
		Long: `Manage AnyList lists and items.

Before commands that contact AnyList, export your credentials:
  export ANYLIST_EMAIL="you@example.com"
  export ANYLIST_PASSWORD="your-password"

Run al auth status to verify these credentials.
Passwords and tokens stay in process memory.`,
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE:          func(cmd *cobra.Command, args []string) error { return cmd.Help() },
		Example: `  al auth status
  al lists
  al items --list "Groceries"
  al add milk --list "Groceries" --quantity "2"
  al check --list-id LIST_ID --id ITEM_ID
  al add --list "Groceries" -- --special
  printf '[{"name":"milk"},{"name":"eggs"}]' | al add --stdin --list "Groceries" --json`,
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.PersistentFlags().BoolVar(&a.json, "json", false, "Print one JSON result, including errors")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return inputError(err.Error()) })
	registerAuthCommands(root, a)
	registerListCommands(root, a)
	registerConfigCommands(root, a)
	registerItemCommands(root, a)
	return root
}

func (a *app) newClient() (*anylist.Client, error) {
	if err := a.loadConfig(); err != nil {
		return nil, err
	}
	opts := a.opts.anylist
	opts.Email = a.opts.getenv("ANYLIST_EMAIL")
	opts.Password = a.opts.getenv("ANYLIST_PASSWORD")
	if opts.Email == "" || opts.Password == "" {
		return nil, configError("set ANYLIST_EMAIL and ANYLIST_PASSWORD for commands that use AnyList")
	}
	if a.config.ClientID == "" {
		a.config.ClientID = anylist.NewID()
		if err := a.saveConfig(); err != nil {
			return nil, err
		}
	}
	opts.ClientID = a.config.ClientID
	return anylist.NewClient(opts), nil
}

func (a *app) connect(ctx context.Context) (*anylist.Client, []anylist.List, error) {
	client, err := a.newClient()
	if err != nil {
		return nil, nil, err
	}
	lists, err := client.Lists(ctx)
	return client, lists, err
}

func inputError(message string) *anylist.Error {
	return &anylist.Error{Code: "input", Message: message}
}

// Detect output selection even when Cobra stops at an earlier parsing error.
func requestedJSON(command *cobra.Command, args []string) bool {
	jsonOutput := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		if arg == "--json" {
			jsonOutput = true
			continue
		}
		if value, ok := strings.CutPrefix(arg, "--json="); ok {
			parsed, err := strconv.ParseBool(value)
			jsonOutput = err != nil || parsed
			continue
		}
		if name, ok := strings.CutPrefix(arg, "--"); ok {
			flag := command.Flags().Lookup(name)
			if flag != nil && flag.NoOptDefVal == "" {
				i++
			}
		}
	}
	return jsonOutput
}
