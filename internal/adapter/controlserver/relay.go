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
		return
	}

	_ = s.RelayFallback.Execute(ctx, req.GetSessionId(), stream)
}
