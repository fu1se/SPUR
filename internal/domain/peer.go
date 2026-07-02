package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// PeerID uniquely identifies a peer. It is derived from the peer's
// PublicKey, not chosen by the user.
type PeerID string

// DerivePeerID computes the canonical PeerID for a PublicKey: the first 16
// bytes of its SHA-256 digest, hex-encoded. Identity is always derived this
// way so two peers can never disagree on a third peer's ID.
func DerivePeerID(pub PublicKey) PeerID {
	sum := sha256.Sum256(pub[:])
	return PeerID(hex.EncodeToString(sum[:16]))
}

// Peer is a node participating in the network — either as the initiator or
// the target of a tunnel.
type Peer struct {
	ID         PeerID
	PublicKey  PublicKey
	Candidates []Candidate
	LastSeenAt time.Time
}

func NewPeer(id PeerID, pub PublicKey) Peer {
	return Peer{ID: id, PublicKey: pub}
}
