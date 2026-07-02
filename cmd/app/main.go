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
	"syscall"

	"golang.org/x/sync/errgroup"

	"github.com/fu1se/localizator/internal/adapter/cli"
	"github.com/fu1se/localizator/internal/adapter/controlclient"
	"github.com/fu1se/localizator/internal/adapter/controlproto"
	"github.com/fu1se/localizator/internal/adapter/controlserver"
	"github.com/fu1se/localizator/internal/adapter/repository/memory"
	"github.com/fu1se/localizator/internal/adapter/stunserver"
	"github.com/fu1se/localizator/internal/domain"
	"github.com/fu1se/localizator/internal/infra"
	"github.com/fu1se/localizator/internal/usecase"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root := cli.NewRootCommand(cli.Dependencies{
		RunServer: runServer,
		Register:  register,
	})

	if err := root.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}

// runServer wires the in-memory peer repository, candidate broker and their
// use cases into the control-plane QUIC server, and runs the STUN
// responder alongside it. The in-memory implementations are deliberately
// temporary — see CLAUDE.md's adapter/repository/memory note. Control-plane
// and STUN run on separate UDP ports (see stunserver's package doc for
// why); both are bound up front so a failure to bind either port surfaces
// immediately instead of racing the accept loop.
func runServer(ctx context.Context, controlAddr, stunAddr string) error {
	tlsConf, err := infra.SelfSignedServerTLSConfig(controlproto.ALPN)
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

	peers := memory.NewPeerRepository()
	candidateBroker := memory.NewCandidateBroker()
	relayBroker := memory.NewRelayBroker()
	srv := &controlserver.Server{
		RegisterPeer:      usecase.RegisterPeer{Peers: peers},
		PublishCandidates: usecase.PublishCandidates{Store: candidateBroker},
		AwaitCandidates:   usecase.AwaitCandidates{Store: candidateBroker},
		RelayFallback:     usecase.RelayFallback{Broker: relayBroker},
	}

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error { return srv.Serve(gctx, controlConn, tlsConf) })
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

	tlsConf := infra.InsecureClientTLSConfig(controlproto.ALPN)

	client, err := controlclient.Dial(ctx, serverAddr, tlsConf)
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
