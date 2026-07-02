// Package memory provides in-memory port.* implementations used as the
// default wiring until persistent (SQLite) implementations replace them.
package memory

import (
	"context"
	"sync"

	"github.com/fu1se/spur/internal/domain"
)

// PeerRepository is a thread-safe in-memory implementation of
// port.PeerRepository.
type PeerRepository struct {
	mu    sync.RWMutex
	peers map[domain.PeerID]domain.Peer
}

func NewPeerRepository() *PeerRepository {
	return &PeerRepository{peers: make(map[domain.PeerID]domain.Peer)}
}

func (r *PeerRepository) Save(_ context.Context, peer domain.Peer) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.peers[peer.ID] = peer
	return nil
}

func (r *PeerRepository) FindByID(_ context.Context, id domain.PeerID) (domain.Peer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	peer, ok := r.peers[id]
	if !ok {
		return domain.Peer{}, domain.ErrPeerNotFound
	}
	return peer, nil
}

// ListByNetwork is not yet meaningful: peer-to-network membership is
// introduced in the mesh VPN phase (CLAUDE.md roadmap, Phase 6).
func (r *PeerRepository) ListByNetwork(_ context.Context, _ string) ([]domain.Peer, error) {
	return nil, nil
}
