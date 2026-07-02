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
		return
	}
	if len(req.PublicKey) != len(domain.PublicKey{}) {
		return
	}

	var pub domain.PublicKey
	copy(pub[:], req.PublicKey)

	observed, err := observedCandidate(conn.RemoteAddr())
	if err != nil {
		return
	}

	peer, err := s.RegisterPeer.Execute(ctx, pub, observed)
	if err != nil {
		return
	}

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
