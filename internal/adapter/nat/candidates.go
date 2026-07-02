package nat

import (
	"fmt"
	"net"
	"net/netip"

	"github.com/fu1se/spur/internal/domain"
)

// HostCandidates enumerates local unicast addresses bound to conn's port.
// Loopback is included deliberately: it costs nothing for genuinely remote
// peers (unreachable candidates are simply ignored during punching) and it
// is what makes two local processes on the same machine punch each other
// during development and in CI, where a routable LAN interface may not
// exist.
func HostCandidates(conn *net.UDPConn) ([]domain.Candidate, error) {
	localAddr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return nil, fmt.Errorf("nat: unexpected local addr type %T", conn.LocalAddr())
	}
	port := uint16(localAddr.Port)

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, fmt.Errorf("nat: interface addrs: %w", err)
	}

	var candidates []domain.Candidate
	for _, a := range addrs {
		ipNet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}

		ip, ok := netip.AddrFromSlice(ipNet.IP.To4())
		if !ok {
			ip, ok = netip.AddrFromSlice(ipNet.IP.To16())
			if !ok {
				continue
			}
		}
		if ip.IsLinkLocalUnicast() || ip.IsMulticast() {
			continue
		}

		candidates = append(candidates, domain.Candidate{
			Kind: domain.CandidateHost,
			Addr: netip.AddrPortFrom(ip.Unmap(), port),
		})
	}

	return candidates, nil
}
