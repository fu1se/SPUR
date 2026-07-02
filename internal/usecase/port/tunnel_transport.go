package port

import (
	"context"
	"io"

	"github.com/fu1se/spur/internal/domain"
)

// Stream is a single bidirectional multiplexed channel within a
// TunnelConn.
type Stream interface {
	io.ReadWriteCloser
}

// TunnelConn is an established, multiplexed data-plane connection. Both
// data-plane modes are built on top of it: port-forward pipes each
// accepted local TCP connection into its own Stream, mesh mode feeds
// WireGuard UDP packets through a Stream.
type TunnelConn interface {
	OpenStream(ctx context.Context) (Stream, error)
	AcceptStream(ctx context.Context) (Stream, error)
	Close() error
}

// TunnelTransport turns an established Session into a multiplexed
// TunnelConn.
//
// This is deliberately not shaped as Dial(addr)/Accept(): a P2P session
// only carries a resolved address, over which the transport still needs
// to run a real handshake (QUIC, reusing the punched socket so the NAT
// mapping stays valid) — but a relay session has no dialable address at
// all, it already *is* a live duplex byte stream (see
// usecase.EstablishSession's doc comment and CLAUDE.md's "Время жизни
// relay-стрима"), which gets multiplexed with yamux instead of QUIC.
// EstablishConn hides that split behind one call; isDialer picks which
// side opens vs accepts the multiplexed connection (domain.IsDialer).
type TunnelTransport interface {
	EstablishConn(ctx context.Context, session domain.Session, relayStream io.ReadWriteCloser, isDialer bool) (TunnelConn, error)
}
