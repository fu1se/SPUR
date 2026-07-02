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
// Tunnel.AcceptStream returns an error. See ForwardPort.Run's doc comment
// for why a semaphore slot is acquired before each AcceptStream call
// rather than only around the resulting goroutine: it's the counterpart
// on the other end of this same tunnel that opens these streams, and
// nothing about it is re-verified per stream, so bounding concurrency
// here guards against it opening an unbounded number of them.
func (uc ServeExposedPort) Run(ctx context.Context) error {
	sem := make(chan struct{}, maxConcurrentTunnels)
	for {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}

		stream, err := uc.Tunnel.AcceptStream(ctx)
		if err != nil {
			<-sem
			return err
		}

		local, err := uc.Dialer.Dial(ctx)
		if err != nil {
			_ = stream.Close()
			<-sem
			continue
		}

		go func() {
			defer func() { <-sem }()
			pipe(local, stream)
		}()
	}
}
