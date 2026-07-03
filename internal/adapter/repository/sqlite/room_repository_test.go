package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/spur/internal/adapter/repository/sqlite"
	"github.com/fu1se/spur/internal/domain"
)

func TestRoomRepository_UpdateCreatesOnFirstCall(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer db.Close()

	repo := sqlite.NewRoomRepository(db)

	updated, err := repo.Update(context.Background(), "friends", func(r domain.Room) (domain.Room, error) {
		require.Empty(t, r.InviteToken, "room shouldn't exist yet")
		r.InviteToken = "tok"
		r.Members = []domain.PeerID{"creator"}
		return r, nil
	})
	require.NoError(t, err)
	require.Equal(t, "tok", updated.InviteToken)

	found, err := repo.FindByName(context.Background(), "friends")
	require.NoError(t, err)
	require.Equal(t, "tok", found.InviteToken)
	require.Equal(t, []domain.PeerID{"creator"}, found.Members)
}

func TestRoomRepository_FindByNameMissingReturnsErrRoomNotFound(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer db.Close()

	_, err = sqlite.NewRoomRepository(db).FindByName(context.Background(), "nope")
	require.ErrorIs(t, err, domain.ErrRoomNotFound)
}

func TestRoomRepository_UpdatePersistsMembers(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer db.Close()

	repo := sqlite.NewRoomRepository(db)

	_, err = repo.Update(context.Background(), "friends", func(r domain.Room) (domain.Room, error) {
		r.InviteToken = "tok"
		r.Members = append(r.Members, "creator")
		return r, nil
	})
	require.NoError(t, err)

	_, err = repo.Update(context.Background(), "friends", func(r domain.Room) (domain.Room, error) {
		r.Members = append(r.Members, "guest")
		return r, nil
	})
	require.NoError(t, err)

	found, err := repo.FindByName(context.Background(), "friends")
	require.NoError(t, err)
	require.ElementsMatch(t, []domain.PeerID{"creator", "guest"}, found.Members)
}

// TestRoomRepository_NoOpUpdateSkipsWrite mirrors
// TestNetworkRepository_NoOpUpdateSkipsWrite: an idempotent rejoin by an
// already-known member must not unconditionally delete-and-reinsert rows.
func TestRoomRepository_NoOpUpdateSkipsWrite(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer db.Close()

	repo := sqlite.NewRoomRepository(db)
	_, err = repo.Update(context.Background(), "friends", func(r domain.Room) (domain.Room, error) {
		r.InviteToken = "tok"
		r.Members = append(r.Members, "creator")
		return r, nil
	})
	require.NoError(t, err)

	var before int
	require.NoError(t, db.QueryRow(`SELECT total_changes()`).Scan(&before))

	_, err = repo.Update(context.Background(), "friends", func(r domain.Room) (domain.Room, error) {
		return r, nil
	})
	require.NoError(t, err)

	var after int
	require.NoError(t, db.QueryRow(`SELECT total_changes()`).Scan(&after))
	require.Equal(t, before, after, "a no-op Update should not have written anything")
}
