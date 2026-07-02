package domain

// TunnelKind selects which data-plane mode a Tunnel operates in, as
// described in CLAUDE.md.
type TunnelKind string

const (
	// TunnelPortForward exposes a single remote service on a local port.
	TunnelPortForward TunnelKind = "port_forward"
	// TunnelMesh carries WireGuard traffic for full network connectivity.
	TunnelMesh TunnelKind = "mesh"
)

// Tunnel is an active data-plane channel layered on top of an established
// Session.
type Tunnel struct {
	ID        string
	SessionID string
	Kind      TunnelKind

	// LocalPort/RemotePort apply only when Kind == TunnelPortForward.
	LocalPort  int
	RemotePort int

	// NetworkName applies only when Kind == TunnelMesh.
	NetworkName string
}
