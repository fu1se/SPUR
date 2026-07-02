package memory

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRelayBroker_JoinPairsAndSplices(t *testing.T) {
	b := NewRelayBroker()

	aLocal, aRemote := net.Pipe()
	bLocal, bRemote := net.Pipe()

	errCh := make(chan error, 2)
	go func() { errCh <- b.Join(context.Background(), "session", aRemote) }()

	// Give the first Join call time to register as the waiting side before
	// the second call claims the pairing.
	time.Sleep(20 * time.Millisecond)
	go func() { errCh <- b.Join(context.Background(), "session", bRemote) }()

	const msg = "hello"
	_, err := aLocal.Write([]byte(msg))
	require.NoError(t, err)

	buf := make([]byte, len(msg))
	_, err = io.ReadFull(bLocal, buf)
	require.NoError(t, err)
	require.Equal(t, msg, string(buf))

	aLocal.Close()
	bLocal.Close()

	require.NoError(t, <-errCh)
	require.NoError(t, <-errCh)
}

// TestRelayBroker_AbandonedWaitDoesNotLeak guards against the DoS/hang gap
// found in a security audit: Join used to block the first caller forever
// with no timeout if no counterpart ever showed up (a real scenario, not
// just an attack -- hole-punch outcomes can be asymmetric, so one side can
// resolve P2P while the other falls back to relay and waits for a
// counterpart that will never call OpenChannel), and it never cleaned up
// its waiting/done map entries on abandonment either.
func TestRelayBroker_AbandonedWaitDoesNotLeak(t *testing.T) {
	b := NewRelayBroker()
	_, remote := net.Pipe()
	defer remote.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := b.Join(ctx, "lonely-session", remote)
	require.Error(t, err)

	b.mu.Lock()
	_, waitingLeft := b.waiting["lonely-session"]
	_, doneLeft := b.done["lonely-session"]
	b.mu.Unlock()
	require.False(t, waitingLeft)
	require.False(t, doneLeft)
}

// TestRelayBroker_PairingTimeoutDoesNotHangForever checks that Join
// returns on its own once the broker's pairing timeout elapses even given
// a context with no deadline of its own -- the real-world shape of the bug
// this guards against: the server's root context lives for the whole
// process, so without an internal timeout a lone client (real scenario:
// asymmetric hole-punch outcomes, not just an attack) would hang forever
// with zero feedback. Uses an injected short timeout instead of waiting
// out the real 60s constant.
func TestRelayBroker_PairingTimeoutDoesNotHangForever(t *testing.T) {
	b := NewRelayBroker()
	b.pairingTimeout = 5 * time.Millisecond

	_, remote := net.Pipe()
	defer remote.Close()

	err := b.Join(context.Background(), "short-session", remote)
	require.Error(t, err)

	b.mu.Lock()
	_, waitingLeft := b.waiting["short-session"]
	b.mu.Unlock()
	require.False(t, waitingLeft)
}
