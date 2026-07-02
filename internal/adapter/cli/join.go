package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func newJoinCommand(deps ClientDependencies, defaults ClientDefaults) *cobra.Command {
	var (
		serverAddr   = defaults.Server
		stunAddr     = defaults.StunServer
		network      string
		inviteToken  string
		identityPath = defaults.Identity
	)

	cmd := &cobra.Command{
		Use:   "join",
		Short: "Присоединиться к mesh-сети (полноценный доступ в локальную сеть через TUN)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if serverAddr == "" || stunAddr == "" || network == "" {
				return errors.New("join: укажите --server, --stun-server и --network")
			}
			return deps.Join(cmd.Context(), serverAddr, stunAddr, network, inviteToken, identityPath, func(selfID string) {
				cmd.Printf("свой peer-id: %s\n", selfID)
			})
		},
	}

	cmd.Flags().StringVar(&serverAddr, "server", serverAddr, "адрес rendezvous/coordination-сервера")
	cmd.Flags().StringVar(&stunAddr, "stun-server", stunAddr, "адрес STUN-эндпоинта сервера")
	cmd.Flags().StringVar(&network, "network", "", "имя mesh-сети")
	cmd.Flags().StringVar(&inviteToken, "invite", "", "инвайт-токен сети (не нужен при создании новой сети или повторном join)")
	cmd.Flags().StringVar(&identityPath, "identity", identityPath, "путь к файлу идентичности (по умолчанию — в конфиг-директории пользователя)")

	return cmd
}
