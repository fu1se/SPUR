package tunnel

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"

	"github.com/hashicorp/yamux"
	"github.com/quic-go/quic-go"

	"github.com/fu1se/localizator/internal/domain"
	"github.com/fu1se/localizator/internal/usecase/port"
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

func establishYamux(relayStream io.ReadWriteCloser, isDialer bool) (port.TunnelConn, error) {
	if relayStream == nil {
		return nil, fmt.Errorf("tunnel: relay session without a relay stream")
	}

	if isDialer {
		session, err := yamux.Client(relayStream, nil)
		if err != nil {
			return nil, fmt.Errorf("tunnel: yamux client: %w", err)
		}
		return &yamuxConn{session: session}, nil
	}

	session, err := yamux.Server(relayStream, nil)
	if err != nil {
		return nil, fmt.Errorf("tunnel: yamux server: %w", err)
	}
	return &yamuxConn{session: session}, nil
}
