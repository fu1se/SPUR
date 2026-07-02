package port

import (
	"context"

	"github.com/fu1se/localizator/internal/domain"
)

// NetworkJoiner is the client's view of joining a mesh network over the
// control channel: it announces the caller's identity and gets back the
// current network (its assigned mesh IP is among the returned members).
type NetworkJoiner interface {
	JoinNetwork(ctx context.Context, networkName string, pub domain.PublicKey) (domain.Network, error)
}
