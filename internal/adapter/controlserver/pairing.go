package controlserver

import (
	"context"
	"errors"

	"github.com/quic-go/quic-go"

	"github.com/fu1se/spur/internal/adapter/controlproto"
	"github.com/fu1se/spur/internal/domain"
)

func (s *Server) handleRegisterPairingCode(ctx context.Context, stream *quic.Stream) {
	var req controlproto.RegisterPairingCodeRequest
	if err := controlproto.ReadFrame(stream, &req); err != nil {
		s.log().Warn().Err(err).Msg("register-pairing-code: read frame failed")
		return
	}
	if len(req.GetPublicKey()) != len(domain.PublicKey{}) {
		s.log().Warn().Int("len", len(req.GetPublicKey())).Msg("register-pairing-code: wrong public key length")
		return
	}

	// Same pattern as handleJoinNetwork/handlePublishCandidates: the peer
	// ID a code resolves to is derived from the caller's own public key
	// server-side, never trusted from the request — otherwise anyone
	// could mint a code that resolves to a victim's peer ID without
	// controlling that identity's private key at all.
	var pub domain.PublicKey
	copy(pub[:], req.GetPublicKey())
	self := domain.DerivePeerID(pub)

	code, err := s.RegisterPairingCode.Execute(ctx, self)
	if err != nil {
		s.log().Error().Err(err).Msg("register-pairing-code: use case failed")
		return
	}

	s.log().Info().Str("peer_id", string(self)).Msg("pairing code registered")

	_ = controlproto.WriteFrame(stream, &controlproto.RegisterPairingCodeResponse{Code: code})
}

func (s *Server) handleResolvePairingCode(ctx context.Context, stream *quic.Stream) {
	var req controlproto.ResolvePairingCodeRequest
	if err := controlproto.ReadFrame(stream, &req); err != nil {
		s.log().Warn().Err(err).Msg("resolve-pairing-code: read frame failed")
		return
	}
	if len(req.GetPublicKey()) != len(domain.PublicKey{}) {
		s.log().Warn().Int("len", len(req.GetPublicKey())).Msg("resolve-pairing-code: wrong public key length")
		return
	}

	var pub domain.PublicKey
	copy(pub[:], req.GetPublicKey())
	guest := domain.DerivePeerID(pub)

	host, err := s.ResolvePairingCode.Execute(ctx, req.GetCode(), guest)
	if err != nil {
		if errors.Is(err, domain.ErrPairingCodeNotFound) {
			s.log().Warn().Str("code", req.GetCode()).Msg("resolve-pairing-code: not found or expired")
			_ = controlproto.WriteFrame(stream, &controlproto.ResolvePairingCodeResponse{Error: err.Error()})
		} else {
			s.log().Error().Err(err).Msg("resolve-pairing-code: use case failed")
		}
		return
	}

	s.log().Info().Str("code", req.GetCode()).Str("host_peer_id", string(host)).Str("guest_peer_id", string(guest)).Msg("pairing code resolved")

	_ = controlproto.WriteFrame(stream, &controlproto.ResolvePairingCodeResponse{PeerId: string(host)})
}

func (s *Server) handleAwaitPairingCodeUse(ctx context.Context, stream *quic.Stream) {
	var req controlproto.AwaitPairingCodeUseRequest
	if err := controlproto.ReadFrame(stream, &req); err != nil {
		s.log().Warn().Err(err).Msg("await-pairing-code-use: read frame failed")
		return
	}

	guest, err := s.AwaitPairingCodeUse.Execute(ctx, req.GetCode())
	if err != nil {
		// Routine: the code simply hasn't been used yet when the caller
		// gives up (ctx done) or was never registered — not worth error
		// level.
		s.log().Debug().Err(err).Str("code", req.GetCode()).Msg("await-pairing-code-use: use case failed")
		return
	}

	s.log().Debug().Str("code", req.GetCode()).Str("guest_peer_id", string(guest)).Msg("pairing code use awaited")

	_ = controlproto.WriteFrame(stream, &controlproto.AwaitPairingCodeUseResponse{PeerId: string(guest)})
}
