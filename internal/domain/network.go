package domain

import (
	"errors"
	"net/netip"
)

// ErrNetworkAddressSpaceExhausted is returned when a Network's CIDR has no
// free address left to assign to a new member.
var ErrNetworkAddressSpaceExhausted = errors.New("domain: network address space exhausted")

// MeshMember is one peer's membership record within a Network: its
// identity, the public key other members need to set up a WireGuard peer
// for it, and the mesh-internal IP it was assigned.
type MeshMember struct {
	PeerID    PeerID
	PublicKey PublicKey
	MeshIP    netip.Addr
}

// Network is a named mesh of peers sharing a private address space. It is
// only meaningful in mesh VPN mode (TunnelMesh); plain port-forward
// sessions do not belong to a Network.
type Network struct {
	Name        string
	CIDR        netip.Prefix
	InviteToken string
	Members     []MeshMember
}

// HasMember reports whether peer already belongs to the network.
func (n Network) HasMember(peer PeerID) bool {
	for _, m := range n.Members {
		if m.PeerID == peer {
			return true
		}
	}
	return false
}

// NextAvailableIP returns the first address in CIDR not already assigned
// to a member, skipping the network address itself (CIDR.Addr()).
func (n Network) NextAvailableIP() (netip.Addr, error) {
	used := make(map[netip.Addr]bool, len(n.Members))
	for _, m := range n.Members {
		used[m.MeshIP] = true
	}

	for addr := n.CIDR.Addr().Next(); n.CIDR.Contains(addr); addr = addr.Next() {
		if !used[addr] {
			return addr, nil
		}
	}
	return netip.Addr{}, ErrNetworkAddressSpaceExhausted
}
