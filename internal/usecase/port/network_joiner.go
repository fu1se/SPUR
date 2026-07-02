package port

import (
	"context"

	"github.com/fu1se/spur/internal/domain"
)

// NetworkJoiner is the client's view of joining a mesh network over the
// control channel: it announces the caller's identity (and, for an
// existing network, its invite token) and gets back the current network
// (its assigned mesh IP is among the returned members). inviteToken is
// only checked when joining a network that already exists and the caller
// isn't already a member — creating a new network or rejoining a known
// one doesn't need it.
type NetworkJoiner interface {
	JoinNetwork(ctx context.Context, networkName, inviteToken string, pub domain.PublicKey) (domain.Network, error)
}
