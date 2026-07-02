package usecase

import (
	"context"

	"github.com/fu1se/spur/internal/usecase/port"
)

// maxConcurrentTunnels bounds how many local connections/tunnel streams a
// single ForwardPort or ServeExposedPort will have spliced open at once.
// Without it, a counterpart that's misbehaving (or just malicious --
// nothing about who established the tunnel is re-verified per connection)
// could open connections/streams without limit and exhaust the local
// process's file descriptors.
const maxConcurrentTunnels = 256

// ForwardPort is the "spur connect" (initiator) side of port-forward mode:
// every local connection accepted on Listener gets its own tunnel stream,
// spliced together until either end closes.
type ForwardPort struct {
	Listener port.LocalListener
	Tunnel   port.TunnelConn
}

// Run blocks accepting local connections until ctx is cancelled or
// Listener.Accept returns an error. Acquiring a semaphore slot before the
// next Accept call means an already-saturated forwarder also naturally
// stops pulling new connections off the listener's backlog, rather than
// accepting unboundedly and queuing goroutines behind the semaphore.
func (uc ForwardPort) Run(ctx context.Context) error {
	sem := make(chan struct{}, maxConcurrentTunnels)
	for {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}

		local, err := uc.Listener.Accept(ctx)
		if err != nil {
			<-sem
			return err
		}

		stream, err := uc.Tunnel.OpenStream(ctx)
		if err != nil {
			_ = local.Close()
			<-sem
			continue
		}

		go func() {
			defer func() { <-sem }()
			pipe(local, stream)
		}()
	}
}
