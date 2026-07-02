package tunnel

import (
	"context"

	"github.com/hashicorp/yamux"

	"github.com/fu1se/spur/internal/usecase/port"
)

type yamuxConn struct {
	session *yamux.Session
}

func (c *yamuxConn) OpenStream(ctx context.Context) (port.Stream, error) {
	return c.session.OpenStream()
}

func (c *yamuxConn) AcceptStream(ctx context.Context) (port.Stream, error) {
	return c.session.AcceptStreamWithContext(ctx)
}

func (c *yamuxConn) Close() error {
	return c.session.Close()
}
