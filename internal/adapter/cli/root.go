// Package cli translates command-line invocations into use case calls.
// It is the outermost adapter for the interactive entrypoint of the
// application; it must not contain business logic itself, and it must not
// construct concrete adapter/infra implementations directly — those are
// wired in the composition roots (cmd/spur for the client, cmd/spur-server for
// the server) and handed to this package as plain functions via
// ClientDependencies/ServerDependencies.
//
// The client and server are separate binaries (cmd/spur, cmd/spur-server) so
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

// Version returns this binary's own build version — the same string
// `spur version`/`spur-server version` print. Exported so cmd/spur can
// pass it into control-plane RPCs (Register's client_version) and
// cmd/spur-server can pass it into controlserver.Server.Version, without
// either composition root reaching into this package's unexported state.
func Version() string {
	return version
}

// ClientDefaults holds fallback values for client flags a user would
// otherwise have to retype on every invocation (--server, --stun-server,
// --identity). Loaded from a config file in cmd/spur (an infra concern —
// this package must not touch disk itself); zero values here just mean
// the flag keeps its original empty default, so an absent config file
// changes nothing.
type ClientDefaults struct {
	Server     string
	StunServer string
	Identity   string

	// Lang is the raw config-file language override (empty if the
	// config file has none set) — used only to tell `spur lang` (with no
	// argument) whether the language currently in effect (SetLanguage,
	// already applied before NewClientRootCommand was called) came from
	// an explicit override or from system-locale auto-detection.
	Lang string
}

