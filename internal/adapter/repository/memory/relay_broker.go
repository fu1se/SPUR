package memory

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"
)

// relayPairingTimeout bounds only the "waiting for a counterpart to join
// this session" phase, not the relay itself: once a second caller shows up
// and splice starts, that runs for the full lifetime of the tunnel with no
// timeout, same as before. Without this, a session where the two peers'
// hole-punch outcomes are asymmetric (recvLoop in nat.UDPPuncher can
// resolve for one side without the other side's punch ever landing) left
// the side that fell back to relay blocked in Join forever — no error, no
// log, nothing — whenever its counterpart resolved P2P instead and never
// called OpenChannel. It also doubled as an easy, unauthenticated resource
// leak: MethodRelay requires no prior Register, so anyone could open
// arbitrarily many never-paired relay streams. Matches
// controlserver.awaitCandidatesTimeout's value for the same kind of wait.
const relayPairingTimeout = 60 * time.Second

// RelayBroker is a thread-safe in-memory implementation of
// port.RelayBroker. The first caller for a sessionID blocks waiting for a
// counterpart; the second caller claims the pairing and splices the two
// connections together (bidirectional io.Copy) until either side closes
// or errors, then wakes the first caller with the result.
type RelayBroker struct {
	mu      sync.Mutex
	waiting map[string]io.ReadWriteCloser
	done    map[string]chan error

	// pairingTimeout defaults to relayPairingTimeout via NewRelayBroker;
	// it's a field rather than using the constant directly so tests can
	// inject a short timeout instead of waiting out the real one.
	pairingTimeout time.Duration
}

func NewRelayBroker() *RelayBroker {
	return &RelayBroker{
		waiting:        make(map[string]io.ReadWriteCloser),
		done:           make(map[string]chan error),
		pairingTimeout: relayPairingTimeout,
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

		return b.awaitPairing(ctx, sessionID, conn, doneCh)
	}

	doneCh := b.done[sessionID]
	delete(b.waiting, sessionID)
	delete(b.done, sessionID)
	b.mu.Unlock()

	err := splice(other, conn)
	doneCh <- err
	return err
}

// awaitPairing is the first caller's wait for a counterpart, bounded by
// relayPairingTimeout so it can't hang forever and so an abandoned wait
// doesn't leak its map entries.
func (b *RelayBroker) awaitPairing(ctx context.Context, sessionID string, conn io.ReadWriteCloser, doneCh chan error) error {
	pairCtx, cancel := context.WithTimeout(ctx, b.pairingTimeout)
	defer cancel()

	select {
	case err := <-doneCh:
		return err
	case <-pairCtx.Done():
		b.mu.Lock()
		if b.waiting[sessionID] != conn {
			// A second caller claimed the pairing in the window between
			// pairCtx firing and us acquiring the lock: splice has
			// already started using our conn, so don't rip the map entry
			// out from under it -- just fall through and wait
			// (unbounded, like the normal paired case) for the real
			// result.
			b.mu.Unlock()
			select {
			case err := <-doneCh:
				return err
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		delete(b.waiting, sessionID)
		delete(b.done, sessionID)
		b.mu.Unlock()

		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("memory: relay: no counterpart joined session %s within %s", sessionID, b.pairingTimeout)
	}
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
