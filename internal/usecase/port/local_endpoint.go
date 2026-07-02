package port

import (
	"context"
	"io"
)

// LocalListener accepts local connections that should be forwarded through
// a tunnel — the port-forward initiator side ("spur connect").
type LocalListener interface {
	Accept(ctx context.Context) (io.ReadWriteCloser, error)
	Close() error
}

// LocalDialer connects to the local service a tunnel stream should be
// forwarded to — the port-forward responder side ("spur expose").
type LocalDialer interface {
	Dial(ctx context.Context) (io.ReadWriteCloser, error)
}
