package memory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/spur/internal/domain"
)

// TestNetworkRepository_FindByInviteToken guards against the stale stub
// found in a security audit: FindByInviteToken used to unconditionally
// return ErrNetworkNotFound, diverging from the real sqlite
// implementation this package doubles for in usecase/adapter unit tests.
func TestNetworkRepository_FindByInviteToken(t *testing.T) {
	r := NewNetworkRepository()

	_, err := r.Update(context.Background(), "home", func(n domain.Network) (domain.Network, error) {
		n.InviteToken = "secret-token"
		return n, nil
	})
	require.NoError(t, err)

	found, err := r.FindByInviteToken(context.Background(), "secret-token")
	require.NoError(t, err)
	require.Equal(t, "home", found.Name)

	_, err = r.FindByInviteToken(context.Background(), "wrong-token")
	require.ErrorIs(t, err, domain.ErrNetworkNotFound)
}
