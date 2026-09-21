package cli

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/cosgroveb/al/internal/anylist"
	"github.com/spf13/cobra"
)

type Config struct {
	ClientID      string `json:"clientId"`
	DefaultListID string `json:"defaultListId"`
}

func (a *app) loadConfig() error {
	if a.configLoaded {
		return nil
	}
	if a.opts.configPath == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return configError("cannot locate the user configuration directory")
		}
		a.opts.configPath = filepath.Join(dir, "al", "config.json")
	}
	file, err := os.Open(a.opts.configPath)
	if errors.Is(err, os.ErrNotExist) {
		a.configLoaded = true
		return nil
	}
	if err != nil {
		return configError("cannot read configuration")
	}
	defer func() { _ = file.Close() }()
	var config *Config
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil || config == nil {
		return configError("invalid configuration JSON")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return configError("configuration must contain one JSON object")
	}
	a.config = *config
	a.configLoaded = true
	return nil
}

func (a *app) saveConfig() error {
	if err := a.loadConfig(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(a.config, "", "  ")
	if err != nil {
		return configError("cannot encode configuration")
	}
	data = append(data, '\n')
	dir := filepath.Dir(a.opts.configPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return configError("cannot create the configuration directory")
	}
	file, err := os.CreateTemp(dir, ".config-*")
	if err != nil {
		return configError("cannot create a configuration file")
	}
	defer func() { _ = os.Remove(file.Name()) }()
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return configError("cannot write configuration")
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return configError("cannot save configuration")
	}
	if err := file.Close(); err != nil {
		return configError("cannot close configuration")
	}
	if err := os.Rename(file.Name(), a.opts.configPath); err != nil {
		return configError("cannot replace configuration")
	}
	return nil
}

func configError(message string) *anylist.Error {
	return &anylist.Error{Code: "configuration", Message: message}
}

func registerConfigCommands(root *cobra.Command, a *app) {
	config := &cobra.Command{Use: "config", Short: "Show or change local configuration", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() }}
	show := &cobra.Command{
		Use: "show", Short: "Show non-secret configuration without contacting AnyList", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.loadConfig(); err != nil {
				return err
			}
			a.data = map[string]any{"config": a.config, "path": a.opts.configPath}
			return nil
		},
	}
	set := &cobra.Command{Use: "set", Short: "Change a configuration value", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() }}
	var listID string
	defaultList := &cobra.Command{
		Use: "default-list [NAME]", Short: "Save a list ID for item commands", Args: cobra.MaximumNArgs(1),
		Example: "  al config set default-list Groceries\n  al config set default-list --list-id LIST_ID",
		RunE: func(cmd *cobra.Command, args []string) error {
			byID := cmd.Flags().Changed("list-id")
			if byID && (listID == "" || len(args) != 0) || !byID && (len(args) != 1 || args[0] == "") {
				return inputError("provide one list name or --list-id ID")
			}
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			_, lists, err := a.connect(cmd.Context())
			if err != nil {
				return err
			}
			list, err := selectList(lists, name, listID, "")
			if err != nil {
				return err
			}
			a.config.DefaultListID = list.ID
			if err := a.saveConfig(); err != nil {
				return err
			}
			a.data = map[string]any{"config": a.config, "path": a.opts.configPath, "list": list}
			return nil
		},
	}
	defaultList.Flags().StringVar(&listID, "list-id", "", "Select a list by ID")
	set.AddCommand(defaultList)
	config.AddCommand(show, set)
	root.AddCommand(config)
}
