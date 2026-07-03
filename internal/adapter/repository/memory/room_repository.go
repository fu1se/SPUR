package memory

import (
	"context"
	"sync"

	"github.com/fu1se/spur/internal/domain"
)

// RoomRepository is a thread-safe in-memory implementation of
// port.RoomRepository — the lightweight test double used by usecase/
// adapter unit tests, mirroring NetworkRepository's shape and locking
// strategy (a single mutex making Update's read-mutate-write atomic).
type RoomRepository struct {
	mu    sync.Mutex
	rooms map[string]domain.Room
}

func NewRoomRepository() *RoomRepository {
	return &RoomRepository{rooms: make(map[string]domain.Room)}
}

func (r *RoomRepository) FindByName(_ context.Context, name string) (domain.Room, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.rooms[name]
	if !ok {
		return domain.Room{}, domain.ErrRoomNotFound
	}
	return room, nil
}

func (r *RoomRepository) Update(_ context.Context, name string, mutate func(domain.Room) (domain.Room, error)) (domain.Room, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	current, ok := r.rooms[name]
	if !ok {
		current = domain.Room{Name: name}
	}

	updated, err := mutate(current)
	if err != nil {
		return domain.Room{}, err
	}

	r.rooms[name] = updated
	return updated, nil
}
