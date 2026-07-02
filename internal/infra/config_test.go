package infra_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/localizator/internal/infra"
)

func TestLoadConfig_MissingFileReturnsZeroValue(t *testing.T) {
	cfg, err := infra.LoadConfig(filepath.Join(t.TempDir(), "does-not-exist.json"))
	require.NoError(t, err)
	require.Equal(t, infra.Config{}, cfg)
}

func TestLoadConfig_ReadsFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{
		"server": "rendezvous.example.com:4443",
		"stun_server": "rendezvous.example.com:4444",
		"identity": "/home/user/.config/localizator/identity.key"
	}`), 0o600))

	cfg, err := infra.LoadConfig(path)
	require.NoError(t, err)
	require.Equal(t, infra.Config{
		Server:     "rendezvous.example.com:4443",
		StunServer: "rendezvous.example.com:4444",
		Identity:   "/home/user/.config/localizator/identity.key",
	}, cfg)
}

func TestLoadConfig_InvalidJSONFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte("not json"), 0o600))

	_, err := infra.LoadConfig(path)
	require.Error(t, err)
}
