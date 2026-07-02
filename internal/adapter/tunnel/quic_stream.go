// Package tunnel implements port.TunnelTransport: QUIC (reusing the punched
// UDP socket) for P2P sessions, yamux for relay sessions. Both back the
// same port.TunnelConn/port.Stream interfaces so use cases (ForwardPort,
// ServeExposedPort, and later the mesh mode) don't need to know which path
// a given session took.
package tunnel

import (
	"context"

	"github.com/quic-go/quic-go"

	"github.com/fu1se/spur/internal/usecase/port"
)

// DataALPN is the QUIC application protocol negotiated for direct P2P
// data-plane connections — distinct from controlproto.ALPN, since this is
// a separate connection dialed straight to the punched peer address, not
// to the rendezvous server.
const DataALPN = "spur-data/1"

type quicConn struct {
	conn *quic.Conn
}

func (c *quicConn) OpenStream(ctx context.Context) (port.Stream, error) {
	return c.conn.OpenStreamSync(ctx)
}

func (c *quicConn) AcceptStream(ctx context.Context) (port.Stream, error) {
	return c.conn.AcceptStream(ctx)
}

func (c *quicConn) Close() error {
	return c.conn.CloseWithError(0, "")
}
