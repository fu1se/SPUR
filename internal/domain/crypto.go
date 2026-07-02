// Package domain holds the Entities layer: types with no dependency on any
// other layer or third-party library beyond the standard library.
package domain

// PublicKey is a peer's identity key (Curve25519 / WireGuard-compatible,
// 32 bytes).
type PublicKey [32]byte

func (k PublicKey) IsZero() bool {
	return k == PublicKey{}
}
