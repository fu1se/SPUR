package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/fu1se/spur/internal/domain"
)

// RoomRepository is a SQLite-backed port.RoomRepository. mu serializes
// Update the same way NetworkRepository's does — see that type's doc
// comment for why the mutex is still needed alongside a SQL transaction.
type RoomRepository struct {
	db *sql.DB
	mu sync.Mutex
}

func NewRoomRepository(db *sql.DB) *RoomRepository {
	return &RoomRepository{db: db}
}

func (r *RoomRepository) FindByName(ctx context.Context, name string) (domain.Room, error) {
	return r.load(ctx, name)
}

func (r *RoomRepository) load(ctx context.Context, name string) (domain.Room, error) {
	row := r.db.QueryRowContext(ctx, `SELECT name, invite_token FROM rooms WHERE name = ?`, name)

	room := domain.Room{}
	if err := row.Scan(&room.Name, &room.InviteToken); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Room{}, domain.ErrRoomNotFound
		}
		return domain.Room{}, fmt.Errorf("sqlite: find room: %w", err)
	}

	members, err := r.loadMembers(ctx, room.Name)
	if err != nil {
		return domain.Room{}, err
	}
	room.Members = members

	return room, nil
}

func (r *RoomRepository) loadMembers(ctx context.Context, roomName string) ([]domain.PeerID, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT peer_id FROM room_members WHERE room_name = ?`, roomName)
	if err != nil {
		return nil, fmt.Errorf("sqlite: load room members: %w", err)
	}
	defer rows.Close()

	var members []domain.PeerID
	for rows.Next() {
		var peerID string
		if err := rows.Scan(&peerID); err != nil {
			return nil, fmt.Errorf("sqlite: scan room member: %w", err)
		}
		members = append(members, domain.PeerID(peerID))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate room members: %w", err)
	}
	return members, nil
}

func (r *RoomRepository) Update(ctx context.Context, name string, mutate func(domain.Room) (domain.Room, error)) (domain.Room, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	current, err := r.load(ctx, name)
	if err != nil {
		if !errors.Is(err, domain.ErrRoomNotFound) {
			return domain.Room{}, err
		}
		current = domain.Room{Name: name}
	}

	updated, err := mutate(current)
	if err != nil {
		return domain.Room{}, err
	}

	if reflect.DeepEqual(current, updated) {
		// No-op update: see NetworkRepository.Update's identical check for
		// why this matters — a rejoin by an already-known member
		// shouldn't unconditionally delete-and-reinsert every row.
		return updated, nil
	}

	if err := r.save(ctx, updated); err != nil {
		return domain.Room{}, err
	}
	return updated, nil
}

func (r *RoomRepository) save(ctx context.Context, room domain.Room) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO rooms (name, invite_token)
		VALUES (?, ?)
		ON CONFLICT (name) DO UPDATE SET invite_token = excluded.invite_token
	`, room.Name, room.InviteToken); err != nil {
		return fmt.Errorf("sqlite: save room: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM room_members WHERE room_name = ?`, room.Name); err != nil {
		return fmt.Errorf("sqlite: clear room members: %w", err)
	}

	for _, peerID := range room.Members {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO room_members (room_name, peer_id) VALUES (?, ?)
		`, room.Name, string(peerID)); err != nil {
			return fmt.Errorf("sqlite: save room member %s: %w", peerID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit: %w", err)
	}
	return nil
}
