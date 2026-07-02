package usecase

import (
	"context"

	"github.com/fu1se/spur/internal/usecase/port"
)

// ServeExposedPort is the "spur expose" (responder) side of port-forward
// mode: every tunnel stream accepted from Tunnel gets dialed out to the
// local service via Dialer, spliced together until either end closes.
type ServeExposedPort struct {
	Dialer port.LocalDialer
	Tunnel port.TunnelConn
}

// Run blocks accepting tunnel streams until ctx is cancelled or
// Tunnel.AcceptStream returns an error.
func (uc ServeExposedPort) Run(ctx context.Context) error {
	for {
		stream, err := uc.Tunnel.AcceptStream(ctx)
		if err != nil {
			return err
		}

		local, err := uc.Dialer.Dial(ctx)
		if err != nil {
			_ = stream.Close()
			continue
		}

		go pipe(local, stream)
	}
}
