package usecase

import (
	"context"

	"github.com/fu1se/spur/internal/usecase/port"
)

// ForwardPort is the "spur connect" (initiator) side of port-forward mode:
// every local connection accepted on Listener gets its own tunnel stream,
// spliced together until either end closes.
type ForwardPort struct {
	Listener port.LocalListener
	Tunnel   port.TunnelConn
}

// Run blocks accepting local connections until ctx is cancelled or
// Listener.Accept returns an error.
func (uc ForwardPort) Run(ctx context.Context) error {
	for {
		local, err := uc.Listener.Accept(ctx)
		if err != nil {
			return err
		}

		stream, err := uc.Tunnel.OpenStream(ctx)
		if err != nil {
			_ = local.Close()
			continue
		}

		go pipe(local, stream)
	}
}
