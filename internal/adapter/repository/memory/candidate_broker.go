package memory

import (
	"context"
	"sync"
	"time"

	"github.com/fu1se/spur/internal/domain"
)

type candidateKey struct {
	sessionID string
	peer      domain.PeerID
}

// candidateTTL bounds how long an unconsumed entry can sit in subs. Without
// it, any client — no authentication is required to call PublishCandidates
// or AwaitCandidates — could grow the map without bound forever by calling
// either RPC with fresh random session IDs, since neither a Wait that never
// sees a matching Put nor a Put nobody ever Waits for used to be cleaned up
// on their own. Set comfortably above awaitCandidatesTimeout (60s, see
// controlserver.awaitCandidatesTimeout) so a legitimately still-blocked
// Wait is never pruned out from under itself.
const candidateTTL = 90 * time.Second

type candidateEntry struct {
	ch        chan domain.CandidateSet
	createdAt time.Time
}

// CandidateBroker is a thread-safe in-memory implementation of
// port.CandidateStore: a one-shot rendezvous point per (session, peer).
// Put is non-blocking; Wait blocks until a matching Put happens or ctx is
// done. Whichever call arrives first for a given key waits for the other.
type CandidateBroker struct {
	mu   sync.Mutex
	subs map[candidateKey]*candidateEntry
}

func NewCandidateBroker() *CandidateBroker {
	return &CandidateBroker{subs: make(map[candidateKey]*candidateEntry)}
}

func (b *CandidateBroker) entry(key candidateKey) *candidateEntry {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.pruneExpiredLocked()

	e, ok := b.subs[key]
	if !ok {
		e = &candidateEntry{ch: make(chan domain.CandidateSet, 1), createdAt: time.Now()}
		b.subs[key] = e
	}
	return e
}

// pruneExpiredLocked removes entries older than candidateTTL. Called
// opportunistically from entry() rather than on a timer, since
// PublishCandidates/AwaitCandidates are called often enough in practice to
// keep the map bounded without a background goroutine to manage.
func (b *CandidateBroker) pruneExpiredLocked() {
	now := time.Now()
	for k, e := range b.subs {
		if now.Sub(e.createdAt) > candidateTTL {
			delete(b.subs, k)
		}
	}
}

func (b *CandidateBroker) delete(key candidateKey) {
	b.mu.Lock()
	delete(b.subs, key)
	b.mu.Unlock()
}

// Put replaces whatever candidate set is currently buffered for this
// key, rather than rejecting the call outright when one is already
// there. domain.SessionIDFor is a pure function of the peer pair, not of
// "attempt number" or time — a retrying caller (usecase.JoinNetwork's
// periodic ConnectToNewMembers loop is the main one, but any repeated
// connect/expose/send/receive against the same counterpart hits this
// too) reuses the exact same session ID on every retry. The original
// "reject if already published" behavior turned that into a
// self-inflicted deadlock: if side A's first Put is never consumed
// (side B hasn't called Wait yet, or already timed out), A's own next
// retry publishing under the very same key would fail with "already
// published" and A would never get past PublishCandidates again for
// this session — regardless of whether B was actually reachable. Found
// live: two genuinely running, continuously-retrying `spur join`
// processes (real desktop peer plus a diagnostic identity on this
// sandbox) that never once succeeded in over two hours, each stuck
// re-colliding with its own leftover unconsumed publish faster than
// candidateTTL could prune it. Overwriting instead means the most
// recent retry's candidates are always what a Wait would see — correct
// for the retry use case (older candidate/port pairs from a previous
// attempt are stale anyway) and harmless for the one-shot case (a
// single connect/expose call only ever Puts once).
func (b *CandidateBroker) Put(_ context.Context, sessionID string, peer domain.PeerID, set domain.CandidateSet) error {
	e := b.entry(candidateKey{sessionID, peer})
	for {
		select {
		case e.ch <- set:
			return nil
		default:
			select {
			case <-e.ch: // drain the stale value, then retry the send above
			default:
			}
		}
	}
}

func (b *CandidateBroker) Wait(ctx context.Context, sessionID string, peer domain.PeerID) (domain.CandidateSet, error) {
	key := candidateKey{sessionID, peer}
	e := b.entry(key)
	select {
	case set := <-e.ch:
		b.delete(key) // consumed: free it immediately rather than waiting for the TTL sweep
		return set, nil
	case <-ctx.Done():
		b.delete(key) // abandoned: don't let a timed-out/cancelled wait leak forever
		return domain.CandidateSet{}, ctx.Err()
	}
}
