package usecase

import (
	"context"
	"io"

	"github.com/fu1se/spur/internal/usecase/port"
)

// RelayFallback is the server-side use case backing the relay data path:
// join a session's relay pairing and block until it's spliced to the
// counterpart and finishes. Backed by port.RelayBroker.
type RelayFallback struct {
	Broker port.RelayBroker
}

func (uc RelayFallback) Execute(ctx context.Context, sessionID string, conn io.ReadWriteCloser) error {
	return uc.Broker.Join(ctx, sessionID, conn)
}
