package cli

import (
	"errors"
	"fmt"

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
		Short: msg().ConnectShort,
		RunE: func(cmd *cobra.Command, args []string) error {
			if serverAddr == "" || stunAddr == "" || localPort == 0 {
				return errors.New(msg().ConnectMissingFlags)
			}
			if peerID != "" && roomName != "" {
				return fmt.Errorf(msg().BothToAndRoom, "connect")
			}
			return deps.Connect(cmd.Context(), serverAddr, stunAddr, peerID, roomName, identityPath, localPort, func(selfID string) {
				cmd.Printf(msg().SelfIDPrinted, selfID)
			}, newCodePrinter(cmd), newVersionWarningPrinter(cmd))
		},
	}

	cmd.Flags().StringVar(&serverAddr, "server", serverAddr, msg().FlagServer)
	cmd.Flags().StringVar(&stunAddr, "stun-server", stunAddr, msg().FlagStunServer)
	cmd.Flags().StringVar(&peerID, "to", "", pairingToFlagHelp(msg().ConnectToSubject))
	cmd.Flags().StringVar(&roomName, "room", "", roomToFlagHelp(msg().ConnectRoomSubject))
	cmd.Flags().StringVar(&identityPath, "identity", identityPath, msg().FlagIdentity)
	cmd.Flags().IntVar(&localPort, "local-port", 0, msg().FlagLocalPort)

	return cmd
}
