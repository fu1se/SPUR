package memory

import (
	"context"
	"io"
	"sync"
)

// RelayBroker is a thread-safe in-memory implementation of
// port.RelayBroker. The first caller for a sessionID blocks waiting for a
// counterpart; the second caller claims the pairing and splices the two
// connections together (bidirectional io.Copy) until either side closes
// or errors, then wakes the first caller with the result.
type RelayBroker struct {
	mu      sync.Mutex
	waiting map[string]io.ReadWriteCloser
	done    map[string]chan error
}

func NewRelayBroker() *RelayBroker {
	return &RelayBroker{
		waiting: make(map[string]io.ReadWriteCloser),
		done:    make(map[string]chan error),
	}
}

func (b *RelayBroker) Join(ctx context.Context, sessionID string, conn io.ReadWriteCloser) error {
	b.mu.Lock()
	other, ok := b.waiting[sessionID]
	if !ok {
		doneCh := make(chan error, 1)
		b.waiting[sessionID] = conn
		b.done[sessionID] = doneCh
		b.mu.Unlock()

		select {
		case err := <-doneCh:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	doneCh := b.done[sessionID]
	delete(b.waiting, sessionID)
	delete(b.done, sessionID)
	b.mu.Unlock()

	err := splice(other, conn)
	doneCh <- err
	return err
}

// splice copies bytes in both directions between a and b until one
// direction ends, then closes both so the other direction's blocked Read
// unblocks too.
func splice(a, b io.ReadWriteCloser) error {
	errCh := make(chan error, 2)
	go func() { _, err := io.Copy(a, b); errCh <- err }()
	go func() { _, err := io.Copy(b, a); errCh <- err }()

	err := <-errCh
	_ = a.Close()
	_ = b.Close()
	<-errCh

	return err
}
