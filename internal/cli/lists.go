package cli

import (
	"net/mail"

	"github.com/cosgroveb/al/internal/anylist"
	"github.com/spf13/cobra"
)

func registerListCommands(root *cobra.Command, a *app) {
	lists := &cobra.Command{
		Use: "lists", Aliases: []string{"ls"}, Short: "Show account-visible lists", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, lists, err := a.connect(cmd.Context())
			if err != nil {
				return err
			}
			a.data = map[string]any{"lists": lists}
			return nil
		},
	}
	list := &cobra.Command{Use: "list", Short: "Create, rename, delete, or share a list", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() }}
	create := &cobra.Command{
		Use: "create NAME", Short: "Create a generic list with an Other category", Args: cobra.ExactArgs(1),
		Example: "  al list create \"Weekend errands\"\n  al list create -- --special",
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] == "" {
				return inputError("list name must not be empty")
			}
			client, _, err := a.connect(cmd.Context())
			if err != nil {
				return err
			}
			result, err := client.CreateList(cmd.Context(), args[0])
			a.listResult(result)
			return err
		},
	}
	list.AddCommand(create)
	for _, operation := range []string{"rename", "delete", "share"} {
		list.AddCommand(a.listCommand(operation))
	}
	root.AddCommand(lists, list)
}

func (a *app) listCommand(operation string) *cobra.Command {
	var id string
	use := operation + " [NAME]"
	short := "Remove a list from this account and clean up its settings"
	switch operation {
	case "rename":
		use += " NEW_NAME"
		short = "Rename an explicit list"
	case "share":
		use += " EMAIL"
		short = "Request list sharing with an email address"
	}
	cmd := &cobra.Command{
		Use: use, Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			byID := cmd.Flags().Changed("id")
			count := 1
			if operation != "delete" {
				count++
			}
			if byID {
				count--
			}
			if len(args) != count || byID && id == "" {
				return inputError("use " + cmd.UseLine() + "; choose one list name or --id ID")
			}
			name := ""
			if !byID {
				name = args[0]
				if name == "" {
					return inputError("list name must not be empty")
				}
			}
			value := ""
			if operation != "delete" {
				value = args[len(args)-1]
				if value == "" {
					return inputError("provide a non-empty " + map[string]string{"rename": "new list name", "share": "email address"}[operation])
				}
			}
			if operation == "share" {
				address, err := mail.ParseAddress(value)
				if err != nil || address.Address != value {
					return inputError("provide an email address without a display name")
				}
			}
			client, lists, err := a.connect(cmd.Context())
			if err != nil {
				return err
			}
			target := anylist.List{ID: id}
			// An explicit delete can finish settings cleanup for an invisible list.
			if operation != "delete" || !byID {
				target, err = selectList(lists, name, id, "")
				if err != nil {
					if selection, ok := err.(*anylist.Error); ok && selection.Code == "ambiguous" {
						return ambiguous("list", "--id", selection.Candidates)
					}
					return err
				}
			}
			var result anylist.MutationResult
			switch operation {
			case "rename":
				result, err = client.RenameList(cmd.Context(), target.ID, value)
			case "delete":
				result, err = client.DeleteList(cmd.Context(), target.ID)
			case "share":
				result, err = client.ShareList(cmd.Context(), target.ID, value)
			}
			a.listResult(result)
			return err
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "Select the list by ID instead of NAME")
	switch operation {
	case "rename":
		cmd.Example = "  al list rename Groceries Shopping\n  al list rename --id LIST_ID Shopping"
	case "delete":
		cmd.Example = "  al list delete Groceries\n  al list delete --id LIST_ID"
	case "share":
		cmd.Example = "  al list share Groceries person@example.com\n  al list share --id LIST_ID person@example.com"
	}
	return cmd
}

func (a *app) listResult(result anylist.MutationResult) {
	if result.ID != "" {
		a.data = map[string]any{"results": []anylist.MutationResult{result}}
	}
}
