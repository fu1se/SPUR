package memory

import (
	"context"
	"sync"

	"github.com/fu1se/spur/internal/domain"
)

// NetworkRepository is a thread-safe in-memory implementation of
// port.NetworkRepository. A single mutex serializes all reads and writes
// — Update needs this to make its read-mutate-write atomic (see the
// port's doc comment), and a single global lock is a fine MVP choice
// while everything is in-memory and traffic is low; a per-network lock
// would be the natural upgrade if this ever becomes a bottleneck.
type NetworkRepository struct {
	mu       sync.Mutex
	networks map[string]domain.Network
}

func NewNetworkRepository() *NetworkRepository {
	return &NetworkRepository{networks: make(map[string]domain.Network)}
}

func (r *NetworkRepository) FindByName(_ context.Context, name string) (domain.Network, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	network, ok := r.networks[name]
	if !ok {
		return domain.Network{}, domain.ErrNetworkNotFound
	}
	return network, nil
}

// FindByInviteToken is not yet meaningful: invite tokens are introduced in
// the auth phase (CLAUDE.md roadmap, Phase 7). Networks are currently
// joinable by name alone.
func (r *NetworkRepository) FindByInviteToken(_ context.Context, _ string) (domain.Network, error) {
	return domain.Network{}, domain.ErrNetworkNotFound
}

func (r *NetworkRepository) Update(_ context.Context, name string, mutate func(domain.Network) (domain.Network, error)) (domain.Network, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	current, ok := r.networks[name]
	if !ok {
		current = domain.Network{Name: name}
	}

	updated, err := mutate(current)
	if err != nil {
		return domain.Network{}, err
	}

	r.networks[name] = updated
	return updated, nil
}
