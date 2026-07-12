package cli

import (
	"errors"
	"fmt"

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
		Short: msg().ExposeShort,
		RunE: func(cmd *cobra.Command, args []string) error {
			if serverAddr == "" || stunAddr == "" || targetPort == 0 {
				return errors.New(msg().ExposeMissingFlags)
			}
			if peerID != "" && roomName != "" {
				return fmt.Errorf(msg().BothToAndRoom, "expose")
			}
			return deps.Expose(cmd.Context(), serverAddr, stunAddr, peerID, roomName, identityPath, targetPort, func(selfID string) {
				cmd.Printf(msg().SelfIDPrinted, selfID)
			}, newCodePrinter(cmd), newVersionWarningPrinter(cmd), newReconnectPrinter(cmd))
		},
	}

	cmd.Flags().StringVar(&serverAddr, "server", serverAddr, msg().FlagServer)
	cmd.Flags().StringVar(&stunAddr, "stun-server", stunAddr, msg().FlagStunServer)
	cmd.Flags().StringVar(&peerID, "to", "", pairingToFlagHelp(msg().ExposeToSubject))
	cmd.Flags().StringVar(&roomName, "room", "", roomToFlagHelp(msg().ExposeRoomSubject))
	cmd.Flags().StringVar(&identityPath, "identity", identityPath, msg().FlagIdentity)
	cmd.Flags().IntVar(&targetPort, "port", 0, msg().FlagPort)

	return cmd
}
