package usecase

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/spur/internal/usecase/port"
)

// fakeSeqListener hands out pre-built connections one at a time, blocking
// (until ctx is done) once exhausted rather than erroring -- so Run keeps
// running and a test can observe exactly how many Accept calls actually
// happened.
type fakeSeqListener struct {
	mu       sync.Mutex
	conns    []net.Conn
	next     int
	accepted int
}

func (l *fakeSeqListener) Accept(ctx context.Context) (io.ReadWriteCloser, error) {
	l.mu.Lock()
	if l.next < len(l.conns) {
		c := l.conns[l.next]
		l.next++
		l.accepted++
		l.mu.Unlock()
		return c, nil
	}
	l.mu.Unlock()

	<-ctx.Done()
	return nil, ctx.Err()
}

func (l *fakeSeqListener) Close() error { return nil }

func (l *fakeSeqListener) acceptedCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.accepted
}

// fakeSeqTunnel hands out pre-built stream connections in lockstep with
// fakeSeqListener's Accept calls (ForwardPort.Run calls Accept then
// OpenStream once per iteration, so an index counter kept in step with
// Accept's is enough).
type fakeSeqTunnel struct {
	streams []net.Conn
	next    int
}

func (t *fakeSeqTunnel) OpenStream(context.Context) (port.Stream, error) {
	s := t.streams[t.next]
	t.next++
	return s, nil
}

func (t *fakeSeqTunnel) AcceptStream(context.Context) (port.Stream, error) {
	panic("not used by ForwardPort")
}

func (t *fakeSeqTunnel) Close() error { return nil }

// TestForwardPort_LimitsConcurrentTunnels is a DoS regression test: a
// counterpart on the other end of an established tunnel is never
// re-verified per forwarded connection, so without a concurrency bound it
// could force ForwardPort to spawn an unbounded number of goroutines/local
// connections and exhaust the process's file descriptors. Each piped
// connection here is left open with nothing written on either side, so
// pipe() blocks forever until the test explicitly closes one -- letting
// the test observe that Accept stalls at exactly maxConcurrentTunnels and
// resumes only after a slot frees up.
func TestForwardPort_LimitsConcurrentTunnels(t *testing.T) {
	const extra = 5
	n := maxConcurrentTunnels + extra

	locals := make([]net.Conn, n)
	remotes := make([]net.Conn, n)
	streams := make([]net.Conn, n)
	for i := range n {
		locals[i], remotes[i] = net.Pipe()
		streams[i], _ = net.Pipe()
	}

	listener := &fakeSeqListener{conns: locals}
	tunnel := &fakeSeqTunnel{streams: streams}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- (ForwardPort{Listener: listener, Tunnel: tunnel}).Run(ctx) }()

	require.Eventually(t, func() bool {
		return listener.acceptedCount() == maxConcurrentTunnels
	}, 2*time.Second, 5*time.Millisecond, "should accept exactly up to the concurrency limit")

	// Give Run a moment to (wrongly, if the bound were absent or broken)
	// accept more than the limit; it must not.
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, maxConcurrentTunnels, listener.acceptedCount())

	// Free one slot: closing the peer end makes both io.Copy directions in
	// that pipe() call return, freeing its semaphore slot.
	require.NoError(t, remotes[0].Close())

	require.Eventually(t, func() bool {
		return listener.acceptedCount() == maxConcurrentTunnels+1
	}, 2*time.Second, 5*time.Millisecond, "freeing one slot should let exactly one more through")

	cancel()
	<-errCh
}

// deadTunnel fails every OpenStream, the way a QUIC connection or yamux
// session that died under ForwardPort does.
type deadTunnel struct{ err error }

func (t *deadTunnel) OpenStream(context.Context) (port.Stream, error)   { return nil, t.err }
func (t *deadTunnel) AcceptStream(context.Context) (port.Stream, error) { return nil, t.err }
func (t *deadTunnel) Close() error                                      { return nil }

// TestForwardPort_ReturnsWhenTunnelDies pins the auto-reconnect
// contract: a failed OpenStream means the tunnel is gone, and Run must
// surface that instead of silently looping forever accepting local
// connections that go nowhere (the pre-reconnect behavior).
func TestForwardPort_ReturnsWhenTunnelDies(t *testing.T) {
	local, remote := net.Pipe()
	defer remote.Close()

	tunnelErr := io.ErrClosedPipe
	listener := &fakeSeqListener{conns: []net.Conn{local}}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := ForwardPort{Listener: listener, Tunnel: &deadTunnel{err: tunnelErr}}.Run(ctx)
	require.ErrorIs(t, err, tunnelErr)
	require.NoError(t, ctx.Err(), "must return from the tunnel error, not the test timeout")
}
