package infra

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fu1se/localizator/internal/domain"
)

// DefaultIdentityPath is where LoadOrCreateIdentity looks by default.
func DefaultIdentityPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("infra: user config dir: %w", err)
	}
	return filepath.Join(dir, "localizator", "identity.key"), nil
}

// LoadOrCreateIdentity persists a peer's identity across process restarts.
// Without this, "app connect --to X" is unusable: expose/connect used to
// generate a brand new random key on every invocation, so there was no
// point in time where a user could learn a peer's ID and hand it to the
// other side before that ID changed again. A real keypair with signing
// (Phase 7) will replace the raw random bytes stored here, but the
// on-disk persistence mechanism stays the same.
func LoadOrCreateIdentity(path string) (domain.PublicKey, error) {
	var pub domain.PublicKey

	data, err := os.ReadFile(path)
	if err == nil {
		if len(data) != len(pub) {
			return domain.PublicKey{}, fmt.Errorf("infra: identity file %s has wrong length %d", path, len(data))
		}
		copy(pub[:], data)
		return pub, nil
	}
	if !os.IsNotExist(err) {
		return domain.PublicKey{}, fmt.Errorf("infra: read identity: %w", err)
	}

	if _, err := rand.Read(pub[:]); err != nil {
		return domain.PublicKey{}, fmt.Errorf("infra: generate identity: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return domain.PublicKey{}, fmt.Errorf("infra: create identity dir: %w", err)
	}
	if err := os.WriteFile(path, pub[:], 0o600); err != nil {
		return domain.PublicKey{}, fmt.Errorf("infra: write identity: %w", err)
	}
	return pub, nil
}
