// Command server is the server composition root: it wires the SQLite-
// backed peer/network repositories, the in-memory candidate broker and
// relay broker, and their use cases into the control-plane QUIC server,
// then runs the STUN responder alongside it. No business logic lives
// here.
//
// This is a separate binary from cmd/app on purpose — see that package's
// doc comment and CLAUDE.md's "Разделение клиента и сервера": splitting
// the composition roots means the client build never links in the SQLite
// driver, controlserver, or the STUN responder it has no use for.
package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"golang.org/x/sync/errgroup"

	"github.com/fu1se/localizator/internal/adapter/cli"
	"github.com/fu1se/localizator/internal/adapter/controlproto"
	"github.com/fu1se/localizator/internal/adapter/controlserver"
	"github.com/fu1se/localizator/internal/adapter/repository/memory"
	"github.com/fu1se/localizator/internal/adapter/repository/sqlite"
	"github.com/fu1se/localizator/internal/adapter/stunserver"
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

	root := cli.NewServerRootCommand(cli.ServerDependencies{
		RunServer: runServer,
	}, defaults)

	if err := root.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}

// loadDefaults gives --db a sane default (infra.DefaultServerStatePath) —
// there is normally exactly one server, so unlike the client's config
// file there is nothing worth persisting across invocations here.
func loadDefaults() (cli.ServerDefaults, error) {
	statePath, err := infra.DefaultServerStatePath()
	if err != nil {
		return cli.ServerDefaults{}, err
	}
	return cli.ServerDefaults{State: statePath}, nil
}

// runServer wires the SQLite-backed peer/network repositories, the
// in-memory candidate broker and relay broker, and their use cases into
// the control-plane QUIC server, then runs the STUN responder alongside
// it. Peers and mesh networks survive a restart (adapter/repository/
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
		return fmt.Errorf("server: tls config: %w", err)
	}

	controlConn, err := net.ListenPacket("udp", controlAddr)
	if err != nil {
		return fmt.Errorf("server: bind control-plane: %w", err)
	}

	stunUDPAddr, err := net.ResolveUDPAddr("udp", stunAddr)
	if err != nil {
		return fmt.Errorf("server: resolve stun addr: %w", err)
	}
	stunConn, err := net.ListenUDP("udp", stunUDPAddr)
	if err != nil {
		return fmt.Errorf("server: bind stun: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return fmt.Errorf("server: create state dir: %w", err)
	}
	db, err := sqlite.Open(dbPath)
	if err != nil {
		return fmt.Errorf("server: open state db: %w", err)
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
