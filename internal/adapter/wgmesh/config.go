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

// BuildDeviceConfig renders the private_key/listen_port portion of a
// wireguard-go UAPI config (as consumed by device.Device.IpcSet). Send
// this exactly once, when the device is first configured.
//
// Deliberately kept separate from peer config (see BuildPeersConfig): any
// IpcSet call containing "listen_port=" makes wireguard-go call
// BindUpdate, which closes and reopens the Bind unconditionally — even if
// the port didn't change (see wireguard-go's device/uapi.go). Our Bind's
// Close tears down every connected peer's stream (adapter/wgmesh.Bind.Close).
// Resending this on every incremental peer addition would silently
// disconnect the whole mesh each time someone new joined.
func BuildDeviceConfig(priv *ecdh.PrivateKey) string {
	var b strings.Builder
	fmt.Fprintf(&b, "private_key=%s\n", hex.EncodeToString(priv.Bytes()))
	fmt.Fprintf(&b, "listen_port=0\n")
	return b.String()
}

// BuildPeersConfig renders one or more peer entries for wireguard-go's
// UAPI (device.Device.IpcSet). Safe to call repeatedly/incrementally:
// without "replace_peers=true" (which this never sets), it only adds or
// updates the listed peers — existing ones are left alone, and critically
// it never touches listen_port, so it never triggers BindUpdate (see
// BuildDeviceConfig's doc comment).
func BuildPeersConfig(peers []PeerConfig) string {
	var b strings.Builder
	for _, p := range peers {
		fmt.Fprintf(&b, "public_key=%s\n", hex.EncodeToString(p.PublicKey[:]))
		fmt.Fprintf(&b, "allowed_ip=%s\n", p.AllowedIP)
		fmt.Fprintf(&b, "endpoint=%s\n", p.Endpoint)
	}
	return b.String()
}
