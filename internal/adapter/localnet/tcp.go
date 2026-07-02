// Package localnet implements port.LocalListener and port.LocalDialer over
// plain TCP: the initiator ("spur connect") listens locally for
// applications to forward, the responder ("spur expose") dials the local
// service being exposed for every incoming tunnel stream.
package localnet

import (
	"context"
	"fmt"
	"io"
	"net"
)

// TCPListener implements port.LocalListener: it accepts local TCP
// connections on Addr (used by "spur connect" to accept the client
// application's connections before forwarding them through the tunnel).
type TCPListener struct {
	ln net.Listener
}

// ListenTCP binds addr immediately so callers know it succeeded before
// accepting.
func ListenTCP(addr string) (*TCPListener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("localnet: listen: %w", err)
	}
	return &TCPListener{ln: ln}, nil
}

func (l *TCPListener) Accept(ctx context.Context) (io.ReadWriteCloser, error) {
	type result struct {
		conn net.Conn
		err  error
	}
	resultCh := make(chan result, 1)
	go func() {
		conn, err := l.ln.Accept()
		resultCh <- result{conn, err}
	}()

	select {
	case r := <-resultCh:
		return r.conn, r.err
	case <-ctx.Done():
		_ = l.ln.Close()
		return nil, ctx.Err()
	}
}

func (l *TCPListener) Close() error {
	return l.ln.Close()
}

// Addr returns the bound local address — useful when addr passed to
// ListenTCP used an ephemeral port (":0").
func (l *TCPListener) Addr() net.Addr {
	return l.ln.Addr()
}

// TCPDialer implements port.LocalDialer: it connects to Addr (used by
// "spur expose" to reach the local service being exposed, for every
// incoming tunnel stream).
type TCPDialer struct {
	Addr string
}

func (d TCPDialer) Dial(ctx context.Context) (io.ReadWriteCloser, error) {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", d.Addr)
	if err != nil {
		return nil, fmt.Errorf("localnet: dial %s: %w", d.Addr, err)
	}
	return conn, nil
}
