package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

func newSendCommand(deps ClientDependencies, defaults ClientDefaults) *cobra.Command {
	var (
		serverAddr   = defaults.Server
		stunAddr     = defaults.StunServer
		peerID       string
		roomName     string
		identityPath = defaults.Identity
	)

	cmd := &cobra.Command{
		Use:   "send <path>",
		Short: msg().SendShort,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if serverAddr == "" || stunAddr == "" {
				return errors.New(msg().SendMissingFlags)
			}
			if peerID != "" && roomName != "" {
				return fmt.Errorf(msg().BothToAndRoom, "send")
			}
			err := deps.Send(cmd.Context(), serverAddr, stunAddr, peerID, roomName, identityPath, args[0], func(selfID string) {
				cmd.Printf(msg().SelfIDPrinted, selfID)
			}, newProgressPrinter(cmd.ErrOrStderr(), msg().ProgressVerbSend), newCodePrinter(cmd), newVersionWarningPrinter(cmd), newReconnectPrinter(cmd))
			progressDone(cmd.ErrOrStderr())
			return err
		},
	}

	cmd.Flags().StringVar(&serverAddr, "server", serverAddr, msg().FlagServer)
	cmd.Flags().StringVar(&stunAddr, "stun-server", stunAddr, msg().FlagStunServer)
	cmd.Flags().StringVar(&peerID, "to", "", pairingToFlagHelp(msg().SendToSubject))
	cmd.Flags().StringVar(&roomName, "room", "", roomToFlagHelp(msg().SendRoomSubject))
	cmd.Flags().StringVar(&identityPath, "identity", identityPath, msg().FlagIdentity)

	return cmd
}
