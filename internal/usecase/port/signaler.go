package port

import (
	"context"

	"github.com/fu1se/localizator/internal/domain"
)

// Signaler exchanges NAT candidates between two peers through the
// rendezvous server's control channel. It never touches data-plane bytes.
type Signaler interface {
	// PublishCandidates sends this peer's candidates for the given session.
	PublishCandidates(ctx context.Context, sessionID string, self domain.PeerID, candidates []domain.Candidate) error
	// AwaitPeerCandidates blocks until the counterpart's candidates for the
	// session are available, or ctx is cancelled.
	AwaitPeerCandidates(ctx context.Context, sessionID string, peer domain.PeerID) ([]domain.Candidate, error)
}
