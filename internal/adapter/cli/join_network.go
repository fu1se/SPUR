package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

// newJoinNetworkCommand exercises the Phase 6 server-side mesh
// coordination end to end — join a network, print the assigned CIDR and
// full membership list — without touching TUN/wireguard-go. It exists for
// the same reason `register` did for Phase 2: validate the control-plane
// piece independently of the data-plane piece (`app join`) that isn't
// wired up yet.
func newJoinNetworkCommand(deps ClientDependencies, defaults ClientDefaults) *cobra.Command {
	var (
		serverAddr   = defaults.Server
		networkName  string
		inviteToken  string
		identityPath = defaults.Identity
	)

	cmd := &cobra.Command{
		Use:   "join-network",
		Short: "Присоединиться к mesh-сети на сервере и показать её участников (без TUN)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if serverAddr == "" || networkName == "" {
				return errors.New("join-network: укажите --server и --network")
			}
			result, err := deps.JoinNetwork(cmd.Context(), serverAddr, networkName, inviteToken, identityPath)
			if err != nil {
				return err
			}
			cmd.Printf("сеть: %s, cidr: %s\n", networkName, result.CIDR)
			if result.InviteToken != "" {
				cmd.Printf("инвайт-токен (передайте тем, кто будет присоединяться): %s\n", result.InviteToken)
			}
			for _, m := range result.Members {
				cmd.Printf("  участник: %s  mesh-ip: %s\n", m.PeerID, m.MeshIP)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&serverAddr, "server", serverAddr, "адрес rendezvous-сервера")
	cmd.Flags().StringVar(&networkName, "network", "", "имя mesh-сети")
	cmd.Flags().StringVar(&inviteToken, "invite", "", "инвайт-токен сети (не нужен при создании новой сети или повторном join)")
	cmd.Flags().StringVar(&identityPath, "identity", identityPath, "путь к файлу идентичности (по умолчанию — в конфиг-директории пользователя)")

	return cmd
}
