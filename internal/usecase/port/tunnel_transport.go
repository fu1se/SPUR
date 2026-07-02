package port

import (
	"context"
	"io"
	"net/netip"
)

// Stream is a single bidirectional multiplexed channel within a
// TunnelConn.
type Stream interface {
	io.ReadWriteCloser
}

// TunnelConn is an established, multiplexed data-plane connection to a
// resolved peer address. Both data-plane modes are built on top of it:
// port-forward pipes each accepted local TCP connection into its own
// Stream, mesh mode feeds WireGuard UDP packets through a Stream.
type TunnelConn interface {
	OpenStream(ctx context.Context) (Stream, error)
	AcceptStream(ctx context.Context) (Stream, error)
	Close() error
}

// TunnelTransport establishes TunnelConns to addresses resolved by a
// Puncher or Relay.
type TunnelTransport interface {
	Dial(ctx context.Context, addr netip.AddrPort) (TunnelConn, error)
	Accept(ctx context.Context) (TunnelConn, error)
}
