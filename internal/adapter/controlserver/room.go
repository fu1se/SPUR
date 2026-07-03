package controlserver

import (
	"context"
	"errors"

	"github.com/quic-go/quic-go"

	"github.com/fu1se/spur/internal/adapter/controlproto"
	"github.com/fu1se/spur/internal/domain"
)

func (s *Server) handleCreateRoom(ctx context.Context, stream *quic.Stream) {
	var req controlproto.CreateRoomRequest
	if err := controlproto.ReadFrame(stream, &req); err != nil {
		s.log().Warn().Err(err).Msg("create-room: read frame failed")
		return
	}
	if len(req.GetPublicKey()) != len(domain.PublicKey{}) {
		s.log().Warn().Int("len", len(req.GetPublicKey())).Msg("create-room: wrong public key length")
		return
	}

	var pub domain.PublicKey
	copy(pub[:], req.GetPublicKey())
	creator := domain.DerivePeerID(pub)

	room, err := s.CreateRoom.Execute(ctx, req.GetRoomName(), creator)
	if err != nil {
		if errors.Is(err, domain.ErrRoomAlreadyExists) {
			s.log().Warn().Str("room", req.GetRoomName()).Msg("create-room: name already taken")
			_ = controlproto.WriteFrame(stream, &controlproto.CreateRoomResponse{Error: err.Error()})
		} else {
			s.log().Error().Err(err).Str("room", req.GetRoomName()).Msg("create-room: use case failed")
		}
		return
	}

	s.log().Info().Str("room", req.GetRoomName()).Str("peer_id", string(creator)).Msg("room created")

	_ = controlproto.WriteFrame(stream, &controlproto.CreateRoomResponse{InviteToken: room.InviteToken})
}

func (s *Server) handleJoinRoom(ctx context.Context, stream *quic.Stream) {
	var req controlproto.JoinRoomRequest
	if err := controlproto.ReadFrame(stream, &req); err != nil {
		s.log().Warn().Err(err).Msg("join-room: read frame failed")
		return
	}
	if len(req.GetPublicKey()) != len(domain.PublicKey{}) {
		s.log().Warn().Int("len", len(req.GetPublicKey())).Msg("join-room: wrong public key length")
		return
	}

	var pub domain.PublicKey
	copy(pub[:], req.GetPublicKey())
	peer := domain.DerivePeerID(pub)

	_, err := s.JoinRoom.Execute(ctx, req.GetRoomName(), peer, req.GetInviteToken())
	if err != nil {
		if errors.Is(err, domain.ErrInvalidInviteToken) || errors.Is(err, domain.ErrRoomFull) || errors.Is(err, domain.ErrRoomNotFound) {
			s.log().Warn().Str("room", req.GetRoomName()).Str("peer_id", string(peer)).Err(err).Msg("join-room: rejected")
			_ = controlproto.WriteFrame(stream, &controlproto.JoinRoomResponse{Error: err.Error()})
		} else {
			s.log().Error().Err(err).Str("room", req.GetRoomName()).Msg("join-room: use case failed")
		}
		return
	}

	s.log().Info().Str("room", req.GetRoomName()).Str("peer_id", string(peer)).Msg("peer joined room")

	_ = controlproto.WriteFrame(stream, &controlproto.JoinRoomResponse{})
}

func (s *Server) handleResolveRoom(ctx context.Context, stream *quic.Stream) {
	var req controlproto.ResolveRoomRequest
	if err := controlproto.ReadFrame(stream, &req); err != nil {
		s.log().Warn().Err(err).Msg("resolve-room: read frame failed")
		return
	}
	if len(req.GetPublicKey()) != len(domain.PublicKey{}) {
		s.log().Warn().Int("len", len(req.GetPublicKey())).Msg("resolve-room: wrong public key length")
		return
	}

	var pub domain.PublicKey
	copy(pub[:], req.GetPublicKey())
	peer := domain.DerivePeerID(pub)

	other, err := s.ResolveRoom.Execute(ctx, req.GetRoomName(), peer)
	if err != nil {
		if errors.Is(err, domain.ErrRoomNotFound) || errors.Is(err, domain.ErrNotRoomMember) || errors.Is(err, domain.ErrRoomNotReady) {
			s.log().Debug().Str("room", req.GetRoomName()).Str("peer_id", string(peer)).Err(err).Msg("resolve-room: not resolvable yet")
			_ = controlproto.WriteFrame(stream, &controlproto.ResolveRoomResponse{Error: err.Error()})
		} else {
			s.log().Error().Err(err).Str("room", req.GetRoomName()).Msg("resolve-room: use case failed")
		}
		return
	}

	_ = controlproto.WriteFrame(stream, &controlproto.ResolveRoomResponse{PeerId: string(other)})
}
