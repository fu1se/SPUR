package meshclient_test

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/spur/internal/adapter/controlproto"
	"github.com/fu1se/spur/internal/adapter/controlserver"
	"github.com/fu1se/spur/internal/adapter/meshclient"
	"github.com/fu1se/spur/internal/adapter/repository/memory"
	"github.com/fu1se/spur/internal/infra"
	"github.com/fu1se/spur/internal/usecase"
)

// TestMembership_SurvivesServerRestart is the auto-reconnect regression
// test for the mesh control plane: before Membership existed, both mesh
// loops kept polling JoinNetwork on the QUIC connection they dialed at
// startup — once that connection died (network drop, server restart),
// every subsequent poll failed on the same dead connection forever.
// Fetch must re-dial and succeed once the server is back.
func TestMembership_SurvivesServerRestart(t *testing.T) {
	// Isolate the TOFU trust store (see guiapp's e2e test for the same
	// pattern) — and note the restarted server below reuses the SAME TLS
	// config, the way a real spur-server's persistent certificate
	// survives restarts; a fresh self-signed cert would (correctly) trip
	// TOFU pinning, which isn't what this test is about.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tlsConf, err := infra.SelfSignedServerTLSConfig(controlproto.ALPN)
	require.NoError(t, err)

	networks := memory.NewNetworkRepository()
	newServer := func() *controlserver.Server {
		return &controlserver.Server{
			RegisterPeer: usecase.RegisterPeer{Peers: memory.NewPeerRepository()},
			JoinNetwork:  usecase.JoinNetwork{Networks: networks},
		}
	}

	conn1, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	serverAddr := conn1.LocalAddr().String()

	srv1Ctx, stopSrv1 := context.WithCancel(ctx)
	srv1Done := make(chan struct{})
	go func() {
		defer close(srv1Done)
		_ = newServer().Serve(srv1Ctx, conn1, tlsConf, infra.DefaultQUICConfig())
	}()

	membership := &meshclient.Membership{
		ServerAddr:    serverAddr,
		IdentityPath:  filepath.Join(t.TempDir(), "identity.key"),
		ClientVersion: "test",
		NetworkName:   "restart-net",
	}
	defer membership.Close()

	network, err := membership.Fetch(ctx)
	require.NoError(t, err)
	require.Len(t, network.Members, 1)

	// Kill the first server incarnation and free its port.
	stopSrv1()
	conn1.Close()
	<-srv1Done

	// The poll against the dead connection fails — that's expected; what
	// matters is that Membership drops the connection instead of keeping
	// it. Membership is documented as not safe for concurrent use, so
	// this polls in a plain sequential loop (testify's Eventually runs
	// its condition in fresh goroutines that can overlap a slow Fetch).
	sawFailure := false
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		fetchCtx, cancelFetch := context.WithTimeout(ctx, 2*time.Second)
		_, err := membership.Fetch(fetchCtx)
		cancelFetch()
		if err != nil {
			sawFailure = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.True(t, sawFailure, "a poll after server death must eventually fail")

	// Restart the server on the same address with the same TLS identity.
	conn2, err := net.ListenPacket("udp", serverAddr)
	require.NoError(t, err)
	go func() { _ = newServer().Serve(ctx, conn2, tlsConf, infra.DefaultQUICConfig()) }()

	// Fetch must recover by re-dialing — the old behavior (polling the
	// original dead connection) would fail here forever.
	recovered := false
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); {
		fetchCtx, cancelFetch := context.WithTimeout(ctx, 3*time.Second)
		network, err := membership.Fetch(fetchCtx)
		cancelFetch()
		if err == nil && len(network.Members) == 1 {
			recovered = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	require.True(t, recovered, "Fetch must re-dial and succeed once the server is back")
}
