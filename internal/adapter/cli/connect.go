package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func newConnectCommand() *cobra.Command {
	var (
		serverAddr string
		peerID     string
		localPort  int
		remotePort int
	)

	cmd := &cobra.Command{
		Use:   "connect",
		Short: "Пробросить один порт с удалённого пира (port-forward режим)",
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = serverAddr
			_ = peerID
			_ = localPort
			_ = remotePort
			return errors.New("connect: не реализовано (Фаза 5)")
		},
	}

	cmd.Flags().StringVar(&serverAddr, "server", "", "адрес rendezvous-сервера")
	cmd.Flags().StringVar(&peerID, "to", "", "идентификатор удалённого пира")
	cmd.Flags().IntVar(&localPort, "local-port", 0, "локальный порт для прослушивания")
	cmd.Flags().IntVar(&remotePort, "remote-port", 0, "порт сервиса на удалённом пире")

	return cmd
}
