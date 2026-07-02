package controlserver

import (
	"context"
	"errors"

	"github.com/quic-go/quic-go"

	"github.com/fu1se/localizator/internal/adapter/controlproto"
	"github.com/fu1se/localizator/internal/domain"
)

func (s *Server) handleJoinNetwork(ctx context.Context, stream *quic.Stream) {
	var req controlproto.JoinNetworkRequest
	if err := controlproto.ReadFrame(stream, &req); err != nil {
		s.log().Warn().Err(err).Msg("join-network: read frame failed")
		return
	}
	if len(req.GetPublicKey()) != len(domain.PublicKey{}) {
		s.log().Warn().Int("len", len(req.GetPublicKey())).Msg("join-network: wrong public key length")
		return
	}

	var pub domain.PublicKey
	copy(pub[:], req.GetPublicKey())
	peer := domain.DerivePeerID(pub)

	network, err := s.JoinNetwork.Execute(ctx, req.GetNetworkName(), peer, pub, req.GetInviteToken())
	if err != nil {
		if errors.Is(err, domain.ErrInvalidInviteToken) {
			// A wrong/missing token is worth a real log line: repeated
			// attempts against the same network are exactly what an
			// operator would want to notice.
			s.log().Warn().Str("network", req.GetNetworkName()).Str("peer_id", string(peer)).Msg("join-network: invalid invite token")
			_ = controlproto.WriteFrame(stream, &controlproto.JoinNetworkResponse{Error: err.Error()})
		} else {
			s.log().Error().Err(err).Str("network", req.GetNetworkName()).Msg("join-network: use case failed")
		}
		return
	}

	s.log().Info().Str("network", req.GetNetworkName()).Str("peer_id", string(peer)).Int("members", len(network.Members)).Msg("peer joined network")

	_ = controlproto.WriteFrame(stream, &controlproto.JoinNetworkResponse{
		Cidr:        network.CIDR.String(),
		Members:     controlproto.MeshMembersToProto(network.Members),
		InviteToken: network.InviteToken,
	})
}
