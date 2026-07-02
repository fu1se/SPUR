package memory

import (
	"context"
	"sync"

	"github.com/fu1se/localizator/internal/domain"
)

// NetworkRepository is a thread-safe in-memory implementation of
// port.NetworkRepository.
type NetworkRepository struct {
	mu       sync.RWMutex
	networks map[string]domain.Network
}

func NewNetworkRepository() *NetworkRepository {
	return &NetworkRepository{networks: make(map[string]domain.Network)}
}

func (r *NetworkRepository) Save(_ context.Context, network domain.Network) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.networks[network.Name] = network
	return nil
}

func (r *NetworkRepository) FindByName(_ context.Context, name string) (domain.Network, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
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
