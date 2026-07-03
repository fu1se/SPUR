package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func newExposeCommand(deps ClientDependencies, defaults ClientDefaults) *cobra.Command {
	var (
		serverAddr   = defaults.Server
		stunAddr     = defaults.StunServer
		peerID       string
		roomName     string
		identityPath = defaults.Identity
		targetPort   int
	)

	cmd := &cobra.Command{
		Use:   "expose",
		Short: "Открыть локальный сервис указанному пиру (port-forward режим)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if serverAddr == "" || stunAddr == "" || targetPort == 0 {
				return errors.New("expose: укажите --server, --stun-server и --port")
			}
			if peerID != "" && roomName != "" {
				return errors.New("expose: укажите либо --to, либо --room, не оба сразу")
			}
			return deps.Expose(cmd.Context(), serverAddr, stunAddr, peerID, roomName, identityPath, targetPort, func(selfID string) {
				cmd.Printf("свой peer-id: %s\n", selfID)
			}, newCodePrinter(cmd), newVersionWarningPrinter(cmd))
		},
	}

	cmd.Flags().StringVar(&serverAddr, "server", serverAddr, "адрес rendezvous-сервера (control-канал)")
	cmd.Flags().StringVar(&stunAddr, "stun-server", stunAddr, "адрес STUN-эндпоинта сервера")
	cmd.Flags().StringVar(&peerID, "to", "", pairingToFlagHelp("пира, которому разрешено подключаться"))
	cmd.Flags().StringVar(&roomName, "room", "", roomToFlagHelp("пиром, которому разрешено подключаться"))
	cmd.Flags().StringVar(&identityPath, "identity", identityPath, "путь к файлу идентичности (по умолчанию — в конфиг-директории пользователя)")
	cmd.Flags().IntVar(&targetPort, "port", 0, "локальный порт сервиса, который открываем")

	return cmd
}
