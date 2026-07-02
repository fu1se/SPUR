// Package controlserver is the rendezvous server's control-plane adapter:
// it accepts QUIC connections from clients and translates control-protocol
// frames into use case calls.
package controlserver

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/netip"

	"github.com/quic-go/quic-go"

	"github.com/fu1se/localizator/internal/adapter/controlproto"
	"github.com/fu1se/localizator/internal/domain"
	"github.com/fu1se/localizator/internal/usecase"
)

// Server serves the control-plane protocol over QUIC.
type Server struct {
	RegisterPeer usecase.RegisterPeer
}

// Serve listens on addr until ctx is cancelled. It blocks.
func (s *Server) Serve(ctx context.Context, addr string, tlsConf *tls.Config) error {
	ln, err := quic.ListenAddr(addr, tlsConf, nil)
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
