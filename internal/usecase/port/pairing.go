package port

import (
	"context"
	"time"

	"github.com/fu1se/spur/internal/domain"
)

// PairingCodeStore lets a peer ("host") register a short-lived code that
// resolves to its own peer ID, so a counterpart ("guest") can address it
// by that code instead of needing to already know the full peer ID out
// of band — the short-code, single-command connect flow. Ephemeral,
// server-side, in-memory — same nature as CandidateStore/RelayBroker
// (see adapter/repository/memory's doc comments), nothing here is worth
// persisting across a server restart.
type PairingCodeStore interface {
	// Register stores code -> host for ttl, so Resolve/AwaitUse can look
	// it up.
	Register(ctx context.Context, code string, host domain.PeerID, ttl time.Duration) error

	// Resolve returns the host peer ID code maps to
	// (domain.ErrPairingCodeNotFound if code doesn't exist or has
	// expired), and records guest as whoever is resolving it — waking up
	// a concurrent AwaitUse call for the same code.
	Resolve(ctx context.Context, code string, guest domain.PeerID) (domain.PeerID, error)

	// AwaitUse blocks until Resolve is called for code by some guest
	// (returning that guest's peer ID), or ctx is done.
	// domain.ErrPairingCodeNotFound if code was never registered or has
	// already expired.
	AwaitUse(ctx context.Context, code string) (domain.PeerID, error)
}
