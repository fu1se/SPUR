package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/localizator/internal/adapter/repository/sqlite"
	"github.com/fu1se/localizator/internal/domain"
)

func TestPeerRepository_SaveAndFindByID(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer db.Close()

	repo := sqlite.NewPeerRepository(db)

	var pub domain.PublicKey
	pub[0] = 0xAB
	peer := domain.Peer{
		ID:         domain.DerivePeerID(pub),
		PublicKey:  pub,
		Candidates: []domain.Candidate{{Kind: domain.CandidateHost, Addr: mustAddrPort("192.168.1.5:4000")}},
		LastSeenAt: time.Now().Truncate(time.Second),
	}

	require.NoError(t, repo.Save(context.Background(), peer))

	found, err := repo.FindByID(context.Background(), peer.ID)
	require.NoError(t, err)
	require.Equal(t, peer.ID, found.ID)
	require.Equal(t, peer.PublicKey, found.PublicKey)
	require.Equal(t, peer.Candidates, found.Candidates)
	require.True(t, peer.LastSeenAt.Equal(found.LastSeenAt))
}

func TestPeerRepository_FindByIDMissingReturnsErrPeerNotFound(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer db.Close()

	repo := sqlite.NewPeerRepository(db)

	_, err = repo.FindByID(context.Background(), "does-not-exist")
	require.ErrorIs(t, err, domain.ErrPeerNotFound)
}

func TestPeerRepository_SaveUpsertsExistingPeer(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer db.Close()

	repo := sqlite.NewPeerRepository(db)

	var pub domain.PublicKey
	pub[0] = 0x01
	id := domain.DerivePeerID(pub)

	require.NoError(t, repo.Save(context.Background(), domain.Peer{
		ID:         id,
		PublicKey:  pub,
		Candidates: []domain.Candidate{{Kind: domain.CandidateHost, Addr: mustAddrPort("10.0.0.1:1")}},
		LastSeenAt: time.Now(),
	}))
	require.NoError(t, repo.Save(context.Background(), domain.Peer{
		ID:         id,
		PublicKey:  pub,
		Candidates: []domain.Candidate{{Kind: domain.CandidateServerReflexive, Addr: mustAddrPort("1.2.3.4:5")}},
		LastSeenAt: time.Now(),
	}))

	found, err := repo.FindByID(context.Background(), id)
	require.NoError(t, err)
	require.Len(t, found.Candidates, 1)
	require.Equal(t, domain.CandidateServerReflexive, found.Candidates[0].Kind)
}

// TestPeerRepository_PersistsAcrossReopen is the whole point of this
// package: state must survive the process (here: the *sql.DB handle)
// being closed and a fresh one opened against the same file — unlike
// adapter/repository/memory, which loses everything on restart.
func TestPeerRepository_PersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	db1, err := sqlite.Open(path)
	require.NoError(t, err)

	var pub domain.PublicKey
	pub[0] = 0x42
	id := domain.DerivePeerID(pub)
	require.NoError(t, sqlite.NewPeerRepository(db1).Save(context.Background(), domain.Peer{
		ID:         id,
		PublicKey:  pub,
		Candidates: []domain.Candidate{{Kind: domain.CandidateHost, Addr: mustAddrPort("127.0.0.1:1")}},
		LastSeenAt: time.Now(),
	}))
	require.NoError(t, db1.Close())

	db2, err := sqlite.Open(path)
	require.NoError(t, err)
	defer db2.Close()

	found, err := sqlite.NewPeerRepository(db2).FindByID(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, pub, found.PublicKey)
}
