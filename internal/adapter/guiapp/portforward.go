package guiapp

import (
	"context"
	"fmt"

	"github.com/fu1se/spur/internal/adapter/cli"
	"github.com/fu1se/spur/internal/adapter/localnet"
	"github.com/fu1se/spur/internal/adapter/rendezvous"
	"github.com/fu1se/spur/internal/usecase"
)

// PortForward is a running "connect"/"expose" session (see
// Client.StartConnect/StartExpose). The tunnel is already established by
// the time a caller gets one back; the forwarding loop then runs in a
// background goroutine until Stop is called or it fails on its own (the
// counterpart disconnecting, say) — Await reports which. Mirrors
// spurmobile.PortForward exactly, minus the gomobile naming workaround
// (no java.lang.Object.wait() collision to avoid here, so this uses the
// ordinary name Wait).
type PortForward struct {
	cancel context.CancelFunc
	done   chan error

	// LocalAddr is the actual local address StartConnect's listener bound
	// to (only set for StartConnect — StartExpose has no local listener of
	// its own to report). Useful when localPort was 0 (let the OS pick a
	// free port): the caller has no other way to learn which one it got.
	LocalAddr string
}

// Stop tears down the tunnel. Safe to call more than once.
func (p *PortForward) Stop() { p.cancel() }

// Wait blocks until the forwarding loop ends — either Stop was called,
// or the tunnel failed on its own — returning the reason: a
// Stop-initiated shutdown surfaces as context.Canceled (usecase.ForwardPort/
// ServeExposedPort.Run returns the run context's own Err()), not nil, so
// callers that only care about "did it fail unexpectedly" should check
// errors.Is(err, context.Canceled) rather than err == nil. Call this from
// a background goroutine to learn when a session ends without polling.
func (p *PortForward) Wait() error { return <-p.done }

// StartConnect is "spur connect": forwards every local connection on
// localPort through a tunnel to whichever counterpart to/room resolves
// to — see rendezvous.CounterpartResolverFor for the three ways that can
// be specified (leave both to and room empty for "host" mode: onCode
// reports a fresh pairing code to show the user). Blocks until the
// tunnel is established (NAT punching, possibly relay fallback — can
// take up to the usual establish timeouts) before returning, so call
// this from a background goroutine, not the GUI's event-dispatch thread.
func (c *Client) StartConnect(ctx context.Context, serverAddr, stunAddr, to, room string, localPort int, onSelfID func(string), onCode cli.OnCodeFunc, onVersionMismatch cli.VersionMismatchFunc) (*PortForward, error) {
	resolve := rendezvous.CounterpartResolverFor(to, room, rendezvous.OnCodeFunc(onCode))
	tun, _, _, err := rendezvous.Establish(ctx, serverAddr, stunAddr, c.identityPath, "", cli.Version(), resolve, onSelfID, rendezvous.VersionMismatchFunc(onVersionMismatch))
	if err != nil {
		return nil, err
	}

	listener, err := localnet.ListenTCP(fmt.Sprintf(":%d", localPort))
	if err != nil {
		tun.Close()
		return nil, fmt.Errorf("guiapp: listen locally: %w", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	pf := &PortForward{cancel: cancel, done: make(chan error, 1), LocalAddr: listener.Addr().String()}
	go func() {
		runErr := usecase.ForwardPort{Listener: listener, Tunnel: tun.Conn}.Run(runCtx)
		tun.Close()
		listener.Close()
		pf.done <- runErr
	}()
	return pf, nil
}

// StartExpose is "spur expose": accepts tunnel streams from the resolved
// counterpart and forwards each to targetPort on localhost. See
// StartConnect for to/room/onCode and the blocking/threading contract.
func (c *Client) StartExpose(ctx context.Context, serverAddr, stunAddr, to, room string, targetPort int, onSelfID func(string), onCode cli.OnCodeFunc, onVersionMismatch cli.VersionMismatchFunc) (*PortForward, error) {
	resolve := rendezvous.CounterpartResolverFor(to, room, rendezvous.OnCodeFunc(onCode))
	tun, _, _, err := rendezvous.Establish(ctx, serverAddr, stunAddr, c.identityPath, "", cli.Version(), resolve, onSelfID, rendezvous.VersionMismatchFunc(onVersionMismatch))
	if err != nil {
		return nil, err
	}

	dialer := localnet.TCPDialer{Addr: fmt.Sprintf("127.0.0.1:%d", targetPort)}

	runCtx, cancel := context.WithCancel(context.Background())
	pf := &PortForward{cancel: cancel, done: make(chan error, 1)}
	go func() {
		runErr := usecase.ServeExposedPort{Dialer: dialer, Tunnel: tun.Conn}.Run(runCtx)
		tun.Close()
		pf.done <- runErr
	}()
	return pf, nil
}
