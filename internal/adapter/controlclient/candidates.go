package controlclient

import (
	"context"
	"fmt"

	"github.com/fu1se/spur/internal/adapter/controlproto"
	"github.com/fu1se/spur/internal/domain"
)

// Client implements port.Signaler: PublishCandidates and
// AwaitPeerCandidates below match that interface's method set exactly.

// PublishCandidates sends this peer's candidate set for a session.
func (c *Client) PublishCandidates(ctx context.Context, sessionID string, self domain.PeerID, set domain.CandidateSet) error {
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
		Candidates: controlproto.CandidatesToProto(set.Candidates),
		PublicKey:  set.PublicKey[:],
	}); err != nil {
		return err
	}

	var resp controlproto.PublishCandidatesResponse
	return controlproto.ReadFrame(stream, &resp)
}

// AwaitPeerCandidates blocks until peer's candidate set for sessionID is
// published by the server, or ctx is cancelled.
func (c *Client) AwaitPeerCandidates(ctx context.Context, sessionID string, peer domain.PeerID) (domain.CandidateSet, error) {
	stream, err := c.conn.OpenStreamSync(ctx)
	if err != nil {
		return domain.CandidateSet{}, fmt.Errorf("controlclient: open stream: %w", err)
	}
	defer stream.Close()

	if err := controlproto.WriteMethod(stream, controlproto.MethodAwaitCandidates); err != nil {
		return domain.CandidateSet{}, err
	}
	if err := controlproto.WriteFrame(stream, &controlproto.AwaitCandidatesRequest{
		SessionId: sessionID,
		PeerId:    string(peer),
	}); err != nil {
		return domain.CandidateSet{}, err
	}

	var resp controlproto.AwaitCandidatesResponse
	if err := controlproto.ReadFrame(stream, &resp); err != nil {
		return domain.CandidateSet{}, err
	}
	if len(resp.GetPublicKey()) != len(domain.PublicKey{}) {
		return domain.CandidateSet{}, fmt.Errorf("controlclient: peer public key has bad length %d", len(resp.GetPublicKey()))
	}

	candidates, err := controlproto.CandidatesFromProto(resp.GetCandidates())
	if err != nil {
		return domain.CandidateSet{}, err
	}

	var pub domain.PublicKey
	copy(pub[:], resp.GetPublicKey())

	return domain.CandidateSet{Candidates: candidates, PublicKey: pub}, nil
}
