package memory

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/spur/internal/domain"
)

// TestCandidateBroker_PutOverwritesUnconsumedEntry guards against a real
// deadlock found live: domain.SessionIDFor is a pure function of the peer
// pair, not of attempt number, so a retrying caller (usecase.JoinNetwork's
// periodic ConnectToNewMembers loop is the main one) reuses the exact same
// session ID on every retry. Put used to reject a second call for a key
// whose first value was never consumed ("already published") -- meaning a
// caller whose own first Put nobody ever Waited for would find every
// subsequent retry failing at that same step forever, regardless of
// whether the counterpart was actually reachable. Confirmed live: two
// genuinely running, continuously-retrying `spur join` processes never
// once succeeded rendezvousing with each other over more than two hours,
// each stuck re-colliding with its own leftover unconsumed publish faster
// than candidateTTL could prune it away.
func TestCandidateBroker_PutOverwritesUnconsumedEntry(t *testing.T) {
	b := NewCandidateBroker()
	first := domain.CandidateSet{PublicKey: domain.PublicKey{1}}
	second := domain.CandidateSet{PublicKey: domain.PublicKey{2}}

	require.NoError(t, b.Put(context.Background(), "session", "peer", first))
	require.NoError(t, b.Put(context.Background(), "session", "peer", second),
		"a retry publishing under the same session/peer key must not fail")

	got, err := b.Wait(context.Background(), "session", "peer")
	require.NoError(t, err)
	require.Equal(t, second, got, "Wait should see the most recent Put, not a stale first one")
}

func TestCandidateBroker_PutThenWaitRoundTrips(t *testing.T) {
	b := NewCandidateBroker()
	set := domain.CandidateSet{PublicKey: domain.PublicKey{1, 2, 3}}

	require.NoError(t, b.Put(context.Background(), "session", "peer", set))

	got, err := b.Wait(context.Background(), "session", "peer")
	require.NoError(t, err)
	require.Equal(t, set, got)
}

// TestCandidateBroker_AbandonedWaitDoesNotLeak guards against the DoS gap
// found in a security audit: neither Put nor Wait used to clean up their
// map entry on their own, so any client -- no authentication is required
// to call either RPC -- could grow the broker's map without bound forever
// using fresh random session IDs. A Wait whose context is cancelled before
// any matching Put arrives must remove its own entry immediately, not wait
// for a background sweep.
func TestCandidateBroker_AbandonedWaitDoesNotLeak(t *testing.T) {
	b := NewCandidateBroker()

	for i := range 50 {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		_, err := b.Wait(ctx, "session", domain.PeerID(string(rune('a'+i))))
		cancel()
		require.Error(t, err)
	}

	b.mu.Lock()
	n := len(b.subs)
	b.mu.Unlock()
	require.Zero(t, n, "abandoned Wait calls should not leave entries behind")
}

// TestCandidateBroker_ConsumedEntryIsRemoved checks that a successful Put
// + Wait pairing frees its map entry right away instead of lingering until
// the TTL sweep.
func TestCandidateBroker_ConsumedEntryIsRemoved(t *testing.T) {
	b := NewCandidateBroker()
	require.NoError(t, b.Put(context.Background(), "session", "peer", domain.CandidateSet{}))
	_, err := b.Wait(context.Background(), "session", "peer")
	require.NoError(t, err)

	b.mu.Lock()
	n := len(b.subs)
	b.mu.Unlock()
	require.Zero(t, n)
}

// TestCandidateBroker_PrunesExpiredEntries checks the TTL-based sweep that
// backstops the case a Put's counterpart never calls Wait at all (so
// there's no Wait call whose ctx.Done() could trigger cleanup).
func TestCandidateBroker_PrunesExpiredEntries(t *testing.T) {
	b := NewCandidateBroker()
	require.NoError(t, b.Put(context.Background(), "orphaned-session", "peer", domain.CandidateSet{}))

	b.mu.Lock()
	e, ok := b.subs[candidateKey{"orphaned-session", "peer"}]
	require.True(t, ok)
	e.createdAt = time.Now().Add(-candidateTTL - time.Second) // backdate, simulating an old orphaned entry
	b.mu.Unlock()

	// Any call into entry() opportunistically sweeps expired entries.
	require.NoError(t, b.Put(context.Background(), "other-session", "peer", domain.CandidateSet{}))

	b.mu.Lock()
	_, stillThere := b.subs[candidateKey{"orphaned-session", "peer"}]
	b.mu.Unlock()
	require.False(t, stillThere, "expired entry should have been pruned")
}
