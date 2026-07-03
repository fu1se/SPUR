package controlclient_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/spur/internal/adapter/controlclient"
	"github.com/fu1se/spur/internal/adapter/controlproto"
	"github.com/fu1se/spur/internal/adapter/controlserver"
	"github.com/fu1se/spur/internal/adapter/repository/memory"
	"github.com/fu1se/spur/internal/domain"
	"github.com/fu1se/spur/internal/infra"
	"github.com/fu1se/spur/internal/usecase"
)

// TestPairingCode_EndToEnd exercises the full single-command connect flow
// against a real QUIC server: the host registers a pairing code and
// blocks in AwaitPairingCodeUse (same as cmd/spur's host-side flow would);
// a guest resolves that code to learn the host's real peer ID; the host's
// blocked call must unblock with the guest's peer ID.
func TestPairingCode_EndToEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	serverTLS, err := infra.SelfSignedServerTLSConfig(controlproto.ALPN)
	require.NoError(t, err)

	broker := memory.NewPairingCodeBroker()
	srv := &controlserver.Server{
		RegisterPairingCode: usecase.RegisterPairingCode{Store: broker},
		ResolvePairingCode:  usecase.ResolvePairingCode{Store: broker},
		AwaitPairingCodeUse: usecase.AwaitPairingCodeUse{Store: broker},
	}

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := conn.LocalAddr().String()
	go func() { _ = srv.Serve(ctx, conn, serverTLS, infra.DefaultQUICConfig()) }()

	hostClient, err := controlclient.Dial(ctx, addr, infra.InsecureClientTLSConfig(controlproto.ALPN), infra.DefaultQUICConfig())
	require.NoError(t, err)
	defer hostClient.Close()

	guestClient, err := controlclient.Dial(ctx, addr, infra.InsecureClientTLSConfig(controlproto.ALPN), infra.DefaultQUICConfig())
	require.NoError(t, err)
	defer guestClient.Close()

	var hostPub, guestPub domain.PublicKey
	hostPub[0], guestPub[0] = 0x01, 0x02
	hostPeer := domain.DerivePeerID(hostPub)
	guestPeer := domain.DerivePeerID(guestPub)

	code, err := hostClient.RegisterPairingCode(ctx, hostPub)
	require.NoError(t, err)
	require.Len(t, code, 6)

	awaitResult := make(chan domain.PeerID, 1)
	awaitErr := make(chan error, 1)
	go func() {
		guest, err := hostClient.AwaitPairingCodeUse(ctx, code)
		if err != nil {
			awaitErr <- err
			return
		}
		awaitResult <- guest
	}()

	time.Sleep(50 * time.Millisecond) // let AwaitPairingCodeUse actually start blocking server-side

	resolvedHost, err := guestClient.ResolvePairingCode(ctx, code, guestPub)
	require.NoError(t, err)
	require.Equal(t, hostPeer, resolvedHost)

	select {
	case guest := <-awaitResult:
		require.Equal(t, guestPeer, guest)
	case err := <-awaitErr:
		t.Fatalf("AwaitPairingCodeUse failed: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("AwaitPairingCodeUse should have unblocked once the guest resolved the code")
	}
}

func TestPairingCode_ResolveUnknownCodeReturnsError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	serverTLS, err := infra.SelfSignedServerTLSConfig(controlproto.ALPN)
	require.NoError(t, err)

	broker := memory.NewPairingCodeBroker()
	srv := &controlserver.Server{
		ResolvePairingCode: usecase.ResolvePairingCode{Store: broker},
	}

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := conn.LocalAddr().String()
	go func() { _ = srv.Serve(ctx, conn, serverTLS, infra.DefaultQUICConfig()) }()

	client, err := controlclient.Dial(ctx, addr, infra.InsecureClientTLSConfig(controlproto.ALPN), infra.DefaultQUICConfig())
	require.NoError(t, err)
	defer client.Close()

	var pub domain.PublicKey
	pub[0] = 0x03
	_, err = client.ResolvePairingCode(ctx, "NOPE99", pub)
	require.Error(t, err)
}
