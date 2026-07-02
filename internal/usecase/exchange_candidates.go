package usecase

import (
	"context"

	"github.com/fu1se/localizator/internal/domain"
	"github.com/fu1se/localizator/internal/usecase/port"
)

// PublishCandidates stores a peer's candidate set for a session so the
// counterpart can retrieve it. Server-side use case, backed by
// port.CandidateStore.
type PublishCandidates struct {
	Store port.CandidateStore
}

func (uc PublishCandidates) Execute(ctx context.Context, sessionID string, peer domain.PeerID, set domain.CandidateSet) error {
	return uc.Store.Put(ctx, sessionID, peer, set)
}

// AwaitCandidates blocks until the requested peer's candidate set for a
// session is available. Server-side use case, backed by
// port.CandidateStore.
type AwaitCandidates struct {
	Store port.CandidateStore
}

func (uc AwaitCandidates) Execute(ctx context.Context, sessionID string, peer domain.PeerID) (domain.CandidateSet, error) {
	return uc.Store.Wait(ctx, sessionID, peer)
}

// ExchangeCandidates is the client-side use case: publish our own
// candidate set for a session, then wait for the counterpart's. Backed by
// port.Signaler, which speaks to the server's PublishCandidates/
// AwaitCandidates use cases over the control channel.
type ExchangeCandidates struct {
	Signaler port.Signaler
}

func (uc ExchangeCandidates) Execute(ctx context.Context, sessionID string, self, counterpart domain.PeerID, own domain.CandidateSet) (domain.CandidateSet, error) {
	if err := uc.Signaler.PublishCandidates(ctx, sessionID, self, own); err != nil {
		return domain.CandidateSet{}, err
	}
	return uc.Signaler.AwaitPeerCandidates(ctx, sessionID, counterpart)
}
