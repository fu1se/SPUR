package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/fu1se/spur/internal/domain"
)

// PeerRepository is a SQLite-backed port.PeerRepository.
type PeerRepository struct {
	db *sql.DB
}

func NewPeerRepository(db *sql.DB) *PeerRepository {
	return &PeerRepository{db: db}
}

func (r *PeerRepository) Save(ctx context.Context, peer domain.Peer) error {
	candidatesJSON, err := json.Marshal(peer.Candidates)
	if err != nil {
		return fmt.Errorf("sqlite: marshal candidates: %w", err)
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO peers (id, public_key, last_seen_at, candidates)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			public_key = excluded.public_key,
			last_seen_at = excluded.last_seen_at,
			candidates = excluded.candidates
	`, string(peer.ID), peer.PublicKey[:], peer.LastSeenAt.UnixNano(), string(candidatesJSON))
	if err != nil {
		return fmt.Errorf("sqlite: save peer: %w", err)
	}
	return nil
}

func (r *PeerRepository) FindByID(ctx context.Context, id domain.PeerID) (domain.Peer, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT public_key, last_seen_at, candidates FROM peers WHERE id = ?
	`, string(id))

	var (
		pubKey        []byte
		lastSeenAtNs  int64
		candidatesRaw string
	)
	if err := row.Scan(&pubKey, &lastSeenAtNs, &candidatesRaw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Peer{}, domain.ErrPeerNotFound
		}
		return domain.Peer{}, fmt.Errorf("sqlite: find peer: %w", err)
	}

	peer, err := scanPeer(id, pubKey, lastSeenAtNs, candidatesRaw)
	if err != nil {
		return domain.Peer{}, err
	}
	return peer, nil
}

// ListByNetwork is not yet meaningful here either: peer-to-network
// membership lives in NetworkRepository's members, not on the peer record
// itself (see domain.MeshMember) — same "not implemented" stance as
// adapter/repository/memory's PeerRepository takes, for the same reason.
func (r *PeerRepository) ListByNetwork(_ context.Context, _ string) ([]domain.Peer, error) {
	return nil, nil
}

func scanPeer(id domain.PeerID, pubKeyBytes []byte, lastSeenAtNs int64, candidatesRaw string) (domain.Peer, error) {
	if len(pubKeyBytes) != len(domain.PublicKey{}) {
		return domain.Peer{}, fmt.Errorf("sqlite: corrupt public key length for peer %s", id)
	}
	var pub domain.PublicKey
	copy(pub[:], pubKeyBytes)

	var candidates []domain.Candidate
	if err := json.Unmarshal([]byte(candidatesRaw), &candidates); err != nil {
		return domain.Peer{}, fmt.Errorf("sqlite: unmarshal candidates for peer %s: %w", id, err)
	}

	return domain.Peer{
		ID:         id,
		PublicKey:  pub,
		Candidates: candidates,
		LastSeenAt: time.Unix(0, lastSeenAtNs),
	}, nil
}
