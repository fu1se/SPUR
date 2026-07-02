package sqlite_test

import (
	"context"
	"fmt"
	"net/netip"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/localizator/internal/adapter/repository/sqlite"
	"github.com/fu1se/localizator/internal/domain"
)

func TestNetworkRepository_UpdateCreatesOnFirstCall(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer db.Close()

	repo := sqlite.NewNetworkRepository(db)

	cidr := netip.MustParsePrefix("100.64.0.0/10")
	updated, err := repo.Update(context.Background(), "home", func(n domain.Network) (domain.Network, error) {
		require.False(t, n.CIDR.IsValid(), "network shouldn't exist yet")
		n.CIDR = cidr
		n.InviteToken = "tok"
		return n, nil
	})
	require.NoError(t, err)
	require.Equal(t, cidr, updated.CIDR)

	found, err := repo.FindByName(context.Background(), "home")
	require.NoError(t, err)
	require.Equal(t, cidr, found.CIDR)
	require.Equal(t, "tok", found.InviteToken)
}

func TestNetworkRepository_FindByNameMissingReturnsErrNetworkNotFound(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer db.Close()

	_, err = sqlite.NewNetworkRepository(db).FindByName(context.Background(), "nope")
	require.ErrorIs(t, err, domain.ErrNetworkNotFound)
}

func TestNetworkRepository_UpdatePersistsMembers(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer db.Close()

	repo := sqlite.NewNetworkRepository(db)
	cidr := netip.MustParsePrefix("100.64.0.0/10")

	var pub domain.PublicKey
	pub[0] = 0x9
	member := domain.MeshMember{PeerID: "peer-a", PublicKey: pub, MeshIP: netip.MustParseAddr("100.64.0.1")}

	_, err = repo.Update(context.Background(), "home", func(n domain.Network) (domain.Network, error) {
		n.CIDR = cidr
		n.Members = append(n.Members, member)
		return n, nil
	})
	require.NoError(t, err)

	found, err := repo.FindByName(context.Background(), "home")
	require.NoError(t, err)
	require.Equal(t, []domain.MeshMember{member}, found.Members)
}

func TestNetworkRepository_FindByInviteToken(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer db.Close()

	repo := sqlite.NewNetworkRepository(db)
	_, err = repo.Update(context.Background(), "home", func(n domain.Network) (domain.Network, error) {
		n.CIDR = netip.MustParsePrefix("100.64.0.0/10")
		n.InviteToken = "secret-token"
		return n, nil
	})
	require.NoError(t, err)

	found, err := repo.FindByInviteToken(context.Background(), "secret-token")
	require.NoError(t, err)
	require.Equal(t, "home", found.Name)

	_, err = repo.FindByInviteToken(context.Background(), "wrong-token")
	require.ErrorIs(t, err, domain.ErrNetworkNotFound)
}

// TestNetworkRepository_ConcurrentUpdatesDontLoseMembers is the SQLite
// counterpart to usecase.TestJoinNetwork_ConcurrentJoinsDontLoseMembers —
// it drives the same read-mutate-write race directly against the
// repository to prove Update's mutex genuinely serializes writers against
// a real SQLite file, not just against the in-memory map the original
// implementation used.
func TestNetworkRepository_ConcurrentUpdatesDontLoseMembers(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer db.Close()

	repo := sqlite.NewNetworkRepository(db)

	_, err = repo.Update(context.Background(), "home", func(n domain.Network) (domain.Network, error) {
		n.CIDR = netip.MustParsePrefix("100.64.0.0/10")
		return n, nil
	})
	require.NoError(t, err)

	const peerCount = 20
	var wg sync.WaitGroup
	for i := 0; i < peerCount; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var pub domain.PublicKey
			pub[0] = byte(i)
			peerID := domain.PeerID(fmt.Sprintf("peer-%d", i))
			_, err := repo.Update(context.Background(), "home", func(n domain.Network) (domain.Network, error) {
				ip, err := n.NextAvailableIP()
				if err != nil {
					return domain.Network{}, err
				}
				n.Members = append(n.Members, domain.MeshMember{PeerID: peerID, PublicKey: pub, MeshIP: ip})
				return n, nil
			})
			require.NoError(t, err)
		}(i)
	}
	wg.Wait()

	final, err := repo.FindByName(context.Background(), "home")
	require.NoError(t, err)
	require.Len(t, final.Members, peerCount, "some concurrent updates were lost")

	seenIPs := make(map[string]bool, peerCount)
	for _, m := range final.Members {
		require.False(t, seenIPs[m.MeshIP.String()], "duplicate mesh IP assigned: %s", m.MeshIP)
		seenIPs[m.MeshIP.String()] = true
	}
}
