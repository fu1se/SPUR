package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func newJoinCommand() *cobra.Command {
	var (
		serverAddr string
		network    string
	)

	cmd := &cobra.Command{
		Use:   "join",
		Short: "Присоединиться к mesh-сети (полноценный доступ в локальную сеть через TUN)",
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = serverAddr
			_ = network
			return errors.New("join: не реализовано (Фаза 6)")
		},
	}

	cmd.Flags().StringVar(&serverAddr, "server", "", "адрес rendezvous/coordination-сервера")
	cmd.Flags().StringVar(&network, "network", "", "имя mesh-сети")

	return cmd
}
