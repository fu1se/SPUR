package port

import (
	"context"
	"net/netip"

	"github.com/fu1se/spur/internal/domain"
)

// Puncher performs simultaneous UDP hole punching against a set of remote
// candidates and returns the address a live path was established on.
type Puncher interface {
	Punch(ctx context.Context, candidates []domain.Candidate) (netip.AddrPort, error)
}