// ServerDefaults holds fallback values for server flags, loaded in
// cmd/spur-server.
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
// calls into. Every field is populated in cmd/spur; commands never know
// what concrete adapters sit behind them.
type ClientDependencies struct {
	// Register dials a control-plane server and registers an (ephemeral,
	// until Phase 7) identity with it. onVersionMismatch: see
	// VersionMismatchFunc.
	Register func(ctx context.Context, serverAddr string, onVersionMismatch VersionMismatchFunc) (RegisterResult, error)

	// Connect is "spur connect": rendezvous with a counterpart, establish
	// a P2P or relay session, and forward every connection to localPort
	// through it. identityPath is where the caller's persisted identity
	// lives (see infra.LoadOrCreateIdentity — without persistence across
	// restarts, the two ends could never learn each other's ID before it
	// changed again). onSelfID is called with the caller's own peer ID as
	// soon as it's known, before Connect starts blocking. The counterpart
	// is chosen by exactly one of: peerID (may be empty — see OnCodeFunc
	// for what that means), or roomName (see CreateRoom/JoinRoom — takes
	// priority over peerID when both are somehow set, though the CLI
	// layer rejects that combination before it gets here). onVersionMismatch:
	// see VersionMismatchFunc. onReconnect: see OnReconnectFunc — a lost
	// tunnel is re-established automatically, not fatal. Blocks until ctx
	// is cancelled or forwarding fails unrecoverably.
	Connect func(ctx context.Context, serverAddr, stunAddr, peerID, roomName, identityPath string, localPort int, onSelfID func(selfID string), onCode OnCodeFunc, onVersionMismatch VersionMismatchFunc, onReconnect OnReconnectFunc) error

	// Expose is "spur expose": rendezvous with a counterpart, establish a
	// P2P or relay session, and dial targetPort locally for every
	// incoming tunnel stream. identityPath, onSelfID, peerID, roomName,
	// onCode, onVersionMismatch, onReconnect: see Connect. Blocks until
	// ctx is cancelled or serving fails unrecoverably.
	Expose func(ctx context.Context, serverAddr, stunAddr, peerID, roomName, identityPath string, targetPort int, onSelfID func(selfID string), onCode OnCodeFunc, onVersionMismatch VersionMismatchFunc, onReconnect OnReconnectFunc) error

	// Whoami loads (or creates) the local identity and returns its peer
	// ID, without any network access — the bootstrap step for learning
	// your own ID before sharing it with a counterpart out-of-band.
	Whoami func(identityPath string) (string, error)

	// JoinNetwork registers with a mesh network on the server and returns
	// its current membership. Control-plane only, no TUN — same
	// "validate the control-plane piece in isolation" pattern as Register
	// did for Phase 2. inviteToken is required to join a network that
	// already exists (irrelevant when creating a new one or rejoining a
	// network the caller is already a member of). onVersionMismatch: see
	// VersionMismatchFunc.
	JoinNetwork func(ctx context.Context, serverAddr, networkName, inviteToken, identityPath string, onVersionMismatch VersionMismatchFunc) (JoinNetworkResult, error)

	// Join is "spur join": full mesh VPN mode — join the network, tunnel
	// to every other member, and route traffic through a real TUN
	// interface. Requires elevated privileges (root/CAP_NET_ADMIN on
	// Linux). inviteToken: see JoinNetwork. verbose switches the
	// WireGuard device's own logger from Error to Verbose (handshake
	// init/complete, peer add/remove) — off by default since it's noisy
	// on every routine handshake, but the only way to see whether a
	// stuck peer ever gets as far as attempting one at all. onSelfID:
	// see Connect. onVersionMismatch: see VersionMismatchFunc. Blocks
	// until ctx is cancelled.
	Join func(ctx context.Context, serverAddr, stunAddr, networkName, inviteToken, identityPath string, verbose bool, onSelfID func(selfID string), onVersionMismatch VersionMismatchFunc) error

	// CreateRoom is "spur room create": creates a brand-new, persistent,
	// two-member room named roomName on the server with the caller as its
	// first member, returning an invite token to hand to the second
	// participant. Unlike a pairing code, a room never expires and, once
	// full, can be used as the --room counterpart for connect/expose/
	// send/receive indefinitely without exchanging anything again.
	// onVersionMismatch: see VersionMismatchFunc.
	CreateRoom func(ctx context.Context, serverAddr, roomName, identityPath string, onVersionMismatch VersionMismatchFunc) (RoomResult, error)

	// JoinRoom is "spur room join": adds the caller as roomName's second
	// member using inviteToken (see CreateRoom). Idempotent if the caller
	// is already a member — inviteToken isn't re-checked in that case.
	// onVersionMismatch: see VersionMismatchFunc.
	JoinRoom func(ctx context.Context, serverAddr, roomName, inviteToken, identityPath string, onVersionMismatch VersionMismatchFunc) error

	// Send is "spur send": rendezvous with a counterpart and stream path
	// (a file or a directory, walked recursively) through the tunnel to
	// whoever runs "spur receive" against the same counterpart.
	// identityPath, onSelfID, peerID, roomName, onCode,
	// onVersionMismatch, onReconnect: see Connect. onProgress: see
	// ProgressFunc. Blocks until the transfer finishes or fails
	// unrecoverably — a network drop mid-transfer reconnects and resumes.
	Send func(ctx context.Context, serverAddr, stunAddr, peerID, roomName, identityPath, path string, onSelfID func(selfID string), onProgress ProgressFunc, onCode OnCodeFunc, onVersionMismatch VersionMismatchFunc, onReconnect OnReconnectFunc) error

	// Receive is "spur receive": rendezvous with a counterpart and write
	// whatever "spur send" streams through the tunnel under destDir,
	// recreating the relative directory structure the sender walked.
	// identityPath, onSelfID, peerID, roomName, onCode,
	// onVersionMismatch, onReconnect: see Connect. onProgress: see
	// ProgressFunc. onResumeOffer: see ResumeOfferFunc — only consulted
	// on the first attempt; automatic reconnect attempts always resume
	// (the partial data came from this very transfer moments ago).
	// Blocks until the transfer finishes or fails unrecoverably.
	Receive func(ctx context.Context, serverAddr, stunAddr, peerID, roomName, identityPath, destDir string, onSelfID func(selfID string), onProgress ProgressFunc, onCode OnCodeFunc, onResumeOffer ResumeOfferFunc, onVersionMismatch VersionMismatchFunc, onReconnect OnReconnectFunc) error

	// DesktopShare is "spur desktop share": start a local desktop (VNC)
	// server and serve it to the counterpart through a persistent tunnel.
	// viewOnly disables remote input. onReady: see DesktopShareReadyFunc.
	// Everything else: see Connect. Blocks until ctx is cancelled.
	DesktopShare func(ctx context.Context, serverAddr, stunAddr, peerID, roomName, identityPath string, viewOnly bool, onSelfID func(selfID string), onCode OnCodeFunc, onReady DesktopShareReadyFunc, onVersionMismatch VersionMismatchFunc, onReconnect OnReconnectFunc) error

	// DesktopView is "spur desktop view": forward a local loopback port
	// to the counterpart's shared desktop through a persistent tunnel and
	// launch a VNC viewer at it. localPort 0 picks the first free port in
	// VNC's traditional 5900-5999 range. onReady: see
	// DesktopViewReadyFunc. Everything else: see Connect. Blocks until
	// ctx is cancelled.
	DesktopView func(ctx context.Context, serverAddr, stunAddr, peerID, roomName, identityPath string, localPort int, onSelfID func(selfID string), onCode OnCodeFunc, onReady DesktopViewReadyFunc, onVersionMismatch VersionMismatchFunc, onReconnect OnReconnectFunc) error

	// SetLanguage is "spur lang": persists a UI language override (or
	// clears it, for lang == "") to the config file for future
	// invocations. Doesn't affect the language already in effect for
	// this process (SetLanguage the package-level function was already
	// called, before this command tree was even built) — just what the
	// next invocation picks up.
	SetLanguage func(lang string) error
}

