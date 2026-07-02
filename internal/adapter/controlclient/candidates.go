package controlclient

import (
	"context"
	"fmt"

	"github.com/fu1se/localizator/internal/adapter/controlproto"
	"github.com/fu1se/localizator/internal/domain"
)

// Client implements port.Signaler: PublishCandidates and
// AwaitPeerCandidates below match that interface's method set exactly.

// PublishCandidates sends this peer's candidates for a session.
func (c *Client) PublishCandidates(ctx context.Context, sessionID string, self domain.PeerID, candidates []domain.Candidate) error {
	stream, err := c.conn.OpenStreamSync(ctx)
	if err != nil {
		return fmt.Errorf("controlclient: open stream: %w", err)
	}
	defer stream.Close()

	if err := controlproto.WriteMethod(stream, controlproto.MethodPublishCandidates); err != nil {
		return err
	}
	if err := controlproto.WriteFrame(stream, &controlproto.PublishCandidatesRequest{
		SessionId:  sessionID,
		PeerId:     string(self),
		Candidates: controlproto.CandidatesToProto(candidates),
	}); err != nil {
		return err
	}

	var resp controlproto.PublishCandidatesResponse
	return controlproto.ReadFrame(stream, &resp)
}

// AwaitPeerCandidates blocks until peer's candidates for sessionID are
// published by the server, or ctx is cancelled.
func (c *Client) AwaitPeerCandidates(ctx context.Context, sessionID string, peer domain.PeerID) ([]domain.Candidate, error) {
	stream, err := c.conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, fmt.Errorf("controlclient: open stream: %w", err)
	}
	defer stream.Close()

	if err := controlproto.WriteMethod(stream, controlproto.MethodAwaitCandidates); err != nil {
		return nil, err
	}
	if err := controlproto.WriteFrame(stream, &controlproto.AwaitCandidatesRequest{
		SessionId: sessionID,
		PeerId:    string(peer),
	}); err != nil {
		return nil, err
	}

	var resp controlproto.AwaitCandidatesResponse
	if err := controlproto.ReadFrame(stream, &resp); err != nil {
		return nil, err
	}

	return controlproto.CandidatesFromProto(resp.GetCandidates())
}
