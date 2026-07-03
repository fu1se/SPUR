package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newWhoamiCommand(deps ClientDependencies, defaults ClientDefaults) *cobra.Command {
	var identityPath = defaults.Identity

	cmd := &cobra.Command{
		Use:   "whoami",
		Short: msg().WhoamiShort,
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := deps.Whoami(identityPath)
			if err != nil {
				return err
			}
			// Deliberately fmt.Fprintln(cmd.OutOrStdout(), ...), not
			// cmd.Println: cobra's Print/Println/Printf helpers default to
			// stderr (OutOrStderr), which silently breaks `id=$(app
			// whoami)` — whoami's entire purpose is being scriptable.
			fmt.Fprintln(cmd.OutOrStdout(), id)
			return nil
		},
	}

	cmd.Flags().StringVar(&identityPath, "identity", identityPath, msg().FlagIdentity)

	return cmd
}
