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

// FindByInviteToken mirrors sqlite.NetworkRepository's implementation. Not
// currently called by any production path (usecase.JoinNetwork checks
// network.InviteToken directly inside its Update mutate closure instead of
// going through this method), but this package also serves as the
// lightweight test double for PeerRepository/NetworkRepository in
// usecase/adapter unit tests — leaving it always-failing here would make
// this double silently diverge from the real sqlite behavior for anyone
// who does write a test against it expecting invite-token lookup to work.
func (r *NetworkRepository) FindByInviteToken(_ context.Context, token string) (domain.Network, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, network := range r.networks {
		if network.InviteToken == token {
			return network, nil
		}
	}
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
