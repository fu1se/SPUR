// Package cli translates command-line invocations into use case calls.
// It is the outermost adapter for the interactive entrypoint of the
// application; it must not contain business logic itself, and it must not
// construct concrete adapter/infra implementations directly — those are
// wired in cmd/app (the composition root) and handed to this package as
// plain functions via Dependencies.
package cli

import (
	"context"

	"github.com/spf13/cobra"
)

// version is set at build time via -ldflags "-X ... version=...".
var version = "dev"

// RegisterResult is what a successful control-plane registration reports
// back to the CLI layer.
type RegisterResult struct {
	PeerID          string
	ObservedAddress string
}

// Dependencies holds the wired entrypoints each subcommand calls into.
// Every field is populated in cmd/app; commands never know what concrete
// adapters sit behind them.
type Dependencies struct {
	// RunServer starts the rendezvous/control-plane server and blocks
	// until ctx is cancelled.
	RunServer func(ctx context.Context, listenAddr string) error

	// Register dials a control-plane server and registers an (ephemeral,
	// until Phase 7) identity with it.
	Register func(ctx context.Context, serverAddr string) (RegisterResult, error)
}

// NewRootCommand builds the root cobra command with all subcommands wired
// against deps.
func NewRootCommand(deps Dependencies) *cobra.Command {
	root := &cobra.Command{
		Use:           "app",
		Short:         "localizator — прямое подключение в локальную сеть в обход NAT",
		SilenceUsage:  true,
		SilenceErrors: false,
	}

	root.AddCommand(
		newVersionCommand(),
		newServerCommand(deps),
		newRegisterCommand(deps),
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
