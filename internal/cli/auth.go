package cli

import "github.com/spf13/cobra"

func registerAuthCommands(root *cobra.Command, a *app) {
	auth := &cobra.Command{Use: "auth", Short: "Check AnyList authentication", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() }}
	status := &cobra.Command{
		Use: "status", Short: "Verify ANYLIST_EMAIL and ANYLIST_PASSWORD with AnyList", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := a.newClient()
			if err != nil {
				return err
			}
			if err := client.Authenticate(cmd.Context()); err != nil {
				return err
			}
			a.data = map[string]any{"authenticated": true}
			return nil
		},
	}
	auth.AddCommand(status)
	root.AddCommand(auth)
}
