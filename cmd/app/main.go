// Command app is the composition root: it wires concrete adapter/infra
// implementations into use cases and hands control to the CLI. No business
// logic lives here.
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
	"github.com/fu1se/localizator/internal/adapter/controlproto"
	"github.com/fu1se/localizator/internal/adapter/controlserver"
	"github.com/fu1se/localizator/internal/adapter/repository/memory"
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

// runServer wires the in-memory peer repository and the RegisterPeer use
// case into the control-plane QUIC server. The in-memory repository is
// deliberately temporary — see CLAUDE.md's adapter/repository/memory note.
func runServer(ctx context.Context, listenAddr string) error {
	tlsConf, err := infra.SelfSignedServerTLSConfig(controlproto.ALPN)
	if err != nil {
		return fmt.Errorf("app: tls config: %w", err)
	}

	peers := memory.NewPeerRepository()
	srv := &controlserver.Server{
		RegisterPeer: usecase.RegisterPeer{Peers: peers},
	}

	return srv.Serve(ctx, listenAddr, tlsConf)
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
