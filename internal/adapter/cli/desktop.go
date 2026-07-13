package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

// DesktopShareReadyFunc is told the local desktop server is up: which
// backend was picked (x11vnc/wayvnc/gnome-remote-desktop) and the local
// port it serves on. Mirrors the callback shape convention of
// OnCodeFunc/ProgressFunc. nil-safe by convention on the cmd side.
type DesktopShareReadyFunc func(backend string, port int)

// DesktopViewReadyFunc is told the counterpart's desktop is reachable:
// localAddr is where a VNC client should connect, password is the
// session password the sharing side generated ("" when its backend
// didn't need one), viewer is the VNC client that was auto-launched
// ("" when none of the known ones is installed — the user connects
// manually then).
type DesktopViewReadyFunc func(localAddr, password, viewer string)

// newDesktopCommand groups "spur desktop share" and "spur desktop view"
// — remote desktop as a third data-plane payload next to port-forward
// and file transfer, riding the exact same rendezvous/tunnel machinery.
func newDesktopCommand(deps ClientDependencies, defaults ClientDefaults) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "desktop",
		Short: msg().DesktopShort,
	}
	cmd.AddCommand(newDesktopShareCommand(deps, defaults), newDesktopViewCommand(deps, defaults))
	return cmd
}

func newDesktopShareCommand(deps ClientDependencies, defaults ClientDefaults) *cobra.Command {
	var (
		serverAddr   = defaults.Server
		stunAddr     = defaults.StunServer
		peerID       string
		roomName     string
		identityPath = defaults.Identity
		viewOnly     bool
	)

	cmd := &cobra.Command{
		Use:   "share",
		Short: msg().DesktopShareShort,
		RunE: func(cmd *cobra.Command, args []string) error {
			if serverAddr == "" || stunAddr == "" {
				return errors.New(msg().DesktopMissingFlags)
			}
			if peerID != "" && roomName != "" {
				return fmt.Errorf(msg().BothToAndRoom, "desktop share")
			}
			return deps.DesktopShare(cmd.Context(), serverAddr, stunAddr, peerID, roomName, identityPath, viewOnly, func(selfID string) {
				cmd.Printf(msg().SelfIDPrinted, selfID)
			}, newCodePrinter(cmd), func(backend string, port int) {
				cmd.Printf(msg().DesktopShareReady, backend, port)
			}, newVersionWarningPrinter(cmd), newReconnectPrinter(cmd))
		},
	}

	cmd.Flags().StringVar(&serverAddr, "server", serverAddr, msg().FlagServer)
	cmd.Flags().StringVar(&stunAddr, "stun-server", stunAddr, msg().FlagStunServer)
	cmd.Flags().StringVar(&peerID, "to", "", pairingToFlagHelp(msg().DesktopShareToSubject))
	cmd.Flags().StringVar(&roomName, "room", "", roomToFlagHelp(msg().DesktopShareRoomSubject))
	cmd.Flags().StringVar(&identityPath, "identity", identityPath, msg().FlagIdentity)
	cmd.Flags().BoolVar(&viewOnly, "view-only", false, msg().FlagViewOnly)

	return cmd
}

func newDesktopViewCommand(deps ClientDependencies, defaults ClientDefaults) *cobra.Command {
	var (
		serverAddr   = defaults.Server
		stunAddr     = defaults.StunServer
		peerID       string
		roomName     string
		identityPath = defaults.Identity
		localPort    int
	)

	cmd := &cobra.Command{
		Use:   "view",
		Short: msg().DesktopViewShort,
		RunE: func(cmd *cobra.Command, args []string) error {
			if serverAddr == "" || stunAddr == "" {
				return errors.New(msg().DesktopMissingFlags)
			}
			if peerID != "" && roomName != "" {
				return fmt.Errorf(msg().BothToAndRoom, "desktop view")
			}
			return deps.DesktopView(cmd.Context(), serverAddr, stunAddr, peerID, roomName, identityPath, localPort, func(selfID string) {
				cmd.Printf(msg().SelfIDPrinted, selfID)
			}, newCodePrinter(cmd), func(localAddr, password, viewer string) {
				cmd.Printf(msg().DesktopViewReady, localAddr)
				if password != "" {
					cmd.Printf(msg().DesktopViewPassword, password)
				}
				if viewer != "" {
					cmd.Printf(msg().DesktopViewerLaunched, viewer)
				} else {
					cmd.Print(msg().DesktopViewerNotFound)
				}
			}, newVersionWarningPrinter(cmd), newReconnectPrinter(cmd))
		},
	}

	cmd.Flags().StringVar(&serverAddr, "server", serverAddr, msg().FlagServer)
	cmd.Flags().StringVar(&stunAddr, "stun-server", stunAddr, msg().FlagStunServer)
	cmd.Flags().StringVar(&peerID, "to", "", pairingToFlagHelp(msg().DesktopViewToSubject))
	cmd.Flags().StringVar(&roomName, "room", "", roomToFlagHelp(msg().DesktopViewRoomSubject))
	cmd.Flags().StringVar(&identityPath, "identity", identityPath, msg().FlagIdentity)
	cmd.Flags().IntVar(&localPort, "local-port", 0, msg().FlagDesktopLocalPort)

	return cmd
}
