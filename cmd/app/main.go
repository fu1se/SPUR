// Command app is the composition root: it wires concrete adapter/infra
// implementations into use cases and hands control to the CLI. No business
// logic lives here.
package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"golang.org/x/sync/errgroup"

	"github.com/fu1se/localizator/internal/adapter/cli"
	"github.com/fu1se/localizator/internal/adapter/controlclient"
	"github.com/fu1se/localizator/internal/adapter/controlproto"
	"github.com/fu1se/localizator/internal/adapter/controlserver"
	"github.com/fu1se/localizator/internal/adapter/repository/memory"
	"github.com/fu1se/localizator/internal/adapter/repository/sqlite"
	"github.com/fu1se/localizator/internal/adapter/stunserver"
	"github.com/fu1se/localizator/internal/domain"
	"github.com/fu1se/localizator/internal/infra"
	"github.com/fu1se/localizator/internal/usecase"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	defaults, err := loadDefaults()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	root := cli.NewRootCommand(cli.Dependencies{
		RunServer:   runServer,
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
// comment) into cli.Defaults. A missing file just means every flag keeps
// its original empty default — the config file is purely additive.
// ServerState isn't read from the config file (there's normally exactly
// one server, so there's little to save by not retyping --db); it's just
// infra.DefaultServerStatePath() so --db has a sane default too.
func loadDefaults() (cli.Defaults, error) {
	path, err := infra.DefaultConfigPath()
	if err != nil {
		return cli.Defaults{}, err
	}
	cfg, err := infra.LoadConfig(path)
	if err != nil {
		return cli.Defaults{}, err
	}
	statePath, err := infra.DefaultServerStatePath()
	if err != nil {
		return cli.Defaults{}, err
	}
	return cli.Defaults{
		Server:      cfg.Server,
		StunServer:  cfg.StunServer,
		Identity:    cfg.Identity,
		ServerState: statePath,
	}, nil
}

// runServer wires the SQLite-backed peer/network repositories, the
// in-memory candidate broker and relay broker, and their use cases into
// the control-plane QUIC server, then runs the STUN responder alongside
// it. Peers and mesh networks now survive a restart (adapter/repository/
// sqlite); candidates and relay splices stay in-memory because they are
// inherently short-lived, in-flight coordination state with nothing
// meaningful to persist (see that package's doc comment). Control-plane
// and STUN run on separate UDP ports (see stunserver's package doc for
// why); both are bound up front so a failure to bind either port surfaces
// immediately instead of racing the accept loop. verbose is threaded into
// the server's zerolog.Logger — every request handler used to drop its
// errors silently, leaving an operator with zero visibility.
func runServer(ctx context.Context, controlAddr, stunAddr, dbPath string, verbose bool) error {
	logger := infra.NewLogger(verbose)

	certPath, err := infra.DefaultServerCertPath()
	if err != nil {
		return err
	}
	tlsConf, err := infra.LoadOrCreateServerTLSConfig(certPath, controlproto.ALPN)
	if err != nil {
		return fmt.Errorf("app: tls config: %w", err)
	}

	controlConn, err := net.ListenPacket("udp", controlAddr)
	if err != nil {
		return fmt.Errorf("app: bind control-plane: %w", err)
	}

	stunUDPAddr, err := net.ResolveUDPAddr("udp", stunAddr)
	if err != nil {
		return fmt.Errorf("app: resolve stun addr: %w", err)
	}
	stunConn, err := net.ListenUDP("udp", stunUDPAddr)
	if err != nil {
		return fmt.Errorf("app: bind stun: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return fmt.Errorf("app: create state dir: %w", err)
	}
	db, err := sqlite.Open(dbPath)
	if err != nil {
		return fmt.Errorf("app: open state db: %w", err)
	}
	defer db.Close()

	peers := sqlite.NewPeerRepository(db)
	networks := sqlite.NewNetworkRepository(db)
	candidateBroker := memory.NewCandidateBroker()
	relayBroker := memory.NewRelayBroker()
	srv := &controlserver.Server{
		RegisterPeer:      usecase.RegisterPeer{Peers: peers},
		PublishCandidates: usecase.PublishCandidates{Store: candidateBroker},
		AwaitCandidates:   usecase.AwaitCandidates{Store: candidateBroker},
		RelayFallback:     usecase.RelayFallback{Broker: relayBroker},
		JoinNetwork:       usecase.JoinNetwork{Networks: networks},
		Logger:            &logger,
	}

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error { return srv.Serve(gctx, controlConn, tlsConf, infra.DefaultQUICConfig()) })
	g.Go(func() error { return stunserver.Serve(gctx, stunConn) })
	return g.Wait()
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
// only — see cli.Dependencies.JoinNetwork's doc comment.
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
