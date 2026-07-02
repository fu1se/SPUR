package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/netip"
	"sync"

	"github.com/fu1se/localizator/internal/domain"
)

// NetworkRepository is a SQLite-backed port.NetworkRepository.
//
// mu serializes Update the same way adapter/repository/memory's single
// mutex does — see that package's doc comment. It is still needed even
// though the actual writes happen inside a SQL transaction: the mutex
// makes the whole "read current network, run the caller's mutate
// callback, write the result back" sequence atomic at the Go level, which
// a bare SQL transaction alone would not give us here, since mutate can
// return an error partway through and there is no SQL-expressible
// equivalent of "atomically compute domain.Network.NextAvailableIP against
// whatever the row currently holds" — the callback has to run in Go.
type NetworkRepository struct {
	db *sql.DB
	mu sync.Mutex
}

func NewNetworkRepository(db *sql.DB) *NetworkRepository {
	return &NetworkRepository{db: db}
}

func (r *NetworkRepository) FindByName(ctx context.Context, name string) (domain.Network, error) {
	return r.load(ctx, "name", name)
}

func (r *NetworkRepository) FindByInviteToken(ctx context.Context, token string) (domain.Network, error) {
	return r.load(ctx, "invite_token", token)
}

func (r *NetworkRepository) load(ctx context.Context, column, value string) (domain.Network, error) {
	row := r.db.QueryRowContext(ctx, fmt.Sprintf(`
		SELECT name, cidr, invite_token FROM networks WHERE %s = ?
	`, column), value)

	var cidrRaw string
	network := domain.Network{}
	if err := row.Scan(&network.Name, &cidrRaw, &network.InviteToken); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Network{}, domain.ErrNetworkNotFound
		}
		return domain.Network{}, fmt.Errorf("sqlite: find network: %w", err)
	}

	cidr, err := netip.ParsePrefix(cidrRaw)
	if err != nil {
		return domain.Network{}, fmt.Errorf("sqlite: corrupt cidr for network %s: %w", network.Name, err)
	}
	network.CIDR = cidr

	members, err := r.loadMembers(ctx, network.Name)
	if err != nil {
		return domain.Network{}, err
	}
	network.Members = members

	return network, nil
}

func (r *NetworkRepository) loadMembers(ctx context.Context, networkName string) ([]domain.MeshMember, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT peer_id, public_key, mesh_ip FROM network_members WHERE network_name = ?
	`, networkName)
	if err != nil {
		return nil, fmt.Errorf("sqlite: load members: %w", err)
	}
	defer rows.Close()

	var members []domain.MeshMember
	for rows.Next() {
		var (
			peerID    string
			pubKey    []byte
			meshIPRaw string
		)
		if err := rows.Scan(&peerID, &pubKey, &meshIPRaw); err != nil {
			return nil, fmt.Errorf("sqlite: scan member: %w", err)
		}
		if len(pubKey) != len(domain.PublicKey{}) {
			return nil, fmt.Errorf("sqlite: corrupt public key length for member %s", peerID)
		}
		var pub domain.PublicKey
		copy(pub[:], pubKey)

		meshIP, err := netip.ParseAddr(meshIPRaw)
		if err != nil {
			return nil, fmt.Errorf("sqlite: corrupt mesh ip for member %s: %w", peerID, err)
		}

		members = append(members, domain.MeshMember{PeerID: domain.PeerID(peerID), PublicKey: pub, MeshIP: meshIP})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate members: %w", err)
	}
	return members, nil
}

func (r *NetworkRepository) Update(ctx context.Context, name string, mutate func(domain.Network) (domain.Network, error)) (domain.Network, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	current, err := r.load(ctx, "name", name)
	if err != nil {
		if !errors.Is(err, domain.ErrNetworkNotFound) {
			return domain.Network{}, err
		}
		current = domain.Network{Name: name}
	}

	updated, err := mutate(current)
	if err != nil {
		return domain.Network{}, err
	}

	if err := r.save(ctx, updated); err != nil {
		return domain.Network{}, err
	}
	return updated, nil
}

func (r *NetworkRepository) save(ctx context.Context, network domain.Network) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO networks (name, cidr, invite_token)
		VALUES (?, ?, ?)
		ON CONFLICT (name) DO UPDATE SET
			cidr = excluded.cidr,
			invite_token = excluded.invite_token
	`, network.Name, network.CIDR.String(), network.InviteToken); err != nil {
		return fmt.Errorf("sqlite: save network: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM network_members WHERE network_name = ?`, network.Name); err != nil {
		return fmt.Errorf("sqlite: clear members: %w", err)
	}

	for _, m := range network.Members {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO network_members (network_name, peer_id, public_key, mesh_ip)
			VALUES (?, ?, ?, ?)
		`, network.Name, string(m.PeerID), m.PublicKey[:], m.MeshIP.String()); err != nil {
			return fmt.Errorf("sqlite: save member %s: %w", m.PeerID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit: %w", err)
	}
	return nil
}
