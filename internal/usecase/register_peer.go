// Package usecase holds the Application Business Rules layer. Use cases
// depend only on internal/domain and internal/usecase/port — never on a
// concrete adapter, infra package, or third-party library.
package usecase

import (
	"context"
	"time"

	"github.com/fu1se/spur/internal/domain"
	"github.com/fu1se/spur/internal/usecase/port"
)

// RegisterPeer records a peer's identity together with its
// server-observed address, so that other peers can later discover it
// during candidate exchange (Phase 3).
type RegisterPeer struct {
	Peers port.PeerRepository
}

// Execute derives the peer's canonical ID from its public key, stores the
// server-reflexive candidate the caller observed for it, and persists the
// result.
func (uc RegisterPeer) Execute(ctx context.Context, pub domain.PublicKey, observed domain.Candidate) (domain.Peer, error) {
	peer := domain.NewPeer(domain.DerivePeerID(pub), pub)
	peer.Candidates = []domain.Candidate{observed}
	peer.LastSeenAt = time.Now()

	if err := uc.Peers.Save(ctx, peer); err != nil {
		return domain.Peer{}, err
	}
	return peer, nil
}
