package tunnel

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/quic-go/quic-go"

	"github.com/fu1se/spur/internal/domain"
	"github.com/fu1se/spur/internal/usecase/port"
)

// Transport implements port.TunnelTransport. Conn is the UDP socket that
// was used for hole punching (see usecase.EstablishSession /
// adapter/nat.UDPPuncher) — reusing it for the QUIC data-plane connection
// is what keeps the punched NAT mapping valid; opening a fresh socket
// would punch a hole nobody is listening through. TLSConf is only used
// when acting as the QUIC listener (isDialer == false); dialing doesn't
// need a certificate.
type Transport struct {
	Conn     *net.UDPConn
	TLSConf  *tls.Config
	QUICConf *quic.Config
}

func (t *Transport) EstablishConn(ctx context.Context, session domain.Session, relayStream io.ReadWriteCloser, isDialer bool) (port.TunnelConn, error) {
	switch session.State {
	case domain.SessionEstablishedP2P:
		return t.establishQUIC(ctx, session, isDialer)
	case domain.SessionEstablishedRelay:
		return establishYamux(relayStream, isDialer)
	default:
		return nil, fmt.Errorf("tunnel: cannot establish transport for session state %q", session.State)
	}
}

func (t *Transport) establishQUIC(ctx context.Context, session domain.Session, isDialer bool) (port.TunnelConn, error) {
	if isDialer {
		remote := net.UDPAddrFromAddrPort(session.ResolvedAddr)
		dialTLSConf := &tls.Config{NextProtos: []string{DataALPN}, InsecureSkipVerify: true} //nolint:gosec // interim, see CLAUDE.md Phase 7
		conn, err := quic.Dial(ctx, t.Conn, remote, dialTLSConf, t.QUICConf)
		if err != nil {
			return nil, fmt.Errorf("tunnel: quic dial: %w", err)
		}
		return &quicConn{conn: conn}, nil
	}

	ln, err := quic.Listen(t.Conn, t.TLSConf, t.QUICConf)
	if err != nil {
		return nil, fmt.Errorf("tunnel: quic listen: %w", err)
	}

	conn, err := ln.Accept(ctx)
	if err != nil {
		return nil, fmt.Errorf("tunnel: quic accept: %w", err)
	}
	return &quicConn{conn: conn}, nil
}

// yamuxConfig is shared by both the client and server side of a relay
// session. yamux.DefaultConfig's ConnectionWriteTimeout (10s) is tuned
// for a lightly loaded connection; on this codebase's relay path it also
// carries a single bulk file/directory transfer (see usecase.SendFiles),
// and yamux's keepalive ping shares the same outbound send queue as that
// data — under sustained heavy writes, the ping frame can genuinely sit
// queued behind in-flight data for more than 10s on a real (if merely
// briefly congested, not actually dead) network path. yamux treats a
// keepalive miss as fatal and tears down the whole session immediately
// (see hashicorp/yamux's Session.keepalive/waitForSendErr), which was a
// real bug found live: a 100GB relay transfer at ~7MB/s died at 19GB in
// with "yamux: keepalive failed: i/o deadline reached" — the connection
// was never actually down, the ping just lost the race against a large
// in-flight write. A generous ConnectionWriteTimeout gives real
// congestion room to clear before yamux gives up; genuinely dead
// connections still get caught, just less eagerly. LogOutput is
// discarded because yamux logs to stderr via the stdlib log package by
// default (a different, unformatted style from the rest of this
// codebase's output) and every failure here already surfaces properly as
// a returned error from the affected Read/Write call.
func yamuxConfig() *yamux.Config {
	cfg := yamux.DefaultConfig()
	cfg.ConnectionWriteTimeout = 60 * time.Second
	cfg.LogOutput = io.Discard
	return cfg
}

func establishYamux(relayStream io.ReadWriteCloser, isDialer bool) (port.TunnelConn, error) {
	if relayStream == nil {
		return nil, fmt.Errorf("tunnel: relay session without a relay stream")
	}

	if isDialer {
		session, err := yamux.Client(relayStream, yamuxConfig())
		if err != nil {
			return nil, fmt.Errorf("tunnel: yamux client: %w", err)
		}
		return &yamuxConn{session: session}, nil
	}

	session, err := yamux.Server(relayStream, yamuxConfig())
	if err != nil {
		return nil, fmt.Errorf("tunnel: yamux server: %w", err)
	}
	return &yamuxConn{session: session}, nil
}
