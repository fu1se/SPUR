package cli

import (
	"github.com/spf13/cobra"
)

func newServerCommand(deps Dependencies, defaults Defaults) *cobra.Command {
	var listenAddr, stunAddr, dbPath string

	cmd := &cobra.Command{
		Use:   "server",
		Short: "Запустить rendezvous/signaling-сервер (control plane + STUN + relay fallback)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Printf("control-plane слушает на %s, STUN — на %s, состояние — в %s\n", listenAddr, stunAddr, dbPath)
			return deps.RunServer(cmd.Context(), listenAddr, stunAddr, dbPath)
		},
	}

	cmd.Flags().StringVar(&listenAddr, "listen", ":4443", "адрес control-канала (QUIC)")
	cmd.Flags().StringVar(&stunAddr, "stun-listen", ":4444", "адрес STUN-эндпоинта (UDP)")
	cmd.Flags().StringVar(&dbPath, "db", defaults.ServerState, "путь к файлу состояния сервера (SQLite)")

	return cmd
}
