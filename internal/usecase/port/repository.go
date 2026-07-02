// Package port declares the interfaces (ports) that the use case layer
// depends on. Use cases import only this package and internal/domain, never
// a concrete adapter or third-party library directly — concrete
// implementations live under internal/adapter and are wired together in
// cmd/app (the composition root).
package port

import (
	"context"

	"github.com/fu1se/localizator/internal/domain"
)

// PeerRepository persists and looks up peers known to the server.
type PeerRepository interface {
	Save(ctx context.Context, peer domain.Peer) error
	FindByID(ctx context.Context, id domain.PeerID) (domain.Peer, error)
	ListByNetwork(ctx context.Context, network string) ([]domain.Peer, error)
}

// NetworkRepository persists mesh network definitions and membership.
type NetworkRepository interface {
	Save(ctx context.Context, network domain.Network) error
	FindByName(ctx context.Context, name string) (domain.Network, error)
	FindByInviteToken(ctx context.Context, token string) (domain.Network, error)
}

// SessionRepository persists in-flight and established tunnel sessions.
type SessionRepository interface {
	Save(ctx context.Context, session domain.Session) error
	FindByID(ctx context.Context, id string) (domain.Session, error)
}
