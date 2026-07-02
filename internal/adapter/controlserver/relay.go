package controlserver

import (
	"context"

	"github.com/quic-go/quic-go"

	"github.com/fu1se/localizator/internal/adapter/controlproto"
)

// handleRelay reads the RelayOpenRequest header, then hands the rest of
// the stream to RelayFallback as an opaque byte pipe. It blocks until the
// splice with the counterpart's stream finishes.
func (s *Server) handleRelay(ctx context.Context, stream *quic.Stream) {
	var req controlproto.RelayOpenRequest
	if err := controlproto.ReadFrame(stream, &req); err != nil {
		s.log().Warn().Err(err).Msg("relay: read frame failed")
		return
	}

	// Worth info level, not debug: relay only kicks in when P2P punching
	// failed, so every occurrence means a client is paying the
	// server-in-the-middle cost instead of a direct path — useful signal
	// for an operator wondering why traffic volume looks higher than
	// expected.
	s.log().Info().Str("session_id", req.GetSessionId()).Msg("relay: session opened")

	if err := s.RelayFallback.Execute(ctx, req.GetSessionId(), stream); err != nil {
		s.log().Debug().Err(err).Str("session_id", req.GetSessionId()).Msg("relay: splice ended")
	}
}
