package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

// newJoinNetworkCommand exercises the Phase 6 server-side mesh
// coordination end to end — join a network, print the assigned CIDR and
// full membership list — without touching TUN/wireguard-go. It exists for
// the same reason `register` did for Phase 2: validate the control-plane
// piece independently of the data-plane piece (`spur join`) that isn't
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
		Short: msg().JoinNetworkShort,
		RunE: func(cmd *cobra.Command, args []string) error {
			if serverAddr == "" || networkName == "" {
				return errors.New(msg().JoinNetworkMissingFlags)
			}
			result, err := deps.JoinNetwork(cmd.Context(), serverAddr, networkName, inviteToken, identityPath, newVersionWarningPrinter(cmd))
			if err != nil {
				return err
			}
			cmd.Printf(msg().JoinNetworkPrinted, networkName, result.CIDR)
			if result.InviteToken != "" {
				cmd.Printf(msg().JoinNetworkInviteToken, result.InviteToken)
			}
			for _, m := range result.Members {
				cmd.Printf(msg().JoinNetworkMemberPrinted, m.PeerID, m.MeshIP)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&serverAddr, "server", serverAddr, msg().FlagServerPlain)
	cmd.Flags().StringVar(&networkName, "network", "", msg().FlagNetwork)
	cmd.Flags().StringVar(&inviteToken, "invite", "", msg().FlagMeshInvite)
	cmd.Flags().StringVar(&identityPath, "identity", identityPath, msg().FlagIdentity)

	return cmd
}
