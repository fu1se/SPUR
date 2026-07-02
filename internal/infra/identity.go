package infra

import (
	"crypto/ecdh"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fu1se/localizator/internal/domain"
)

// Identity is a peer's real X25519 keypair: PrivateKey is the credential
// (never leaves this process except to be written to the identity file),
// PublicKey is what gets registered with the server and handed to
// counterparts. Both control-channel authentication (Phase 7) and the
// mesh data plane (Phase 6 — WireGuard's own Noise handshake needs a
// genuine, matching keypair, not just an arbitrary 32-byte label) depend
// on PublicKey actually being PrivateKey's derived public point.
type Identity struct {
	PrivateKey *ecdh.PrivateKey
	PublicKey  domain.PublicKey
}

// DefaultIdentityPath is where LoadOrCreateIdentity looks by default.
func DefaultIdentityPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("infra: user config dir: %w", err)
	}
	return filepath.Join(dir, "localizator", "identity.key"), nil
}

// LoadOrCreateIdentity persists a peer's X25519 private key across process
// restarts, deriving the public key from it every time rather than storing
// it separately. Without persistence, "app connect --to X" is unusable:
// there would be no point in time where a user could learn a peer's ID and
// hand it to the other side before that ID changed again on the next run.
func LoadOrCreateIdentity(path string) (Identity, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		priv, err := ecdh.X25519().NewPrivateKey(data)
		if err != nil {
			return Identity{}, fmt.Errorf("infra: identity file %s is not a valid X25519 key: %w", path, err)
		}
		return identityFrom(priv), nil
	}
	if !os.IsNotExist(err) {
		return Identity{}, fmt.Errorf("infra: read identity: %w", err)
	}

	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return Identity{}, fmt.Errorf("infra: generate identity: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return Identity{}, fmt.Errorf("infra: create identity dir: %w", err)
	}
	if err := os.WriteFile(path, priv.Bytes(), 0o600); err != nil {
		return Identity{}, fmt.Errorf("infra: write identity: %w", err)
	}
	return identityFrom(priv), nil
}

func identityFrom(priv *ecdh.PrivateKey) Identity {
	var pub domain.PublicKey
	copy(pub[:], priv.PublicKey().Bytes())
	return Identity{PrivateKey: priv, PublicKey: pub}
}
