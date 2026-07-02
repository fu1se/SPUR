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

	"github.com/fu1se/localizator/internal/domain"
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

	buf := make([]byte, 1500)
	n, _, err := conn.ReadFromUDPAddrPort(buf)
	if err != nil {
		return domain.Candidate{}, fmt.Errorf("nat: read stun response: %w", err)
	}

	res := &stun.Message{Raw: buf[:n]}
	if err := res.Decode(); err != nil {
		return domain.Candidate{}, fmt.Errorf("nat: decode stun response: %w", err)
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
