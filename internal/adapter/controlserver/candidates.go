package controlserver

import (
	"context"

	"github.com/quic-go/quic-go"

	"github.com/fu1se/spur/internal/adapter/controlproto"
	"github.com/fu1se/spur/internal/domain"
)

func (s *Server) handlePublishCandidates(ctx context.Context, stream *quic.Stream) {
	var req controlproto.PublishCandidatesRequest
	if err := controlproto.ReadFrame(stream, &req); err != nil {
		s.log().Warn().Err(err).Msg("publish-candidates: read frame failed")
		return
	}
	if len(req.GetPublicKey()) != len(domain.PublicKey{}) {
		s.log().Warn().Int("len", len(req.GetPublicKey())).Msg("publish-candidates: wrong public key length")
		return
	}
	if len(req.GetSalt()) != 32 {
		s.log().Warn().Int("len", len(req.GetSalt())).Msg("publish-candidates: wrong salt length")
		return
	}

	candidates, err := controlproto.CandidatesFromProto(req.GetCandidates())
	if err != nil {
		s.log().Warn().Err(err).Msg("publish-candidates: decode candidates failed")
		return
	}

	var pub domain.PublicKey
	copy(pub[:], req.GetPublicKey())
	var salt [32]byte
	copy(salt[:], req.GetSalt())
	set := domain.CandidateSet{Candidates: candidates, PublicKey: pub, Salt: salt}

	// The peer ID this gets stored under is derived from the caller's own
	// public_key, not trusted from req.GetPeerId(): a client-asserted
	// peer_id with no binding to the key it's paired with would let anyone
	// who knows a victim's peer_id (peer IDs are exchanged out-of-band,
	// not secret) publish forged candidates + their own public key under
	// that ID and hijack the session the victim's counterpart establishes.
	// See handleJoinNetwork for the same pattern already in use there.
	peer := domain.DerivePeerID(pub)

	if err := s.PublishCandidates.Execute(ctx, req.GetSessionId(), peer, set); err != nil {
		s.log().Error().Err(err).Str("session_id", req.GetSessionId()).Msg("publish-candidates: use case failed")
		return
	}

	s.log().Debug().Str("session_id", req.GetSessionId()).Str("peer_id", string(peer)).Msg("candidates published")

	_ = controlproto.WriteFrame(stream, &controlproto.PublishCandidatesResponse{})
}

func (s *Server) handleAwaitCandidates(ctx context.Context, stream *quic.Stream) {
	var req controlproto.AwaitCandidatesRequest
	if err := controlproto.ReadFrame(stream, &req); err != nil {
		s.log().Warn().Err(err).Msg("await-candidates: read frame failed")
		return
	}

	set, err := s.AwaitCandidates.Execute(ctx, req.GetSessionId(), domain.PeerID(req.GetPeerId()))
	if err != nil {
		// Routine: the awaiting side times out whenever its counterpart
		// never shows up (see awaitCandidatesTimeout) — not worth error
		// level, that would fire on every solo/abandoned session.
		s.log().Debug().Err(err).Str("session_id", req.GetSessionId()).Msg("await-candidates: use case failed")
		return
	}

	s.log().Debug().Str("session_id", req.GetSessionId()).Str("peer_id", req.GetPeerId()).Msg("candidates awaited")

	_ = controlproto.WriteFrame(stream, &controlproto.AwaitCandidatesResponse{
		Candidates: controlproto.CandidatesToProto(set.Candidates),
		PublicKey:  set.PublicKey[:],
		Salt:       set.Salt[:],
	})
}
