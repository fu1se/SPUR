package controlserver_test

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
	"github.com/fu1se/spur/internal/infra"
	"github.com/fu1se/spur/internal/usecase"
)

// blockingRelayBroker never resolves Join until ctx is cancelled -- used to
// hold a handleStream goroutine (and its concurrency-limit slot) open
// deterministically for the life of the test, instead of racing against
// RelayBroker's own real pairing timeout.
type blockingRelayBroker struct{}

func (blockingRelayBroker) Join(ctx context.Context, sessionID string, conn io.ReadWriteCloser) error {
	<-ctx.Done()
	return ctx.Err()
}

// readReturnsWithin reports whether a Read on r returns (with any result,
// including an error) within timeout.
func readReturnsWithin(r io.Reader, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		buf := make([]byte, 1)
		_, _ = r.Read(buf)
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// TestServer_LimitsConcurrentStreams is a DoS regression test found in a
// security audit: none of the control-protocol RPCs require prior
// authentication, and Relay in particular holds a goroutine open for a
// whole tunnel's lifetime once paired, so without a cap a client could
// flood the server with streams and exhaust its resources.
// MaxConcurrentStreams is set low here so the test doesn't need to
// actually open 1000+ real streams to exercise the limit.
func TestServer_LimitsConcurrentStreams(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tlsConf, err := infra.SelfSignedServerTLSConfig(controlproto.ALPN)
	require.NoError(t, err)

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := conn.LocalAddr().String()

	srv := &controlserver.Server{
		RelayFallback:        usecase.RelayFallback{Broker: blockingRelayBroker{}},
		MaxConcurrentStreams: 2,
	}
	go func() { _ = srv.Serve(ctx, conn, tlsConf, infra.DefaultQUICConfig()) }()

	client, err := controlclient.Dial(ctx, addr, infra.InsecureClientTLSConfig(controlproto.ALPN), infra.DefaultQUICConfig())
	require.NoError(t, err)
	defer client.Close()

	channel1, err := client.OpenChannel(ctx, "session-1")
	require.NoError(t, err)
	defer channel1.Close()
	channel2, err := client.OpenChannel(ctx, "session-2")
	require.NoError(t, err)
	defer channel2.Close()

	// Give the server a moment to actually dispatch and start blocking
	// (via blockingRelayBroker) on both streams above before the
	// over-limit third one is opened.
	time.Sleep(100 * time.Millisecond)

	channel3, err := client.OpenChannel(ctx, "session-3")
	require.NoError(t, err) // OpenChannel only opens the stream and writes a request; rejection happens after that, server-side
	defer channel3.Close()

	require.True(t, readReturnsWithin(channel3, 2*time.Second),
		"over-limit stream should have been closed by the server, not held open")

	require.False(t, readReturnsWithin(channel1, 300*time.Millisecond),
		"within-limit stream should still be genuinely held open by the server")
}
