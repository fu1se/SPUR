package infra_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/localizator/internal/infra"
)

func TestTOFUClientTLSConfig_PinsOnFirstConnect(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_servers.json")
	cfg := infra.TOFUClientTLSConfig(path, "server:1234", "test-alpn")

	certA := []byte("fake-cert-bytes-a")
	require.NoError(t, cfg.VerifyPeerCertificate([][]byte{certA}, nil))

	// Same cert on a later connection: still trusted.
	require.NoError(t, cfg.VerifyPeerCertificate([][]byte{certA}, nil))

	// A different cert presented for the same server: rejected — this is
	// the whole point, it's what would catch a MITM (or a legitimately
	// regenerated server identity, which is why the error message says
	// how to re-trust).
	certB := []byte("fake-cert-bytes-b")
	err := cfg.VerifyPeerCertificate([][]byte{certB}, nil)
	require.Error(t, err)
}

func TestTOFUClientTLSConfig_NoCertificatePresented(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_servers.json")
	cfg := infra.TOFUClientTLSConfig(path, "server:1234", "test-alpn")

	err := cfg.VerifyPeerCertificate(nil, nil)
	require.Error(t, err)
}

func TestTOFUClientTLSConfig_IndependentPerServer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_servers.json")

	cfgA := infra.TOFUClientTLSConfig(path, "server-a:1234", "test-alpn")
	cfgB := infra.TOFUClientTLSConfig(path, "server-b:1234", "test-alpn")

	certA := []byte("cert-a")
	certB := []byte("cert-b")

	require.NoError(t, cfgA.VerifyPeerCertificate([][]byte{certA}, nil))
	require.NoError(t, cfgB.VerifyPeerCertificate([][]byte{certB}, nil))

	// Each server's pin is independent: B's cert would never be confused
	// with A's, and vice versa.
	require.NoError(t, cfgA.VerifyPeerCertificate([][]byte{certA}, nil))
	require.NoError(t, cfgB.VerifyPeerCertificate([][]byte{certB}, nil))
}
