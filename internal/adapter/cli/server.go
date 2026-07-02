package cli

import (
	"github.com/spf13/cobra"
)

func newServerCommand(deps Dependencies) *cobra.Command {
	var listenAddr string

	cmd := &cobra.Command{
		Use:   "server",
		Short: "Запустить rendezvous/signaling-сервер (control plane + relay fallback)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Printf("control-plane слушает на %s\n", listenAddr)
			return deps.RunServer(cmd.Context(), listenAddr)
		},
	}

	cmd.Flags().StringVar(&listenAddr, "listen", ":4443", "адрес control-канала (QUIC)")

	return cmd
}
