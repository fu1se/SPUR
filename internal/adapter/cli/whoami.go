package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newWhoamiCommand(deps ClientDependencies, defaults ClientDefaults) *cobra.Command {
	var identityPath = defaults.Identity

	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "Показать свой peer-id (без обращения к сети)",
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

	cmd.Flags().StringVar(&identityPath, "identity", identityPath, "путь к файлу идентичности (по умолчанию — в конфиг-директории пользователя)")

	return cmd
}
