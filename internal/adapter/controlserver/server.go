// Package controlserver is the rendezvous server's control-plane adapter:
// it accepts QUIC connections from clients and translates control-protocol
// frames into use case calls.
package controlserver

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/quic-go/quic-go"

	"github.com/fu1se/localizator/internal/adapter/controlproto"
	"github.com/fu1se/localizator/internal/usecase"
)

// awaitCandidatesTimeout bounds how long the server will hold an
// AwaitCandidates stream open waiting for the counterpart to publish. It is
// a safety net against a counterpart that never shows up, not the primary
// shutdown mechanism (server shutdown is handled via the ctx passed into
// Serve, which is the parent of every request context).
const awaitCandidatesTimeout = 60 * time.Second

// Server serves the control-plane protocol over QUIC.
type Server struct {
	RegisterPeer      usecase.RegisterPeer
	PublishCandidates usecase.PublishCandidates
	AwaitCandidates   usecase.AwaitCandidates
}

// Serve runs the control-plane QUIC listener on conn until ctx is
// cancelled. It blocks. conn is typically obtained via net.ListenPacket by
// the caller (composition root or a test) so the caller knows the actual
// bound address before Serve is called — important for tests, which use
// ephemeral ports and must not race a separate bind against this call.
func (s *Server) Serve(ctx context.Context, conn net.PacketConn, tlsConf *tls.Config) error {
	ln, err := quic.Listen(conn, tlsConf, nil)
	if err != nil {
		return fmt.Errorf("controlserver: listen: %w", err)
	}
	defer ln.Close()

	for {
		conn, err := ln.Accept(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("controlserver: accept: %w", err)
		}
		go s.handleConn(ctx, conn)
	}
}

func (s *Server) handleConn(ctx context.Context, conn *quic.Conn) {
	for {
		stream, err := conn.AcceptStream(ctx)
		if err != nil {
			return
		}
		go s.handleStream(ctx, conn, stream)
	}
}

func (s *Server) handleStream(ctx context.Context, conn *quic.Conn, stream *quic.Stream) {
	defer stream.Close()

	method, err := controlproto.ReadMethod(stream)
	if err != nil {
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
	}
}
