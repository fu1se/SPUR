package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func newExposeCommand(deps Dependencies, defaults Defaults) *cobra.Command {
	var (
		serverAddr   = defaults.Server
		stunAddr     = defaults.StunServer
		peerID       string
		identityPath = defaults.Identity
		targetPort   int
	)

	cmd := &cobra.Command{
		Use:   "expose",
		Short: "Открыть локальный сервис указанному пиру (port-forward режим)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if serverAddr == "" || stunAddr == "" || peerID == "" || targetPort == 0 {
				return errors.New("expose: укажите --server, --stun-server, --to и --port")
			}
			return deps.Expose(cmd.Context(), serverAddr, stunAddr, peerID, identityPath, targetPort, func(selfID string) {
				cmd.Printf("свой peer-id: %s\n", selfID)
			})
		},
	}

	cmd.Flags().StringVar(&serverAddr, "server", serverAddr, "адрес rendezvous-сервера (control-канал)")
	cmd.Flags().StringVar(&stunAddr, "stun-server", stunAddr, "адрес STUN-эндпоинта сервера")
	cmd.Flags().StringVar(&peerID, "to", "", "идентификатор пира, которому разрешено подключаться")
	cmd.Flags().StringVar(&identityPath, "identity", identityPath, "путь к файлу идентичности (по умолчанию — в конфиг-директории пользователя)")
	cmd.Flags().IntVar(&targetPort, "port", 0, "локальный порт сервиса, который открываем")

	return cmd
}
