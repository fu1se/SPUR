package controlserver

import (
	"context"

	"github.com/quic-go/quic-go"

	"github.com/fu1se/localizator/internal/adapter/controlproto"
	"github.com/fu1se/localizator/internal/domain"
)

func (s *Server) handleJoinNetwork(ctx context.Context, stream *quic.Stream) {
	var req controlproto.JoinNetworkRequest
	if err := controlproto.ReadFrame(stream, &req); err != nil {
		return
	}
	if len(req.GetPublicKey()) != len(domain.PublicKey{}) {
		return
	}

	var pub domain.PublicKey
	copy(pub[:], req.GetPublicKey())
	peer := domain.DerivePeerID(pub)

	network, err := s.JoinNetwork.Execute(ctx, req.GetNetworkName(), peer, pub)
	if err != nil {
		return
	}

	_ = controlproto.WriteFrame(stream, &controlproto.JoinNetworkResponse{
		Cidr:    network.CIDR.String(),
		Members: controlproto.MeshMembersToProto(network.Members),
	})
}
