package infra_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/spur/internal/infra"
)

func TestLoadOrCreateIdentity_PersistsAcrossCalls(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.key")

	first, err := infra.LoadOrCreateIdentity(path)
	require.NoError(t, err)

	second, err := infra.LoadOrCreateIdentity(path)
	require.NoError(t, err)

	require.Equal(t, first.PublicKey, second.PublicKey)
	require.Equal(t, first.PrivateKey.Bytes(), second.PrivateKey.Bytes())
}

func TestLoadOrCreateIdentity_PublicKeyMatchesPrivateKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.key")

	id, err := infra.LoadOrCreateIdentity(path)
	require.NoError(t, err)

	require.Equal(t, id.PrivateKey.PublicKey().Bytes(), id.PublicKey[:])
}

func TestLoadOrCreateIdentity_DifferentPathsDifferentKeys(t *testing.T) {
	dir := t.TempDir()

	a, err := infra.LoadOrCreateIdentity(filepath.Join(dir, "a.key"))
	require.NoError(t, err)

	b, err := infra.LoadOrCreateIdentity(filepath.Join(dir, "b.key"))
	require.NoError(t, err)

	require.NotEqual(t, a.PublicKey, b.PublicKey)
}
