package wgmesh

import (
	"crypto/ecdh"
	"encoding/hex"
	"fmt"
	"net/netip"
	"strings"

	"github.com/fu1se/localizator/internal/domain"
)

// PeerConfig is one WireGuard peer entry: its identity key, the mesh
// address routed to it, and the Endpoint string our Bind will resolve
// (via ParseEndpoint) back to a domain.PeerID — see Bind's doc comment for
// why that's a peer ID rather than an IP:port.
type PeerConfig struct {
	PublicKey domain.PublicKey
	AllowedIP netip.Prefix
	Endpoint  string
}

// BuildUAPIConfig renders a wireguard-go UAPI config string (as consumed
// by device.Device.IpcSet) for priv and peers. Keys are lowercase hex per
// the UAPI protocol (see wireguard-go's device/uapi.go).
func BuildUAPIConfig(priv *ecdh.PrivateKey, peers []PeerConfig) string {
	var b strings.Builder

	fmt.Fprintf(&b, "private_key=%s\n", hex.EncodeToString(priv.Bytes()))
	fmt.Fprintf(&b, "listen_port=0\n")

	for _, p := range peers {
		fmt.Fprintf(&b, "public_key=%s\n", hex.EncodeToString(p.PublicKey[:]))
		fmt.Fprintf(&b, "allowed_ip=%s\n", p.AllowedIP)
		fmt.Fprintf(&b, "endpoint=%s\n", p.Endpoint)
	}

	return b.String()
}
