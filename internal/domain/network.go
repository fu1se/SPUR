package domain

import "net/netip"

// Network is a named mesh of peers sharing a private address space. It is
// only meaningful in mesh VPN mode (TunnelMesh); plain port-forward
// sessions do not belong to a Network.
type Network struct {
	Name        string
	CIDR        netip.Prefix
	InviteToken string
	MemberIDs   []PeerID
}
