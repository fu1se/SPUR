package usecase

import (
	"context"

	"github.com/fu1se/localizator/internal/domain"
	"github.com/fu1se/localizator/internal/usecase/port"
)

// PublishCandidates stores a peer's candidates for a session so the
// counterpart can retrieve them. Server-side use case, backed by
// port.CandidateStore.
type PublishCandidates struct {
	Store port.CandidateStore
}

func (uc PublishCandidates) Execute(ctx context.Context, sessionID string, peer domain.PeerID, candidates []domain.Candidate) error {
	return uc.Store.Put(ctx, sessionID, peer, candidates)
}

// AwaitCandidates blocks until the requested peer's candidates for a
// session are available. Server-side use case, backed by
// port.CandidateStore.
type AwaitCandidates struct {
	Store port.CandidateStore
}

func (uc AwaitCandidates) Execute(ctx context.Context, sessionID string, peer domain.PeerID) ([]domain.Candidate, error) {
	return uc.Store.Wait(ctx, sessionID, peer)
}

// ExchangeCandidates is the client-side use case: publish our own
// candidates for a session, then wait for the counterpart's. Backed by
// port.Signaler, which speaks to the server's PublishCandidates/
// AwaitCandidates use cases over the control channel.
type ExchangeCandidates struct {
	Signaler port.Signaler
}

func (uc ExchangeCandidates) Execute(ctx context.Context, sessionID string, self, counterpart domain.PeerID, ownCandidates []domain.Candidate) ([]domain.Candidate, error) {
	if err := uc.Signaler.PublishCandidates(ctx, sessionID, self, ownCandidates); err != nil {
		return nil, err
	}
	return uc.Signaler.AwaitPeerCandidates(ctx, sessionID, counterpart)
}
