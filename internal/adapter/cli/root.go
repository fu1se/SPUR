// Package cli translates command-line invocations into use case calls.
// It is the outermost adapter for the interactive entrypoint of the
// application; it must not contain business logic itself, and it must not
// construct concrete adapter/infra implementations directly — those are
// wired in cmd/app (the composition root) and handed to this package as
// plain functions via Dependencies.
package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

// version is set at build time via -ldflags "-X ... version=...".
var version = "dev"

// Defaults holds fallback values for flags a user would otherwise have to
// retype on every invocation (--server, --stun-server, --identity). Loaded
// from a config file in cmd/app (an infra concern — this package must not
// touch disk itself); zero values here just mean the flag keeps its
// original empty default, so an absent config file changes nothing.
type Defaults struct {
	Server     string
	StunServer string
	Identity   string
}

// RegisterResult is what a successful control-plane registration reports
// back to the CLI layer.
type RegisterResult struct {
	PeerID          string
	ObservedAddress string
}

// MeshMemberResult is one member of a mesh network, as reported back to
// the CLI layer.
type MeshMemberResult struct {
	PeerID string
	MeshIP string
}

// JoinNetworkResult is what a successful mesh network join reports back to
// the CLI layer.
type JoinNetworkResult struct {
	CIDR        string
	Members     []MeshMemberResult
	InviteToken string // share with whoever should join this network next
}

// Dependencies holds the wired entrypoints each subcommand calls into.
// Every field is populated in cmd/app; commands never know what concrete
// adapters sit behind them.
type Dependencies struct {
	// RunServer starts the rendezvous/control-plane server plus its STUN
	// endpoint and blocks until ctx is cancelled.
	RunServer func(ctx context.Context, listenAddr, stunAddr string) error

	// Register dials a control-plane server and registers an (ephemeral,
	// until Phase 7) identity with it.
	Register func(ctx context.Context, serverAddr string) (RegisterResult, error)

	// Connect is "app connect": rendezvous with peerID, establish a P2P or
	// relay session, and forward every connection to localPort through it.
	// identityPath is where the caller's persisted identity lives (see
	// infra.LoadOrCreateIdentity — without persistence across restarts,
	// the two ends could never learn each other's ID before it changed
	// again). onSelfID is called with the caller's own peer ID as soon as
	// it's known, before Connect starts blocking. Blocks until ctx is
	// cancelled or forwarding fails.
	Connect func(ctx context.Context, serverAddr, stunAddr, peerID, identityPath string, localPort int, onSelfID func(selfID string)) error

	// Expose is "app expose": rendezvous with peerID, establish a P2P or
	// relay session, and dial targetPort locally for every incoming
	// tunnel stream. identityPath, onSelfID: see Connect. Blocks until
	// ctx is cancelled or serving fails.
	Expose func(ctx context.Context, serverAddr, stunAddr, peerID, identityPath string, targetPort int, onSelfID func(selfID string)) error

	// Whoami loads (or creates) the local identity and returns its peer
	// ID, without any network access — the bootstrap step for learning
	// your own ID before sharing it with a counterpart out-of-band.
	Whoami func(identityPath string) (string, error)

	// JoinNetwork registers with a mesh network on the server and returns
	// its current membership. Control-plane only, no TUN — same
	// "validate the control-plane piece in isolation" pattern as Register
	// did for Phase 2. inviteToken is required to join a network that
	// already exists (irrelevant when creating a new one or rejoining a
	// network the caller is already a member of).
	JoinNetwork func(ctx context.Context, serverAddr, networkName, inviteToken, identityPath string) (JoinNetworkResult, error)

	// Join is "app join": full mesh VPN mode — join the network, tunnel
	// to every other member, and route traffic through a real TUN
	// interface. Requires elevated privileges (root/CAP_NET_ADMIN on
	// Linux). inviteToken: see JoinNetwork. onSelfID: see Connect. Blocks
	// until ctx is cancelled.
	Join func(ctx context.Context, serverAddr, stunAddr, networkName, inviteToken, identityPath string, onSelfID func(selfID string)) error
}

// NewRootCommand builds the root cobra command with all subcommands wired
// against deps. defaults pre-fills flags from a config file; every field
// can still be overridden per-invocation by passing the flag explicitly.
func NewRootCommand(deps Dependencies, defaults Defaults) *cobra.Command {
	root := &cobra.Command{
		Use:           "app",
		Short:         "localizator — прямое подключение в локальную сеть в обход NAT",
		SilenceUsage:  true,
		SilenceErrors: false,
	}

	root.AddCommand(
		newVersionCommand(),
		newServerCommand(deps),
		newRegisterCommand(deps, defaults),
		newWhoamiCommand(deps, defaults),
		newConnectCommand(deps, defaults),
		newExposeCommand(deps, defaults),
		newJoinCommand(deps, defaults),
		newJoinNetworkCommand(deps, defaults),
	)

	return root
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Показать версию",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), version)
			return nil
		},
	}
}
