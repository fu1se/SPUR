package controlclient_test

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/spur/internal/adapter/controlclient"
	"github.com/fu1se/spur/internal/adapter/controlproto"
	"github.com/fu1se/spur/internal/adapter/controlserver"
	"github.com/fu1se/spur/internal/adapter/repository/memory"
	"github.com/fu1se/spur/internal/infra"
	"github.com/fu1se/spur/internal/usecase"
)

// TestRelay_SplicesTwoPeers verifies that two independent control
// connections opening a relay channel for the same session ID get spliced
// together by the server: bytes written on one side arrive on the other,
// in both directions, and closing one side ends the other's read.
func TestRelay_SplicesTwoPeers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tlsConf, err := infra.SelfSignedServerTLSConfig(controlproto.ALPN)
	require.NoError(t, err)

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := conn.LocalAddr().String()

	relayBroker := memory.NewRelayBroker()
	srv := &controlserver.Server{RelayFallback: usecase.RelayFallback{Broker: relayBroker}}

	go func() { _ = srv.Serve(ctx, conn, tlsConf, infra.DefaultQUICConfig()) }()

	clientA, err := controlclient.Dial(ctx, addr, infra.InsecureClientTLSConfig(controlproto.ALPN), infra.DefaultQUICConfig())
	require.NoError(t, err)
	defer clientA.Close()

	clientB, err := controlclient.Dial(ctx, addr, infra.InsecureClientTLSConfig(controlproto.ALPN), infra.DefaultQUICConfig())
	require.NoError(t, err)
	defer clientB.Close()

	const sessionID = "test-session"

	channelA, err := clientA.OpenChannel(ctx, sessionID)
	require.NoError(t, err)
	defer channelA.Close()

	channelB, err := clientB.OpenChannel(ctx, sessionID)
	require.NoError(t, err)
	defer channelB.Close()

	_, err = channelA.Write([]byte("hello from A"))
	require.NoError(t, err)

	buf := make([]byte, 64)
	n, err := io.ReadFull(channelB, buf[:len("hello from A")])
	require.NoError(t, err)
	require.Equal(t, "hello from A", string(buf[:n]))

	_, err = channelB.Write([]byte("hello from B"))
	require.NoError(t, err)

	n, err = io.ReadFull(channelA, buf[:len("hello from B")])
	require.NoError(t, err)
	require.Equal(t, "hello from B", string(buf[:n]))

	require.NoError(t, channelA.Close())

	n, err = channelB.Read(buf)
	require.Zero(t, n)
	require.ErrorIs(t, err, io.EOF)
}
