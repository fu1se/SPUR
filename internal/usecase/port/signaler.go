package port

import (
	"context"

	"github.com/fu1se/localizator/internal/domain"
)

// Signaler exchanges NAT candidates (and identity keys, see
// domain.CandidateSet) between two peers through the rendezvous server's
// control channel. It never touches data-plane bytes.
type Signaler interface {
	// PublishCandidates sends this peer's candidate set for the given session.
	PublishCandidates(ctx context.Context, sessionID string, self domain.PeerID, set domain.CandidateSet) error
	// AwaitPeerCandidates blocks until the counterpart's candidate set for
	// the session is available, or ctx is cancelled.
	AwaitPeerCandidates(ctx context.Context, sessionID string, peer domain.PeerID) (domain.CandidateSet, error)
}
