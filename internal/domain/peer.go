package domain

import "time"

// PeerID uniquely identifies a peer. It is derived from the peer's
// PublicKey (e.g. a base32/hex fingerprint), not chosen by the user.
type PeerID string

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
