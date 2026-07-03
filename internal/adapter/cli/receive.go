package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

func newReceiveCommand(deps ClientDependencies, defaults ClientDefaults) *cobra.Command {
	var (
		serverAddr   = defaults.Server
		stunAddr     = defaults.StunServer
		peerID       string
		roomName     string
		identityPath = defaults.Identity
		outDir       string
	)

	cmd := &cobra.Command{
		Use:   "receive",
		Short: msg().ReceiveShort,
		RunE: func(cmd *cobra.Command, args []string) error {
			if serverAddr == "" || stunAddr == "" || outDir == "" {
				return errors.New(msg().ReceiveMissingFlags)
			}
			if peerID != "" && roomName != "" {
				return fmt.Errorf(msg().BothToAndRoom, "receive")
			}
			err := deps.Receive(cmd.Context(), serverAddr, stunAddr, peerID, roomName, identityPath, outDir, func(selfID string) {
				cmd.Printf(msg().SelfIDPrinted, selfID)
			}, newProgressPrinter(cmd.ErrOrStderr(), msg().ProgressVerbReceive), newCodePrinter(cmd), newResumePrompt(cmd), newVersionWarningPrinter(cmd))
			progressDone(cmd.ErrOrStderr())
			return err
		},
	}

	cmd.Flags().StringVar(&serverAddr, "server", serverAddr, msg().FlagServer)
	cmd.Flags().StringVar(&stunAddr, "stun-server", stunAddr, msg().FlagStunServer)
	cmd.Flags().StringVar(&peerID, "to", "", pairingToFlagHelp(msg().ReceiveToSubject))
	cmd.Flags().StringVar(&roomName, "room", "", roomToFlagHelp(msg().ReceiveRoomSubject))
	cmd.Flags().StringVar(&identityPath, "identity", identityPath, msg().FlagIdentity)
	cmd.Flags().StringVar(&outDir, "out", "", msg().FlagOutDir)

	return cmd
}
