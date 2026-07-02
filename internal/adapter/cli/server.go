package cli

import (
	"github.com/spf13/cobra"
)

func newServerCommand(deps Dependencies) *cobra.Command {
	var listenAddr, stunAddr string

	cmd := &cobra.Command{
		Use:   "server",
		Short: "Запустить rendezvous/signaling-сервер (control plane + STUN + relay fallback)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Printf("control-plane слушает на %s, STUN — на %s\n", listenAddr, stunAddr)
			return deps.RunServer(cmd.Context(), listenAddr, stunAddr)
		},
	}

	cmd.Flags().StringVar(&listenAddr, "listen", ":4443", "адрес control-канала (QUIC)")
	cmd.Flags().StringVar(&stunAddr, "stun-listen", ":4444", "адрес STUN-эндпоинта (UDP)")

	return cmd
}
