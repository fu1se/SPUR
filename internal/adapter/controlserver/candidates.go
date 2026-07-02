package controlserver

import (
	"context"

	"github.com/quic-go/quic-go"

	"github.com/fu1se/localizator/internal/adapter/controlproto"
	"github.com/fu1se/localizator/internal/domain"
)

func (s *Server) handlePublishCandidates(ctx context.Context, stream *quic.Stream) {
	var req controlproto.PublishCandidatesRequest
	if err := controlproto.ReadFrame(stream, &req); err != nil {
		return
	}
	if len(req.GetPublicKey()) != len(domain.PublicKey{}) {
		return
	}

	candidates, err := controlproto.CandidatesFromProto(req.GetCandidates())
	if err != nil {
		return
	}

	var pub domain.PublicKey
	copy(pub[:], req.GetPublicKey())
	set := domain.CandidateSet{Candidates: candidates, PublicKey: pub}

	if err := s.PublishCandidates.Execute(ctx, req.GetSessionId(), domain.PeerID(req.GetPeerId()), set); err != nil {
		return
	}

	_ = controlproto.WriteFrame(stream, &controlproto.PublishCandidatesResponse{})
}

func (s *Server) handleAwaitCandidates(ctx context.Context, stream *quic.Stream) {
	var req controlproto.AwaitCandidatesRequest
	if err := controlproto.ReadFrame(stream, &req); err != nil {
		return
	}

	set, err := s.AwaitCandidates.Execute(ctx, req.GetSessionId(), domain.PeerID(req.GetPeerId()))
	if err != nil {
		return
	}

	_ = controlproto.WriteFrame(stream, &controlproto.AwaitCandidatesResponse{
		Candidates: controlproto.CandidatesToProto(set.Candidates),
		PublicKey:  set.PublicKey[:],
	})
}
