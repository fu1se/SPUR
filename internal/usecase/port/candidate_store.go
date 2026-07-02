package port

import (
	"context"

	"github.com/fu1se/localizator/internal/domain"
)

// CandidateStore is the server-side rendezvous point for candidate
// exchange: it lets one connection publish a peer's candidate set and
// another connection block until it's available. It is distinct from
// Signaler, which is the client's view of talking to the server over the
// wire — CandidateStore is what backs the server side of that
// conversation.
type CandidateStore interface {
	Put(ctx context.Context, sessionID string, peer domain.PeerID, set domain.CandidateSet) error
	// Wait blocks until a candidate set published for (sessionID, peer) is
	// available, or ctx is done.
	Wait(ctx context.Context, sessionID string, peer domain.PeerID) (domain.CandidateSet, error)
}
