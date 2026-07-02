package infra

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// DefaultTrustStorePath is where TOFUClientTLSConfig looks by default.
func DefaultTrustStorePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("infra: user config dir: %w", err)
	}
	return filepath.Join(dir, "localizator", "known_servers.json"), nil
}

// tofuMu serializes trust-store reads/writes within this process, but two
// separate CLI processes (e.g. `connect` and `expose` both pinning the
// same server at nearly the same time — an entirely normal thing to
// happen) can still race on the file itself. saveTrustStore writes
// atomically (temp file + rename) specifically so a concurrent reader
// never sees a half-written file; a real bug here — os.WriteFile
// truncates before writing, and a reader landing in that window got
// "unexpected end of JSON input" — is why this isn't just a plain
// os.WriteFile call.
var tofuMu sync.Mutex

// TOFUClientTLSConfig builds a tls.Config for dialing serverAddr that
// trusts whatever self-signed certificate the server presents on the
// first connection and pins it afterwards (fingerprints kept in a small
// JSON file at trustStorePath, keyed by serverAddr). This replaces blind
// InsecureSkipVerify trust for the control-plane connection: without it,
// nothing stopped a network-level attacker from impersonating the
// rendezvous server and intercepting registration, candidate exchange,
// or relay traffic. It doesn't protect the very first connection to a
// given server (classic trust-on-first-use limitation — same tradeoff
// SSH host keys make), but it does mean a later MITM attempt against an
// already-known server gets rejected instead of silently trusted.
func TOFUClientTLSConfig(trustStorePath, serverAddr, alpn string) *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec // verified in VerifyPeerCertificate below, not via chain-of-trust
		NextProtos:         []string{alpn},
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return fmt.Errorf("infra: server presented no certificate")
			}
			sum := sha256.Sum256(rawCerts[0])
			return verifyOrPinServer(trustStorePath, serverAddr, hex.EncodeToString(sum[:]))
		},
	}
}

func verifyOrPinServer(trustStorePath, serverAddr, fingerprint string) error {
	tofuMu.Lock()
	defer tofuMu.Unlock()

	store, err := loadTrustStore(trustStorePath)
	if err != nil {
		return err
	}

	if known, ok := store[serverAddr]; ok {
		if known != fingerprint {
			return fmt.Errorf("infra: certificate for %s changed since it was first trusted "+
				"(expected fingerprint %s, got %s) — this could mean the server's identity was "+
				"legitimately regenerated, or someone is impersonating it; if you're sure it's "+
				"legitimate, remove its entry from %s and reconnect", serverAddr, known, fingerprint, trustStorePath)
		}
		return nil
	}

	store[serverAddr] = fingerprint
	return saveTrustStore(trustStorePath, store)
}

func loadTrustStore(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]string), nil
		}
		return nil, fmt.Errorf("infra: read trust store: %w", err)
	}

	store := make(map[string]string)
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("infra: parse trust store %s: %w", path, err)
	}
	return store, nil
}

func saveTrustStore(path string, store map[string]string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("infra: create trust store dir: %w", err)
	}

	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("infra: encode trust store: %w", err)
	}

	// Atomic write: a plain os.WriteFile truncates the file before
	// writing, so a concurrent reader (a different `app` process pinning
	// the same server at nearly the same time) can land in that window
	// and see a truncated/partial JSON file. Writing to a temp file in
	// the same directory and renaming over the target is atomic on
	// POSIX filesystems — any reader sees either the fully-old or
	// fully-new content, never a partial one.
	tmp, err := os.CreateTemp(dir, ".known_servers-*.tmp")
	if err != nil {
		return fmt.Errorf("infra: create trust store temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once the rename below succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("infra: write trust store temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("infra: close trust store temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return fmt.Errorf("infra: chmod trust store temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("infra: replace trust store: %w", err)
	}
	return nil
}
