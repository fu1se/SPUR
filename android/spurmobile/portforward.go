package spurmobile

import (
	"context"
	"fmt"

	"github.com/fu1se/spur/internal/adapter/localnet"
	"github.com/fu1se/spur/internal/adapter/rendezvous"
	"github.com/fu1se/spur/internal/usecase"
)

// CodeCallback is notified with a freshly minted pairing code when
// StartConnect/StartExpose are called in "host" mode (to and room both
// empty) — mirrors rendezvous.OnCodeFunc, but as a single-method
// interface: gomobile can only bind callbacks shaped that way, not a
// bare Go func type.
type CodeCallback interface {
	OnCode(code string)
}

func codeFunc(cb CodeCallback) rendezvous.OnCodeFunc {
	if cb == nil {
		return nil
	}
	return func(code string) { cb.OnCode(code) }
}

// PortForward is a running "connect"/"expose" session (see
// Client.StartConnect/StartExpose). The tunnel itself is already
// established by the time a caller gets one back; the forwarding loop
// then runs in a background goroutine until Stop is called or it fails
// on its own (a peer disconnecting, say) — Wait reports which.
type PortForward struct {
	cancel context.CancelFunc
	done   chan error
}

// Stop tears down the tunnel. Safe to call more than once.
func (p *PortForward) Stop() { p.cancel() }

// Await blocks until the forwarding loop ends — either Stop was called,
// or the tunnel failed on its own — returning the reason (nil for a
// clean Stop-initiated shutdown). Call this from a dedicated background
// thread/coroutine to learn when a session ends without polling. Named
// Await, not Wait: gomobile maps it to a Java method named wait(), which
// collides with java.lang.Object's final wait() and fails javac.
func (p *PortForward) Await() error { return <-p.done }

// StartConnect is "spur connect": forwards every local connection on
// localPort through a tunnel to whichever counterpart to/room resolves
// to — see rendezvous.CounterpartResolverFor for the three ways that can
// be specified (leave both to and room empty for "host" mode: onCode
// reports a fresh pairing code to show the user; nil is valid and means
// don't report). Blocks until the tunnel is established (NAT punching,
// possibly relay fallback — can take up to the usual establish
// timeouts) before returning, so call this from a background thread.
func (c *Client) StartConnect(serverAddr, stunAddr, to, room string, localPort int, onCode CodeCallback) (*PortForward, error) {
	resolve := rendezvous.CounterpartResolverFor(to, room, codeFunc(onCode))
	tun, _, _, err := rendezvous.Establish(context.Background(), serverAddr, stunAddr, c.identityPath, c.trustStorePath, Version(), resolve, func(string) {}, nil)
	if err != nil {
		return nil, err
	}

	listener, err := localnet.ListenTCP(fmt.Sprintf(":%d", localPort))
	if err != nil {
		tun.Close()
		return nil, fmt.Errorf("spurmobile: listen locally: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	pf := &PortForward{cancel: cancel, done: make(chan error, 1)}
	go func() {
		runErr := usecase.ForwardPort{Listener: listener, Tunnel: tun.Conn}.Run(ctx)
		tun.Close()
		listener.Close()
		pf.done <- runErr
	}()
	return pf, nil
}

// StartExpose is "spur expose": accepts tunnel streams from the resolved
// counterpart and forwards each to targetPort on localhost. See
// StartConnect for to/room/onCode and the blocking/threading contract.
func (c *Client) StartExpose(serverAddr, stunAddr, to, room string, targetPort int, onCode CodeCallback) (*PortForward, error) {
	resolve := rendezvous.CounterpartResolverFor(to, room, codeFunc(onCode))
	tun, _, _, err := rendezvous.Establish(context.Background(), serverAddr, stunAddr, c.identityPath, c.trustStorePath, Version(), resolve, func(string) {}, nil)
	if err != nil {
		return nil, err
	}

	dialer := localnet.TCPDialer{Addr: fmt.Sprintf("127.0.0.1:%d", targetPort)}

	ctx, cancel := context.WithCancel(context.Background())
	pf := &PortForward{cancel: cancel, done: make(chan error, 1)}
	go func() {
		runErr := usecase.ServeExposedPort{Dialer: dialer, Tunnel: tun.Conn}.Run(ctx)
		tun.Close()
		pf.done <- runErr
	}()
	return pf, nil
}
