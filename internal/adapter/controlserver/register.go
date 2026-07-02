package controlserver

import (
	"context"
	"errors"
	"net"
	"net/netip"

	"github.com/quic-go/quic-go"

	"github.com/fu1se/localizator/internal/adapter/controlproto"
	"github.com/fu1se/localizator/internal/domain"
)

func (s *Server) handleRegister(ctx context.Context, conn *quic.Conn, stream *quic.Stream) {
	var req controlproto.RegisterRequest
	if err := controlproto.ReadFrame(stream, &req); err != nil {
		s.log().Warn().Err(err).Msg("register: read frame failed")
		return
	}
	if len(req.PublicKey) != len(domain.PublicKey{}) {
		s.log().Warn().Int("len", len(req.PublicKey)).Msg("register: wrong public key length")
		return
	}

	var pub domain.PublicKey
	copy(pub[:], req.PublicKey)

	observed, err := observedCandidate(conn.RemoteAddr())
	if err != nil {
		s.log().Warn().Err(err).Msg("register: observed candidate failed")
		return
	}

	peer, err := s.RegisterPeer.Execute(ctx, pub, observed)
	if err != nil {
		s.log().Error().Err(err).Msg("register: use case failed")
		return
	}

	s.log().Info().Str("peer_id", string(peer.ID)).Str("observed", observed.Addr.String()).Msg("peer registered")

	_ = controlproto.WriteFrame(stream, &controlproto.RegisterResponse{
		PeerId:          string(peer.ID),
		ObservedAddress: observed.Addr.String(),
	})
}

func observedCandidate(addr net.Addr) (domain.Candidate, error) {
	ap, err := netip.ParseAddrPort(addr.String())
	if err != nil {
		return domain.Candidate{}, errors.New("controlserver: unparsable remote addr")
	}
	return domain.Candidate{Kind: domain.CandidateServerReflexive, Addr: ap}, nil
}
