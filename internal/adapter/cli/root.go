// Package cli translates command-line invocations into use case calls.
// It is the outermost adapter for the interactive entrypoint of the
// application; it must not contain business logic itself.
package cli

import (
	"github.com/spf13/cobra"
)

// version is set at build time via -ldflags "-X ... version=...".
var version = "dev"

// NewRootCommand builds the root cobra command with all subcommands wired.
// This is the composition point for the CLI adapter: as use cases and their
// port implementations are introduced, they get constructed in cmd/app and
// passed into the subcommand constructors below.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "app",
		Short:         "localizator — прямое подключение в локальную сеть в обход NAT",
		SilenceUsage:  true,
		SilenceErrors: false,
	}

	root.AddCommand(
		newVersionCommand(),
		newServerCommand(),
		newConnectCommand(),
		newJoinCommand(),
	)

	return root
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Показать версию",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println(version)
			return nil
		},
	}
}
