// Package controlserver is the rendezvous server's control-plane adapter:
// it accepts QUIC connections from clients and translates control-protocol
// frames into use case calls.
package controlserver

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/rs/zerolog"

	"github.com/fu1se/spur/internal/adapter/controlproto"
	"github.com/fu1se/spur/internal/usecase"
)

// nopLogger is what log() falls back to when Server.Logger is nil.
// Every existing test constructs &Server{...} without setting Logger —
// this keeps them working unchanged, with logging silently disabled,
// instead of panicking on a nil writer.
var nopLogger = zerolog.Nop()

// awaitCandidatesTimeout bounds how long the server will hold an
// AwaitCandidates stream open waiting for the counterpart to publish. It is
// a safety net against a counterpart that never shows up, not the primary
// shutdown mechanism (server shutdown is handled via the ctx passed into
// Serve, which is the parent of every request context).
const awaitCandidatesTimeout = 60 * time.Second

// maxConcurrentStreams bounds how many control-protocol streams (across
// every connection) this server will handle at once. Several RPCs hold a
// goroutine open for a while — AwaitCandidates up to awaitCandidatesTimeout,
// Relay for a whole tunnel's lifetime once paired — and none of them
// require prior authentication to invoke, so without a cap a client could
// flood the server with streams that each cost it a blocked goroutine,
// exhausting resources for everyone else. This is deliberately a single
// global limit, not per-connection: a per-connection cap alone wouldn't
// stop the same flood spread across many connections.
const maxConcurrentStreams = 1024

// Server serves the control-plane protocol over QUIC.
type Server struct {
	RegisterPeer      usecase.RegisterPeer
	PublishCandidates usecase.PublishCandidates
	AwaitCandidates   usecase.AwaitCandidates
	RelayFallback     usecase.RelayFallback
	JoinNetwork       usecase.JoinNetwork

	// Logger receives operational events (connections, per-RPC errors).
	// Every request handler used to drop its errors silently — an
	// operator running this as a long-lived process had no way to tell a
	// malformed request from a client bug from an attack attempt. nil is
	// valid and means "don't log" (see nopLogger); set it from
	// infra.NewLogger in the composition root to get real output.
	Logger *zerolog.Logger

	// MaxConcurrentStreams overrides maxConcurrentStreams; zero (the
	// default zero value, so every existing &Server{...} literal keeps
	// working unchanged) means "use maxConcurrentStreams". A field mainly
	// so a test can exercise the limit with a small number instead of
	// actually opening 1000+ real streams.
	MaxConcurrentStreams int64

	// activeStreams counts in-flight handleStream calls, see
	// maxConcurrentStreams. Zero value is a valid, ready-to-use counter,
	// so this doesn't require a constructor — every existing test and
	// composition root that builds &Server{...} directly keeps working.
	activeStreams atomic.Int64
}

func (s *Server) maxConcurrentStreams() int64 {
	if s.MaxConcurrentStreams > 0 {
		return s.MaxConcurrentStreams
	}
	return maxConcurrentStreams
}

func (s *Server) log() *zerolog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return &nopLogger
}

// Serve runs the control-plane QUIC listener on conn until ctx is
// cancelled. It blocks. conn is typically obtained via net.ListenPacket by
// the caller (composition root or a test) so the caller knows the actual
// bound address before Serve is called — important for tests, which use
// ephemeral ports and must not race a separate bind against this call.
func (s *Server) Serve(ctx context.Context, conn net.PacketConn, tlsConf *tls.Config, quicConf *quic.Config) error {
	ln, err := quic.Listen(conn, tlsConf, quicConf)
	if err != nil {
		return fmt.Errorf("controlserver: listen: %w", err)
	}
	defer ln.Close()

	s.log().Info().Str("addr", conn.LocalAddr().String()).Msg("control-plane listening")

	for {
		conn, err := ln.Accept(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("controlserver: accept: %w", err)
		}
		s.log().Debug().Str("remote", conn.RemoteAddr().String()).Msg("connection accepted")
		go s.handleConn(ctx, conn)
	}
}

func (s *Server) handleConn(ctx context.Context, conn *quic.Conn) {
	for {
		stream, err := conn.AcceptStream(ctx)
		if err != nil {
			return
		}

		if s.activeStreams.Add(1) > s.maxConcurrentStreams() {
			s.activeStreams.Add(-1)
			s.log().Warn().Str("remote", conn.RemoteAddr().String()).Msg("stream limit reached, rejecting")
			_ = stream.Close()
			continue
		}

		go func() {
			defer s.activeStreams.Add(-1)
			s.handleStream(ctx, conn, stream)
		}()
	}
}

func (s *Server) handleStream(ctx context.Context, conn *quic.Conn, stream *quic.Stream) {
	defer stream.Close()

	method, err := controlproto.ReadMethod(stream)
	if err != nil {
		s.log().Warn().Err(err).Str("remote", conn.RemoteAddr().String()).Msg("read method failed")
		return
	}

	reqCtx := ctx
	if method == controlproto.MethodAwaitCandidates {
		var cancel context.CancelFunc
		reqCtx, cancel = context.WithTimeout(ctx, awaitCandidatesTimeout)
		defer cancel()
	}

	switch method {
	case controlproto.MethodRegister:
		s.handleRegister(reqCtx, conn, stream)
	case controlproto.MethodPublishCandidates:
		s.handlePublishCandidates(reqCtx, stream)
	case controlproto.MethodAwaitCandidates:
		s.handleAwaitCandidates(reqCtx, stream)
	case controlproto.MethodRelay:
		s.handleRelay(reqCtx, stream)
	case controlproto.MethodJoinNetwork:
		s.handleJoinNetwork(reqCtx, stream)
	default:
		s.log().Warn().Str("remote", conn.RemoteAddr().String()).Interface("method", method).Msg("unknown method")
	}
}
