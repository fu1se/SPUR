package memory

import (
	"context"
	"sync"
	"time"

	"github.com/fu1se/spur/internal/domain"
)

type pairingCodeEntry struct {
	host      domain.PeerID
	expiresAt time.Time
	used      chan domain.PeerID // buffered 1: first Resolve wakes AwaitUse
}

// PairingCodeBroker is a thread-safe in-memory implementation of
// port.PairingCodeStore. Entries are pruned opportunistically (same
// pattern as CandidateBroker — see that type's doc comment for why an
// opportunistic sweep on every call is enough without a background
// goroutine) so an abandoned code (registered, never resolved) doesn't
// grow the map forever.
type PairingCodeBroker struct {
	mu    sync.Mutex
	codes map[string]*pairingCodeEntry
}

func NewPairingCodeBroker() *PairingCodeBroker {
	return &PairingCodeBroker{codes: make(map[string]*pairingCodeEntry)}
}

func (b *PairingCodeBroker) pruneExpiredLocked() {
	now := time.Now()
	for k, e := range b.codes {
		if now.After(e.expiresAt) {
			delete(b.codes, k)
		}
	}
}

func (b *PairingCodeBroker) Register(_ context.Context, code string, host domain.PeerID, ttl time.Duration) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pruneExpiredLocked()
	b.codes[code] = &pairingCodeEntry{
		host:      host,
		expiresAt: time.Now().Add(ttl),
		used:      make(chan domain.PeerID, 1),
	}
	return nil
}

func (b *PairingCodeBroker) Resolve(_ context.Context, code string, guest domain.PeerID) (domain.PeerID, error) {
	b.mu.Lock()
	b.pruneExpiredLocked()
	e, ok := b.codes[code]
	b.mu.Unlock()
	if !ok {
		return "", domain.ErrPairingCodeNotFound
	}

	select {
	case e.used <- guest:
	default:
		// Already resolved once — still a valid lookup (the host's peer
		// ID hasn't changed), just don't block: only the first resolve
		// wakes a waiting AwaitUse, matching "one code, one connection
		// attempt" as the expected case. A retry after a failed
		// connection attempt still gets a correct answer here even
		// though it won't wake AwaitUse a second time.
	}
	return e.host, nil
}

func (b *PairingCodeBroker) AwaitUse(ctx context.Context, code string) (domain.PeerID, error) {
	b.mu.Lock()
	b.pruneExpiredLocked()
	e, ok := b.codes[code]
	b.mu.Unlock()
	if !ok {
		return "", domain.ErrPairingCodeNotFound
	}

	select {
	case guest := <-e.used:
		return guest, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
