package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func newConnectCommand(deps Dependencies, defaults Defaults) *cobra.Command {
	var (
		serverAddr   = defaults.Server
		stunAddr     = defaults.StunServer
		peerID       string
		identityPath = defaults.Identity
		localPort    int
	)

	cmd := &cobra.Command{
		Use:   "connect",
		Short: "Пробросить локальный порт на сервис, открытый пиром через `app expose`",
		RunE: func(cmd *cobra.Command, args []string) error {
			if serverAddr == "" || stunAddr == "" || peerID == "" || localPort == 0 {
				return errors.New("connect: укажите --server, --stun-server, --to и --local-port")
			}
			return deps.Connect(cmd.Context(), serverAddr, stunAddr, peerID, identityPath, localPort, func(selfID string) {
				cmd.Printf("свой peer-id: %s\n", selfID)
			})
		},
	}

	cmd.Flags().StringVar(&serverAddr, "server", serverAddr, "адрес rendezvous-сервера (control-канал)")
	cmd.Flags().StringVar(&stunAddr, "stun-server", stunAddr, "адрес STUN-эндпоинта сервера")
	cmd.Flags().StringVar(&peerID, "to", "", "идентификатор пира, чей сервис пробрасываем")
	cmd.Flags().StringVar(&identityPath, "identity", identityPath, "путь к файлу идентичности (по умолчанию — в конфиг-директории пользователя)")
	cmd.Flags().IntVar(&localPort, "local-port", 0, "локальный порт для прослушивания")

	return cmd
}