// RoomResult is what a successful `spur room create` reports back to the
// CLI layer.
type RoomResult struct {
	InviteToken string // share with the second participant
}

// OnCodeFunc is called with a freshly minted pairing code when the
// corresponding peerID argument was left empty — "host" mode of the
// single-command connect flow (see cmd/spur/tunnel.go's
// counterpartResolverFor): instead of already knowing who they're
// talking to, the caller registers a short code with the server and
// waits for a counterpart to use it (invoking a matching command with
// that code as its own peerID argument). nil is valid and means "don't
// report" — same nil-safe-callback pattern as ProgressFunc/
// controlserver.Server.Logger.
type OnCodeFunc func(code string)

// ProgressFunc reports incremental progress during a file transfer:
// relPath is the file currently in flight, fileDone/fileTotal describe
// just that file, overallDone/overallTotal the whole transfer. Both sides
// know the real overallTotal from the start of content transfer — the
// receiver reads the sender's full manifest before any file bytes arrive
// (see usecase.TransferProgress's doc comment). This mirrors that
// usecase-layer type's shape rather than importing it directly, same
// reason RegisterResult/JoinNetworkResult are cli's own mirror types
// instead of aliases onto domain/controlclient types — cli must not
// depend on usecase.
type ProgressFunc func(relPath string, fileDone, fileTotal, overallDone, overallTotal int64)

// VersionMismatchFunc is called when a control-plane RPC's response
// reveals the server is running a different build version than this
// client — best-effort compatibility hint, not a hard failure: the
// client doesn't know which specific features differ, only that the
// versions don't match and something might not work as expected. nil is
// valid and means "don't report" — same nil-safe-callback pattern as
// ProgressFunc/OnCodeFunc.
type VersionMismatchFunc func(clientVersion, serverVersion string)

// ResumeOfferFunc is asked whether to resume a detected partially-complete
// file transfer instead of starting over: filesWithData is how many
// entries in the manifest already have some bytes present at the
// destination, alreadyHave/total describe the combined size across all
// files. Mirrors usecase.ResumeOffer's shape rather than importing it
// directly, same reasoning as ProgressFunc/OnCodeFunc. nil is valid and
// means "always start fresh" — the same behavior as before resume support
// existed.
type ResumeOfferFunc func(filesWithData int, alreadyHave, total int64) bool

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
// (spur) with all client subcommands wired against deps. defaults
// pre-fills flags from a config file; every field can still be overridden
// per-invocation by passing the flag explicitly.
func NewClientRootCommand(deps ClientDependencies, defaults ClientDefaults) *cobra.Command {
	root := &cobra.Command{
		Use:          "spur",
		Short:        msg().RootClientShort,
		SilenceUsage: true,
		// SilenceErrors: cobra's own "Error: <err>" print bypasses
		// Explain — the composition root (cmd/spur's main) prints the
		// error itself after ExecuteContext returns, running it through
		// Explain first for a human-friendly message instead of a bare
		// Go error chain.
		SilenceErrors: true,
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
		newRoomCommand(deps, defaults),
		newDesktopCommand(deps, defaults),
		newLangCommand(deps, defaults),
	)

	return root
}

// NewServerRootCommand builds the server binary's root cobra command
// (spur-server). Serving is the root command's own action — there is
// nothing else this binary does — with "version" as its only subcommand.
func NewServerRootCommand(deps ServerDependencies, defaults ServerDefaults) *cobra.Command {
	var listenAddr, stunAddr, dbPath string
	var verbose bool

	root := &cobra.Command{
		Use:           "spur-server",
		Short:         msg().RootServerShort,
		SilenceUsage:  true,
		SilenceErrors: true, // see NewClientRootCommand's doc comment on the same field
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Printf(msg().ServerListening, listenAddr, stunAddr, dbPath)
			return deps.RunServer(cmd.Context(), listenAddr, stunAddr, dbPath, verbose)
		},
	}

	root.Flags().StringVar(&listenAddr, "listen", ":4443", msg().FlagListen)
	root.Flags().StringVar(&stunAddr, "stun-listen", ":4444", msg().FlagStunListen)
	root.Flags().StringVar(&dbPath, "db", defaults.State, msg().FlagDB)
	root.Flags().BoolVar(&verbose, "verbose", false, msg().FlagVerbose)

	root.AddCommand(newVersionCommand())

	return root
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: msg().VersionShort,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), version)
			return nil
		},
	}
}
