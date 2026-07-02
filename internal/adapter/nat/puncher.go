package nat

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/fu1se/spur/internal/domain"
)

const (
	punchMagic    = "spur-punch1:"
	punchInterval = 150 * time.Millisecond
	punchTimeout  = 10 * time.Second
	punchAckSends = 3
)

// UDPPuncher implements port.Puncher over an already-bound UDP socket: it
// sends a small marker payload to every candidate repeatedly while
// listening for the same marker coming back, and returns the first address
// that round-trips. sessionID scopes the marker so unrelated traffic on
// the same socket during the same window can't be mistaken for a punch.
type UDPPuncher struct {
	Conn      *net.UDPConn
	SessionID string
}

// Punch blocks until a path to one of candidates is confirmed bidirectional,
// ctx is cancelled, or the internal punch timeout elapses.
//
// Conn is handed off to a QUIC listener/dialer right after Punch returns
// (see adapter/tunnel.Transport) — reusing the same socket is what keeps
// the punched NAT mapping valid. That handoff is exactly why this method
// cannot just return as soon as it has an answer: recvLoop calls
// SetReadDeadline on every iteration, and if Punch returned while recvLoop
// was still mid-flight, a deadline set moments earlier could still be in
// effect (or about to be re-armed) when the caller starts using Conn for
// QUIC — every read then fails instantly with an expired-deadline error
// instead of blocking, which starves QUIC's receive loop and looks like a
// runaway retransmit storm (high CPU, a growing unread recv queue). So
// Punch explicitly waits for both goroutines to fully exit before it
// clears the deadline and returns.
func (p *UDPPuncher) Punch(ctx context.Context, candidates []domain.Candidate) (netip.AddrPort, error) {
	if len(candidates) == 0 {
		return netip.AddrPort{}, errors.New("nat: no candidates to punch")
	}

	ctx, cancel := context.WithTimeout(ctx, punchTimeout)
	defer cancel()

	payload := []byte(punchMagic + p.SessionID)

	result := make(chan netip.AddrPort, 1)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); p.sendLoop(ctx, candidates, payload) }()
	go func() { defer wg.Done(); p.recvLoop(ctx, payload, result) }()

	var addr netip.AddrPort
	var punchErr error
	select {
	case addr = <-result:
	case <-ctx.Done():
		punchErr = fmt.Errorf("nat: punch did not succeed: %w", ctx.Err())
	}

	cancel() // wake both loops now, don't wait for the deferred cancel on return
	wg.Wait()
	_ = p.Conn.SetReadDeadline(time.Time{})

	return addr, punchErr
}

func (p *UDPPuncher) sendLoop(ctx context.Context, candidates []domain.Candidate, payload []byte) {
	ticker := time.NewTicker(punchInterval)
	defer ticker.Stop()

	for {
		for _, c := range candidates {
			_, _ = p.Conn.WriteToUDPAddrPort(payload, c.Addr)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (p *UDPPuncher) recvLoop(ctx context.Context, payload []byte, result chan<- netip.AddrPort) {
	buf := make([]byte, 1500)

	for {
		if ctx.Err() != nil {
			return
		}

		_ = p.Conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		n, from, err := p.Conn.ReadFromUDPAddrPort(buf)
		if err != nil {
			continue // read timeout or transient error; loop and recheck ctx
		}
		if n != len(payload) || string(buf[:n]) != string(payload) {
			continue // not our marker (or a stale one) — ignore
		}

		// Ack a few times so the counterpart's recvLoop sees a reply even
		// if one of our packets is lost.
		for i := 0; i < punchAckSends; i++ {
			_, _ = p.Conn.WriteToUDPAddrPort(payload, from)
		}

		select {
		case result <- from:
		default:
		}
		return
	}
}
