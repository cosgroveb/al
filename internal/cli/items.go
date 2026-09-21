package cli

import (
	"context"

	"github.com/cosgroveb/al/internal/anylist"
	"github.com/spf13/cobra"
)

type listFlags struct {
	name string
	id   string
}

func (f *listFlags) bind(command *cobra.Command) {
	command.Flags().StringVar(&f.name, "list", "", "Select a list by exact name")
	command.Flags().StringVar(&f.id, "list-id", "", "Select a list by ID")
}

func (f *listFlags) validate(command *cobra.Command) error {
	if command.Flags().Changed("list") && command.Flags().Changed("list-id") {
		return inputError("--list and --list-id are mutually exclusive")
	}
	if command.Flags().Changed("list") && f.name == "" {
		return inputError("list name must not be empty")
	}
	if command.Flags().Changed("list-id") && f.id == "" {
		return inputError("list ID must not be empty")
	}
	return nil
}

func itemState(ctx context.Context, a *app, flags listFlags) (*anylist.Client, anylist.ListState, error) {
	if err := a.loadConfig(); err != nil {
		return nil, anylist.ListState{}, err
	}
	if flags.name == "" && flags.id == "" && a.config.DefaultListID == "" {
		return nil, anylist.ListState{}, &anylist.Error{Code: "configuration", Message: "select a list with --list or --list-id, or run al config set default-list"}
	}
	client, lists, err := a.connect(ctx)
	if err != nil {
		return nil, anylist.ListState{}, err
	}
	list, err := selectList(lists, flags.name, flags.id, a.config.DefaultListID)
	if err != nil {
		return nil, anylist.ListState{}, err
	}
	state, err := client.ListState(list.ID)
	return client, state, err
}

func registerItemCommands(root *cobra.Command, a *app) {
	for _, operation := range []string{"add", "edit", "check", "uncheck", "remove"} {
		root.AddCommand(itemCommand(a, operation))
	}
	var lists listFlags
	var all, checked bool
	items := &cobra.Command{Use: "items", Short: "Show unchecked items in a list", Args: cobra.NoArgs, Example: "  al items --list Groceries\n  al items --list-id LIST_ID --all", RunE: func(command *cobra.Command, args []string) error {
		if err := lists.validate(command); err != nil {
			return err
		}
		if command.Flags().Changed("all") && command.Flags().Changed("checked") {
			return inputError("--all and --checked are mutually exclusive")
		}
		_, state, err := itemState(command.Context(), a, lists)
		if err != nil {
			return err
		}
		visible := []anylist.Item{}
		for _, item := range state.Items {
			if all || item.Checked == checked {
				visible = append(visible, item)
			}
		}
		a.data = map[string]any{"list": state.List, "items": visible}
		return nil
	}}
	lists.bind(items)
	items.Flags().BoolVar(&all, "all", false, "Show checked and unchecked items")
	items.Flags().BoolVar(&checked, "checked", false, "Show checked items only")
	root.AddCommand(items)
	var categoryList listFlags
	categories := &cobra.Command{Use: "categories", Short: "Show categories in the list's active category group", Args: cobra.NoArgs, Example: "  al categories --list-id LIST_ID", RunE: func(command *cobra.Command, args []string) error {
		if err := categoryList.validate(command); err != nil {
			return err
		}
		_, state, err := itemState(command.Context(), a, categoryList)
		if err != nil {
			return err
		}
		a.data = map[string]any{"list": state.List, "categories": state.Categories}
		return nil
	}}
	categoryList.bind(categories)
	root.AddCommand(categories)
}

func itemCommand(a *app, operation string) *cobra.Command {
	var list listFlags
	var id, newName, quantity, notes, category, categoryID string
	var stdin, newItem bool
	descriptions := map[string]string{"add": "Add an item or uncheck an exact existing match", "edit": "Replace selected item fields", "check": "Check an item", "uncheck": "Uncheck an item", "remove": "Remove an item"}
	command := &cobra.Command{Use: operation + " [NAME]", Short: descriptions[operation], Args: cobra.MaximumNArgs(1)}
	command.Example = "  al " + operation + " --list Groceries --id ITEM_ID\n  al " + operation + " --list Groceries -- '-item'\n  printf '[{\"name\":\"milk\"}]' | al " + operation + " --list Groceries --stdin --json"
	if operation == "edit" {
		command.Example = "  al edit --list Groceries --id ITEM_ID --name 'whole milk'\n  al edit milk --list Groceries --quantity '' --notes '' --category ''\n  printf '[{\"id\":\"ITEM_ID\",\"notes\":\"\"}]' | al edit --list Groceries --stdin --json"
	}
	command.Flags().StringVar(&id, "id", "", "Select an existing item by ID")
	command.Flags().BoolVar(&stdin, "stdin", false, "Read one JSON array of records from stdin")
	list.bind(command)
	if operation == "add" || operation == "edit" {
		command.Flags().StringVar(&quantity, "quantity", "", "Replace quantity text; empty clears it")
		command.Flags().StringVar(&notes, "notes", "", "Replace notes; empty clears them")
		command.Flags().StringVar(&category, "category", "", "Select active-group category by name; empty clears it")
		command.Flags().StringVar(&categoryID, "category-id", "", "Select active-group category by ID")
	}
	if operation == "add" {
		command.Flags().BoolVar(&newItem, "new", false, "Create an intentional duplicate instead of reusing a name")
	}
	if operation == "edit" {
		command.Flags().StringVar(&newName, "name", "", "Replace the item's name")
	}
	command.RunE = func(command *cobra.Command, args []string) error {
		if err := list.validate(command); err != nil {
			return err
		}
		var records []itemInput
		if stdin {
			if len(args) > 0 {
				return inputError("--stdin excludes positional item targets")
			}
			for _, flag := range []string{"id", "name", "quantity", "notes", "category", "category-id", "new"} {
				if command.Flags().Changed(flag) {
					return inputError("--stdin excludes per-item flags")
				}
			}
			var err error
			records, err = readBatchContext(command.Context(), a.in, operation)
			if err != nil {
				return err
			}
		} else {
			record := itemInput{New: newItem, newSet: command.Flags().Changed("new")}
			if len(args) == 1 {
				record.Name = &args[0]
			}
			if command.Flags().Changed("id") {
				record.ID = &id
			}
			if command.Flags().Changed("name") {
				record.NewName = &newName
			}
			if command.Flags().Changed("quantity") {
				record.Quantity = &quantity
			}
			if command.Flags().Changed("notes") {
				record.Notes = &notes
			}
			if command.Flags().Changed("category") {
				record.Category = &category
			}
			if command.Flags().Changed("category-id") {
				record.CategoryID = &categoryID
			}
			if err := validateItemInput(record, operation); err != nil {
				return err
			}
			records = []itemInput{record}
		}
		if len(records) == 0 {
			a.data = map[string]any{"results": []anylist.MutationResult{}}
			return nil
		}
		client, state, err := itemState(command.Context(), a, list)
		if err != nil {
			return err
		}
		plans, failed, err := planBatch(state, records, operation)
		if err != nil {
			a.data = map[string]any{"list": state.List, "results": preflightResults(records, plans, failed)}
			return err
		}
		results, err := executeBatch(command.Context(), client, state.ID, plans)
		a.data = map[string]any{"list": state.List, "results": results}
		return err
	}
	return command
}
