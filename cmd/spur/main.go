// Command app is the client composition root: it wires concrete
// adapter/infra implementations into use cases and hands control to the
// CLI. No business logic lives here.
//
// This binary deliberately does not import controlserver, stunserver, or
// adapter/repository (sqlite or memory) — those are server-only weight,
// wired instead in cmd/spur-server. Keeping them out of this build is the
// whole point of the split: a client running "spur connect" has no reason
// to link in a SQLite driver it will never open.
package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"time"

	"github.com/fu1se/spur/internal/adapter/cli"
	"github.com/fu1se/spur/internal/adapter/controlclient"
	"github.com/fu1se/spur/internal/adapter/rendezvous"
	"github.com/fu1se/spur/internal/domain"
	"github.com/fu1se/spur/internal/infra"
)

// interruptConfirmWindow is how long a user has, after the first Ctrl+C,
// to press it again before it's treated as accidental and ignored — long
// enough for a deliberate second press, short enough not to feel broken.
const interruptConfirmWindow = 3 * time.Second

func main() {
	defaults, err := loadDefaults()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// Resolved before building the command tree: Short/flag-help text is
	// baked in at construction time, not at RunE time — see cli.SetLanguage's
	// doc comment for why this can't happen any later.
	cli.SetLanguage(defaults.Lang, infra.DetectSystemLanguage())

	ctx, stop := infra.ContextWithConfirmedInterrupt(context.Background(), interruptConfirmWindow, func() {
		fmt.Fprint(os.Stderr, cli.CtrlCWarningClient(interruptConfirmWindow))
	})
	defer stop()

	root := cli.NewClientRootCommand(cli.ClientDependencies{
		Register:    register,
		Connect:     connect,
		Expose:      expose,
		Whoami:      whoami,
		JoinNetwork: joinNetwork,
		Join:        join,
		Send:        send,
		Receive:     receive,
		CreateRoom:  createRoom,
		JoinRoom:    joinRoom,
		SetLanguage: setLanguage,
	}, defaults)

	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, cli.ErrorPrefix(), cli.Explain(err))
		os.Exit(1)
	}
}

// loadDefaults reads the optional config file (see infra.Config's doc
// comment) into cli.ClientDefaults. A missing file just means every flag
// keeps its original empty default — the config file is purely additive.
func loadDefaults() (cli.ClientDefaults, error) {
	path, err := infra.DefaultConfigPath()
	if err != nil {
		return cli.ClientDefaults{}, err
	}
	cfg, err := infra.LoadConfig(path)
	if err != nil {
		return cli.ClientDefaults{}, err
	}
	return cli.ClientDefaults{
		Server:     cfg.Server,
		StunServer: cfg.StunServer,
		Identity:   cfg.Identity,
		Lang:       cfg.Lang,
	}, nil
}

// setLanguage is "spur lang <ru|en|auto>": persists a UI language
// override to the config file for future invocations (lang == "" clears
// it back to system-locale auto-detection). Doesn't touch every other
// field in the config file — loads the existing one first so a saved
// --server/--stun-server/--identity default isn't wiped out by changing
// the language.
func setLanguage(lang string) error {
	path, err := infra.DefaultConfigPath()
	if err != nil {
		return err
	}
	cfg, err := infra.LoadConfig(path)
	if err != nil {
		return err
	}
	cfg.Lang = lang
	return infra.SaveConfig(path, cfg)
}

// register dials serverAddr and registers an ephemeral identity. Real,
// persistent keypairs land in Phase 7 (see CLAUDE.md roadmap); for now a
// fresh random key is generated on every call, which is enough to exercise
// the control-plane wire protocol end to end.
func register(ctx context.Context, serverAddr string, onVersionMismatch cli.VersionMismatchFunc) (cli.RegisterResult, error) {
	var pub domain.PublicKey
	if _, err := rand.Read(pub[:]); err != nil {
		return cli.RegisterResult{}, fmt.Errorf("app: generate ephemeral key: %w", err)
	}

	tlsConf, err := rendezvous.ControlClientTLS(serverAddr)
	if err != nil {
		return cli.RegisterResult{}, err
	}

	client, err := controlclient.Dial(ctx, serverAddr, tlsConf, infra.DefaultQUICConfig())
	if err != nil {
		return cli.RegisterResult{}, err
	}
	defer client.Close()

	result, err := client.Register(ctx, pub, cli.Version())
	if err != nil {
		return cli.RegisterResult{}, err
	}
	rendezvous.WarnIfVersionMismatch(cli.Version(), result.ServerVersion, rendezvous.VersionMismatchFunc(onVersionMismatch))

	return cli.RegisterResult{
		PeerID:          string(result.PeerID),
		ObservedAddress: result.ObservedAddress,
	}, nil
}

// whoami loads (or creates) the local identity and returns its peer ID.
// Pure local operation, no network access — see resolveIdentityPath and
// rendezvous's doc comment for why this bootstrap step exists.
func whoami(identityPath string) (string, error) {
	resolvedIdentityPath, err := rendezvous.ResolveIdentityPath(identityPath)
	if err != nil {
		return "", err
	}
	id, err := infra.LoadOrCreateIdentity(resolvedIdentityPath)
	if err != nil {
		return "", fmt.Errorf("app: load identity: %w", err)
	}
	return string(domain.DerivePeerID(id.PublicKey)), nil
}

// joinNetwork loads (or creates) the local identity and joins a mesh
// network on the server, returning its current membership. Control-plane
// only — see cli.ClientDependencies.JoinNetwork's doc comment.
func joinNetwork(ctx context.Context, serverAddr, networkName, inviteToken, identityPath string, onVersionMismatch cli.VersionMismatchFunc) (cli.JoinNetworkResult, error) {
	client, id, err := rendezvous.DialAndRegister(ctx, serverAddr, identityPath, cli.Version(), rendezvous.VersionMismatchFunc(onVersionMismatch))
	if err != nil {
		return cli.JoinNetworkResult{}, err
	}
	defer client.Close()

	network, err := client.JoinNetwork(ctx, networkName, inviteToken, id.PublicKey)
	if err != nil {
		return cli.JoinNetworkResult{}, err
	}

	members := make([]cli.MeshMemberResult, 0, len(network.Members))
	for _, m := range network.Members {
		members = append(members, cli.MeshMemberResult{PeerID: string(m.PeerID), MeshIP: m.MeshIP.String()})
	}

	return cli.JoinNetworkResult{CIDR: network.CIDR.String(), Members: members, InviteToken: network.InviteToken}, nil
}
