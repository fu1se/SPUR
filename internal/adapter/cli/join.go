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
		Short: msg().JoinShort,
		RunE: func(cmd *cobra.Command, args []string) error {
			if serverAddr == "" || stunAddr == "" || network == "" {
				return errors.New(msg().JoinMissingFlags)
			}
			return deps.Join(cmd.Context(), serverAddr, stunAddr, network, inviteToken, identityPath, func(selfID string) {
				cmd.Printf(msg().SelfIDPrinted, selfID)
			}, newVersionWarningPrinter(cmd))
		},
	}

	cmd.Flags().StringVar(&serverAddr, "server", serverAddr, msg().FlagRendezvousCoordSrv)
	cmd.Flags().StringVar(&stunAddr, "stun-server", stunAddr, msg().FlagStunServer)
	cmd.Flags().StringVar(&network, "network", "", msg().FlagNetwork)
	cmd.Flags().StringVar(&inviteToken, "invite", "", msg().FlagMeshInvite)
	cmd.Flags().StringVar(&identityPath, "identity", identityPath, msg().FlagIdentity)

	return cmd
}
