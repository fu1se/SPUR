package memory

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/spur/internal/domain"
)

func TestPairingCodeBroker_RegisterThenResolveRoundTrips(t *testing.T) {
	b := NewPairingCodeBroker()
	require.NoError(t, b.Register(context.Background(), "ABC123", "host-peer", time.Minute))

	got, err := b.Resolve(context.Background(), "ABC123", "guest-peer")
	require.NoError(t, err)
	require.Equal(t, domain.PeerID("host-peer"), got)
}

func TestPairingCodeBroker_ResolveUnknownCodeFails(t *testing.T) {
	b := NewPairingCodeBroker()
	_, err := b.Resolve(context.Background(), "NOPE99", "guest-peer")
	require.ErrorIs(t, err, domain.ErrPairingCodeNotFound)
}

func TestPairingCodeBroker_AwaitUseUnblocksOnResolve(t *testing.T) {
	b := NewPairingCodeBroker()
	require.NoError(t, b.Register(context.Background(), "ABC123", "host-peer", time.Minute))

	resultCh := make(chan domain.PeerID, 1)
	go func() {
		guest, err := b.AwaitUse(context.Background(), "ABC123")
		require.NoError(t, err)
		resultCh <- guest
	}()

	time.Sleep(20 * time.Millisecond)
	_, err := b.Resolve(context.Background(), "ABC123", "guest-peer")
	require.NoError(t, err)

	select {
	case guest := <-resultCh:
		require.Equal(t, domain.PeerID("guest-peer"), guest)
	case <-time.After(time.Second):
		t.Fatal("AwaitUse should have unblocked")
	}
}

func TestPairingCodeBroker_AwaitUseUnknownCodeFailsImmediately(t *testing.T) {
	b := NewPairingCodeBroker()
	_, err := b.AwaitUse(context.Background(), "NOPE99")
	require.ErrorIs(t, err, domain.ErrPairingCodeNotFound)
}

func TestPairingCodeBroker_AwaitUseCancelledContext(t *testing.T) {
	b := NewPairingCodeBroker()
	require.NoError(t, b.Register(context.Background(), "ABC123", "host-peer", time.Minute))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := b.AwaitUse(ctx, "ABC123")
	require.Error(t, err)
}

// TestPairingCodeBroker_ExpiredEntriesArePruned guards against unbounded
// growth from abandoned codes (registered, never resolved, no auth
// required to call Register) — same pattern as CandidateBroker's TTL
// sweep.
func TestPairingCodeBroker_ExpiredEntriesArePruned(t *testing.T) {
	b := NewPairingCodeBroker()
	require.NoError(t, b.Register(context.Background(), "OLD001", "host-peer", time.Minute))

	b.mu.Lock()
	e, ok := b.codes["OLD001"]
	require.True(t, ok)
	e.expiresAt = time.Now().Add(-time.Second) // backdate, simulating an expired entry
	b.mu.Unlock()

	// Any call opportunistically sweeps expired entries.
	require.NoError(t, b.Register(context.Background(), "NEW002", "host-peer", time.Minute))

	b.mu.Lock()
	_, stillThere := b.codes["OLD001"]
	n := len(b.codes)
	b.mu.Unlock()
	require.False(t, stillThere, "expired entry should have been pruned")
	require.Equal(t, 1, n)
}

func TestPairingCodeBroker_ResolveAfterExpiryFails(t *testing.T) {
	b := NewPairingCodeBroker()
	require.NoError(t, b.Register(context.Background(), "ABC123", "host-peer", time.Millisecond))
	time.Sleep(10 * time.Millisecond)

	_, err := b.Resolve(context.Background(), "ABC123", "guest-peer")
	require.ErrorIs(t, err, domain.ErrPairingCodeNotFound)
}
