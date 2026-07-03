// Package port declares the interfaces (ports) that the use case layer
// depends on. Use cases import only this package and internal/domain, never
// a concrete adapter or third-party library directly — concrete
// implementations live under internal/adapter and are wired together in
// cmd/spur (the composition root).
package port

import (
	"context"

	"github.com/fu1se/spur/internal/domain"
)

// PeerRepository persists and looks up peers known to the server.
type PeerRepository interface {
	Save(ctx context.Context, peer domain.Peer) error
	FindByID(ctx context.Context, id domain.PeerID) (domain.Peer, error)
	ListByNetwork(ctx context.Context, network string) ([]domain.Peer, error)
}

// NetworkRepository persists mesh network definitions and membership.
type NetworkRepository interface {
	FindByName(ctx context.Context, name string) (domain.Network, error)
	FindByInviteToken(ctx context.Context, token string) (domain.Network, error)

	// Update atomically loads the network named name (or a zero-value
	// domain.Network{Name: name} if none exists yet — check
	// network.CIDR.IsValid() to tell the difference), applies mutate, and
	// persists the result. Implementations must serialize concurrent
	// Update calls for the same name: two peers joining at once must
	// never both observe the same pre-mutation membership list, or one
	// join silently overwrites the other's (see CLAUDE.md's mesh-join
	// race note — this replaced a separate Save method for exactly that
	// reason).
	Update(ctx context.Context, name string, mutate func(domain.Network) (domain.Network, error)) (domain.Network, error)
}

// SessionRepository persists in-flight and established tunnel sessions.
type SessionRepository interface {
	Save(ctx context.Context, session domain.Session) error
	FindByID(ctx context.Context, id string) (domain.Session, error)
}

// RoomRepository persists long-lived, two-member room definitions.
type RoomRepository interface {
	FindByName(ctx context.Context, name string) (domain.Room, error)

	// Update atomically loads the room named name (or a zero-value
	// domain.Room{Name: name} if none exists yet — check len(room.Members)
	// == 0 && room.InviteToken == "" to tell the difference), applies
	// mutate, and persists the result. Implementations must serialize
	// concurrent Update calls for the same name — same atomicity
	// requirement, and the same reason, as NetworkRepository.Update.
	Update(ctx context.Context, name string, mutate func(domain.Room) (domain.Room, error)) (domain.Room, error)
}
