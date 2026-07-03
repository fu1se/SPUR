package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

// roomToFlagHelp is the shared --room flag description for connect/
// expose/send/receive — an alternative to --to for two people who
// already set up a room via `spur room create`/`spur room join`.
func roomToFlagHelp(subject string) string {
	return fmt.Sprintf(msg().RoomToFlagHelpFormat, subject)
}

// newRoomCommand groups the two room-management subcommands under
// `spur room`, the same nesting pattern cobra encourages for a small
// family of related actions that don't fit as top-level verbs on their
// own (there's no bare "spur room" action, only "create"/"join").
func newRoomCommand(deps ClientDependencies, defaults ClientDefaults) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "room",
		Short: msg().RoomParentShort,
	}
	cmd.AddCommand(
		newRoomCreateCommand(deps, defaults),
		newRoomJoinCommand(deps, defaults),
	)
	return cmd
}

func newRoomCreateCommand(deps ClientDependencies, defaults ClientDefaults) *cobra.Command {
	var (
		serverAddr   = defaults.Server
		roomName     string
		identityPath = defaults.Identity
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: msg().RoomCreateShort,
		RunE: func(cmd *cobra.Command, args []string) error {
			if serverAddr == "" || roomName == "" {
				return errors.New(msg().RoomCreateMissingFlags)
			}
			result, err := deps.CreateRoom(cmd.Context(), serverAddr, roomName, identityPath, newVersionWarningPrinter(cmd))
			if err != nil {
				return err
			}
			cmd.Printf(msg().RoomCreatedPrinted, roomName)
			cmd.Printf(msg().RoomInviteToken, result.InviteToken)
			cmd.Printf(msg().RoomUsageHint, roomName)
			return nil
		},
	}

	cmd.Flags().StringVar(&serverAddr, "server", serverAddr, msg().FlagServerPlain)
	cmd.Flags().StringVar(&roomName, "room", "", msg().FlagRoomNameNew)
	cmd.Flags().StringVar(&identityPath, "identity", identityPath, msg().FlagIdentity)

	return cmd
}

func newRoomJoinCommand(deps ClientDependencies, defaults ClientDefaults) *cobra.Command {
	var (
		serverAddr   = defaults.Server
		roomName     string
		inviteToken  string
		identityPath = defaults.Identity
	)

	cmd := &cobra.Command{
		Use:   "join",
		Short: msg().RoomJoinShort,
		RunE: func(cmd *cobra.Command, args []string) error {
			if serverAddr == "" || roomName == "" {
				return errors.New(msg().RoomJoinMissingFlags)
			}
			if err := deps.JoinRoom(cmd.Context(), serverAddr, roomName, inviteToken, identityPath, newVersionWarningPrinter(cmd)); err != nil {
				return err
			}
			cmd.Printf(msg().RoomJoinedPrinted, roomName)
			cmd.Printf(msg().RoomUsageHint, roomName)
			return nil
		},
	}

	cmd.Flags().StringVar(&serverAddr, "server", serverAddr, msg().FlagServerPlain)
	cmd.Flags().StringVar(&roomName, "room", "", msg().FlagRoomName)
	cmd.Flags().StringVar(&inviteToken, "invite", "", msg().FlagRoomInvite)
	cmd.Flags().StringVar(&identityPath, "identity", identityPath, msg().FlagIdentity)

	return cmd
}
