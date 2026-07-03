package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func newConnectCommand(deps ClientDependencies, defaults ClientDefaults) *cobra.Command {
	var (
		serverAddr   = defaults.Server
		stunAddr     = defaults.StunServer
		peerID       string
		roomName     string
		identityPath = defaults.Identity
		localPort    int
	)

	cmd := &cobra.Command{
		Use:   "connect",
		Short: "Пробросить локальный порт на сервис, открытый пиром через `spur expose`",
		RunE: func(cmd *cobra.Command, args []string) error {
			if serverAddr == "" || stunAddr == "" || localPort == 0 {
				return errors.New("connect: укажите --server, --stun-server и --local-port")
			}
			if peerID != "" && roomName != "" {
				return errors.New("connect: укажите либо --to, либо --room, не оба сразу")
			}
			return deps.Connect(cmd.Context(), serverAddr, stunAddr, peerID, roomName, identityPath, localPort, func(selfID string) {
				cmd.Printf("свой peer-id: %s\n", selfID)
			}, newCodePrinter(cmd), newVersionWarningPrinter(cmd))
		},
	}

	cmd.Flags().StringVar(&serverAddr, "server", serverAddr, "адрес rendezvous-сервера (control-канал)")
	cmd.Flags().StringVar(&stunAddr, "stun-server", stunAddr, "адрес STUN-эндпоинта сервера")
	cmd.Flags().StringVar(&peerID, "to", "", pairingToFlagHelp("пира, чей сервис пробрасываем"))
	cmd.Flags().StringVar(&roomName, "room", "", roomToFlagHelp("пиром, чей сервис пробрасываем"))
	cmd.Flags().StringVar(&identityPath, "identity", identityPath, "путь к файлу идентичности (по умолчанию — в конфиг-директории пользователя)")
	cmd.Flags().IntVar(&localPort, "local-port", 0, "локальный порт для прослушивания")

	return cmd
}
