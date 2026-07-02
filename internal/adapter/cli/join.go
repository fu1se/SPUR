package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func newJoinCommand(deps Dependencies) *cobra.Command {
	var (
		serverAddr   string
		stunAddr     string
		network      string
		identityPath string
	)

	cmd := &cobra.Command{
		Use:   "join",
		Short: "Присоединиться к mesh-сети (полноценный доступ в локальную сеть через TUN)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if serverAddr == "" || stunAddr == "" || network == "" {
				return errors.New("join: укажите --server, --stun-server и --network")
			}
			return deps.Join(cmd.Context(), serverAddr, stunAddr, network, identityPath, func(selfID string) {
				cmd.Printf("свой peer-id: %s\n", selfID)
			})
		},
	}

	cmd.Flags().StringVar(&serverAddr, "server", "", "адрес rendezvous/coordination-сервера")
	cmd.Flags().StringVar(&stunAddr, "stun-server", "", "адрес STUN-эндпоинта сервера")
	cmd.Flags().StringVar(&network, "network", "", "имя mesh-сети")
	cmd.Flags().StringVar(&identityPath, "identity", "", "путь к файлу идентичности (по умолчанию — в конфиг-директории пользователя)")

	return cmd
}
