package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/fu1se/localizator/internal/domain"
)

type candidateKey struct {
	sessionID string
	peer      domain.PeerID
}

// CandidateBroker is a thread-safe in-memory implementation of
// port.CandidateStore: a one-shot rendezvous point per (session, peer).
// Put is non-blocking; Wait blocks until a matching Put happens or ctx is
// done. Whichever call arrives first for a given key waits for the other.
type CandidateBroker struct {
	mu   sync.Mutex
	subs map[candidateKey]chan domain.CandidateSet
}

func NewCandidateBroker() *CandidateBroker {
	return &CandidateBroker{subs: make(map[candidateKey]chan domain.CandidateSet)}
}

func (b *CandidateBroker) channel(key candidateKey) chan domain.CandidateSet {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch, ok := b.subs[key]
	if !ok {
		ch = make(chan domain.CandidateSet, 1)
		b.subs[key] = ch
	}
	return ch
}

func (b *CandidateBroker) Put(_ context.Context, sessionID string, peer domain.PeerID, set domain.CandidateSet) error {
	ch := b.channel(candidateKey{sessionID, peer})
	select {
	case ch <- set:
		return nil
	default:
		return fmt.Errorf("memory: candidates for session %s peer %s already published", sessionID, peer)
	}
}

func (b *CandidateBroker) Wait(ctx context.Context, sessionID string, peer domain.PeerID) (domain.CandidateSet, error) {
	ch := b.channel(candidateKey{sessionID, peer})
	select {
	case set := <-ch:
		return set, nil
	case <-ctx.Done():
		return domain.CandidateSet{}, ctx.Err()
	}
}
