// Package nat implements port.Puncher plus the supporting candidate
// gathering: host candidates from local interfaces and a server-reflexive
// candidate via STUN, both obtained on the same UDP socket that Puncher
// later punches with — this is what makes the discovered mapping valid for
// punching in the first place.
package nat

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"time"

	"github.com/pion/stun/v3"

	"github.com/fu1se/spur/internal/domain"
)

const stunTimeout = 3 * time.Second

// DiscoverServerReflexive sends a single STUN Binding request to stunServer
// over conn and returns the resulting server-reflexive candidate.
func DiscoverServerReflexive(ctx context.Context, conn *net.UDPConn, stunServer netip.AddrPort) (domain.Candidate, error) {
	req, err := stun.Build(stun.TransactionID, stun.BindingRequest)
	if err != nil {
		return domain.Candidate{}, fmt.Errorf("nat: build stun request: %w", err)
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(stunTimeout)
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return domain.Candidate{}, fmt.Errorf("nat: set deadline: %w", err)
	}
	defer conn.SetDeadline(time.Time{}) //nolint:errcheck // best-effort deadline clear

	if _, err := conn.WriteToUDPAddrPort(req.Raw, stunServer); err != nil {
		return domain.Candidate{}, fmt.Errorf("nat: send stun request: %w", err)
	}

	// Only trust a response that both came from the STUN server we asked
	// (not just any source that got a packet onto this socket first) and
	// answers our own transaction: without this, anyone able to race a
	// forged UDP packet onto the socket within stunTimeout could poison
	// this client's belief about its own public ip:port, redirecting
	// where a counterpart later attempts to punch.
	buf := make([]byte, 1500)
	var res *stun.Message
	for {
		n, from, err := conn.ReadFromUDPAddrPort(buf)
		if err != nil {
			return domain.Candidate{}, fmt.Errorf("nat: read stun response: %w", err)
		}
		// Unmap before comparing: a dual-stack ("::") socket reports an
		// IPv4 peer's address as a v4-in-v6-mapped address
		// (::ffff:a.b.c.d), which compares unequal to the plain IPv4
		// stunServer AddrPort despite being the same endpoint.
		if netip.AddrPortFrom(from.Addr().Unmap(), from.Port()) != netip.AddrPortFrom(stunServer.Addr().Unmap(), stunServer.Port()) {
			continue
		}
		msg := &stun.Message{Raw: append([]byte(nil), buf[:n]...)}
		if err := msg.Decode(); err != nil {
			continue
		}
		if msg.TransactionID != req.TransactionID {
			continue
		}
		res = msg
		break
	}

	var xorAddr stun.XORMappedAddress
	if err := xorAddr.GetFrom(res); err != nil {
		return domain.Candidate{}, fmt.Errorf("nat: no xor-mapped-address in stun response: %w", err)
	}

	ip, ok := netip.AddrFromSlice(xorAddr.IP)
	if !ok {
		return domain.Candidate{}, fmt.Errorf("nat: invalid stun mapped ip %v", xorAddr.IP)
	}

	return domain.Candidate{
		Kind: domain.CandidateServerReflexive,
		Addr: netip.AddrPortFrom(ip.Unmap(), uint16(xorAddr.Port)),
	}, nil
}
