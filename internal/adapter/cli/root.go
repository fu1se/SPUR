// Package cli translates command-line invocations into use case calls.
// It is the outermost adapter for the interactive entrypoint of the
// application; it must not contain business logic itself, and it must not
// construct concrete adapter/infra implementations directly — those are
// wired in the composition roots (cmd/app for the client, cmd/server for
// the server) and handed to this package as plain functions via
// ClientDependencies/ServerDependencies.
//
// The client and server are separate binaries (cmd/app, cmd/server) so
// the client doesn't have to link in server-only weight (SQLite driver,
// controlserver, STUN responder) it never runs — see CLAUDE.md's
// "Разделение клиента и сервера" for why. This package still builds both
// command trees, since translating flags to use case calls is the same
// kind of work either way; only the composition roots differ.
package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

// version is set at build time via -ldflags "-X ... version=...".
var version = "dev"

// ClientDefaults holds fallback values for client flags a user would
// otherwise have to retype on every invocation (--server, --stun-server,
// --identity). Loaded from a config file in cmd/app (an infra concern —
// this package must not touch disk itself); zero values here just mean
// the flag keeps its original empty default, so an absent config file
// changes nothing.
type ClientDefaults struct {
	Server     string
	StunServer string
	Identity   string
}

// ServerDefaults holds fallback values for server flags, loaded in
// cmd/server.
type ServerDefaults struct {
	State string
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

// ClientDependencies holds the wired entrypoints each client subcommand
// calls into. Every field is populated in cmd/app; commands never know
// what concrete adapters sit behind them.
type ClientDependencies struct {
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

	// Send is "app send": rendezvous with peerID and stream path (a file
	// or a directory, walked recursively) through the tunnel to whoever
	// runs "app receive" against the same peerID. identityPath, onSelfID:
	// see Connect. Blocks until the transfer finishes or fails.
	Send func(ctx context.Context, serverAddr, stunAddr, peerID, identityPath, path string, onSelfID func(selfID string)) error

	// Receive is "app receive": rendezvous with peerID and write whatever
	// "app send" streams through the tunnel under destDir, recreating the
	// relative directory structure the sender walked. identityPath,
	// onSelfID: see Connect. Blocks until the transfer finishes or fails.
	Receive func(ctx context.Context, serverAddr, stunAddr, peerID, identityPath, destDir string, onSelfID func(selfID string)) error
}

// ServerDependencies holds the wired entrypoint the server binary's root
// command calls into.
type ServerDependencies struct {
	// RunServer starts the rendezvous/control-plane server plus its STUN
	// endpoint and blocks until ctx is cancelled. dbPath is where server
	// state (peers, mesh networks) persists across restarts — see
	// adapter/repository/sqlite. verbose switches operational logging
	// from info to debug level.
	RunServer func(ctx context.Context, listenAddr, stunAddr, dbPath string, verbose bool) error
}

// NewClientRootCommand builds the client binary's root cobra command
// (app) with all client subcommands wired against deps. defaults
// pre-fills flags from a config file; every field can still be overridden
// per-invocation by passing the flag explicitly.
func NewClientRootCommand(deps ClientDependencies, defaults ClientDefaults) *cobra.Command {
	root := &cobra.Command{
		Use:           "app",
		Short:         "localizator — прямое подключение в локальную сеть в обход NAT (клиент)",
		SilenceUsage:  true,
		SilenceErrors: false,
	}

	root.AddCommand(
		newVersionCommand(),
		newRegisterCommand(deps, defaults),
		newWhoamiCommand(deps, defaults),
		newConnectCommand(deps, defaults),
		newExposeCommand(deps, defaults),
		newJoinCommand(deps, defaults),
		newJoinNetworkCommand(deps, defaults),
		newSendCommand(deps, defaults),
		newReceiveCommand(deps, defaults),
	)

	return root
}

// NewServerRootCommand builds the server binary's root cobra command
// (app-server). Serving is the root command's own action — there is
// nothing else this binary does — with "version" as its only subcommand.
func NewServerRootCommand(deps ServerDependencies, defaults ServerDefaults) *cobra.Command {
	var listenAddr, stunAddr, dbPath string
	var verbose bool

	root := &cobra.Command{
		Use:           "app-server",
		Short:         "localizator — rendezvous/signaling-сервер (control plane + STUN + relay fallback)",
		SilenceUsage:  true,
		SilenceErrors: false,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Printf("control-plane слушает на %s, STUN — на %s, состояние — в %s\n", listenAddr, stunAddr, dbPath)
			return deps.RunServer(cmd.Context(), listenAddr, stunAddr, dbPath, verbose)
		},
	}

	root.Flags().StringVar(&listenAddr, "listen", ":4443", "адрес control-канала (QUIC)")
	root.Flags().StringVar(&stunAddr, "stun-listen", ":4444", "адрес STUN-эндпоинта (UDP)")
	root.Flags().StringVar(&dbPath, "db", defaults.State, "путь к файлу состояния сервера (SQLite)")
	root.Flags().BoolVar(&verbose, "verbose", false, "подробные (debug-уровня) логи вместо info")

	root.AddCommand(newVersionCommand())

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
