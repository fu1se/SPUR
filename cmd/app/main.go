// Command app is the client composition root: it wires concrete
// adapter/infra implementations into use cases and hands control to the
// CLI. No business logic lives here.
//
// This binary deliberately does not import controlserver, stunserver, or
// adapter/repository (sqlite or memory) — those are server-only weight,
// wired instead in cmd/server. Keeping them out of this build is the
// whole point of the split: a client running "app connect" has no reason
// to link in a SQLite driver it will never open.
package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/fu1se/localizator/internal/adapter/cli"
	"github.com/fu1se/localizator/internal/adapter/controlclient"
	"github.com/fu1se/localizator/internal/domain"
	"github.com/fu1se/localizator/internal/infra"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	defaults, err := loadDefaults()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	root := cli.NewClientRootCommand(cli.ClientDependencies{
		Register:    register,
		Connect:     connect,
		Expose:      expose,
		Whoami:      whoami,
		JoinNetwork: joinNetwork,
		Join:        join,
	}, defaults)

	if err := root.ExecuteContext(ctx); err != nil {
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
	}, nil
}

// register dials serverAddr and registers an ephemeral identity. Real,
// persistent keypairs land in Phase 7 (see CLAUDE.md roadmap); for now a
// fresh random key is generated on every call, which is enough to exercise
// the control-plane wire protocol end to end.
func register(ctx context.Context, serverAddr string) (cli.RegisterResult, error) {
	var pub domain.PublicKey
	if _, err := rand.Read(pub[:]); err != nil {
		return cli.RegisterResult{}, fmt.Errorf("app: generate ephemeral key: %w", err)
	}

	tlsConf, err := controlClientTLS(serverAddr)
	if err != nil {
		return cli.RegisterResult{}, err
	}

	client, err := controlclient.Dial(ctx, serverAddr, tlsConf, infra.DefaultQUICConfig())
	if err != nil {
		return cli.RegisterResult{}, err
	}
	defer client.Close()

	result, err := client.Register(ctx, pub)
	if err != nil {
		return cli.RegisterResult{}, err
	}

	return cli.RegisterResult{
		PeerID:          string(result.PeerID),
		ObservedAddress: result.ObservedAddress,
	}, nil
}

// whoami loads (or creates) the local identity and returns its peer ID.
// Pure local operation, no network access — see resolveIdentityPath and
// rendezvous's doc comment for why this bootstrap step exists.
func whoami(identityPath string) (string, error) {
	resolvedIdentityPath, err := resolveIdentityPath(identityPath)
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
func joinNetwork(ctx context.Context, serverAddr, networkName, inviteToken, identityPath string) (cli.JoinNetworkResult, error) {
	resolvedIdentityPath, err := resolveIdentityPath(identityPath)
	if err != nil {
		return cli.JoinNetworkResult{}, err
	}
	id, err := infra.LoadOrCreateIdentity(resolvedIdentityPath)
	if err != nil {
		return cli.JoinNetworkResult{}, fmt.Errorf("app: load identity: %w", err)
	}

	tlsConf, err := controlClientTLS(serverAddr)
	if err != nil {
		return cli.JoinNetworkResult{}, err
	}

	client, err := controlclient.Dial(ctx, serverAddr, tlsConf, infra.DefaultQUICConfig())
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
