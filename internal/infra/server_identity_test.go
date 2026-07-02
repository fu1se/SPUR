package infra_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/localizator/internal/infra"
)

func TestLoadOrCreateServerTLSConfig_PersistsAcrossCalls(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.pem")

	first, err := infra.LoadOrCreateServerTLSConfig(path, "test-alpn")
	require.NoError(t, err)
	require.Len(t, first.Certificates, 1)

	second, err := infra.LoadOrCreateServerTLSConfig(path, "test-alpn")
	require.NoError(t, err)
	require.Len(t, second.Certificates, 1)

	require.Equal(t, first.Certificates[0].Certificate, second.Certificates[0].Certificate,
		"certificate must be identical across restarts, or TOFU pinning on the client side would break every time the server restarts")
}

func TestLoadOrCreateServerTLSConfig_DifferentPathsDifferentCerts(t *testing.T) {
	dir := t.TempDir()

	a, err := infra.LoadOrCreateServerTLSConfig(filepath.Join(dir, "a.pem"), "test-alpn")
	require.NoError(t, err)

	b, err := infra.LoadOrCreateServerTLSConfig(filepath.Join(dir, "b.pem"), "test-alpn")
	require.NoError(t, err)

	require.NotEqual(t, a.Certificates[0].Certificate, b.Certificates[0].Certificate)
}
