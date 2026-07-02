package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"net/netip"
	"time"
)

// SessionIDFor deterministically derives the shared session identifier two
// peers use to rendezvous, from their two peer IDs alone — order does not
// matter, and no third party needs to hand out the ID. This lets both
// sides independently start publishing/awaiting candidates for the same
// session without a prior "create session" round trip.
func SessionIDFor(a, b PeerID) string {
	x, y := string(a), string(b)
	if x > y {
		x, y = y, x
	}
	sum := sha256.Sum256([]byte(x + ":" + y))
	return hex.EncodeToString(sum[:16])
}

// IsDialer deterministically picks, between two peers, which one opens the
// data-plane connection (QUIC dial, or yamux.Client) and which one accepts
// it (QUIC listen, or yamux.Server) — without any extra coordination round
// trip. Comparing PeerIDs lexicographically guarantees both sides land on
// opposite answers when called with their own view of (self, counterpart).
func IsDialer(self, counterpart PeerID) bool {
	return self < counterpart
}

// SessionState is the lifecycle of one attempt to establish a data channel
// between two peers, following the ICE-like flow described in CLAUDE.md:
// register -> exchange candidates -> punch -> established (P2P or relay).
type SessionState string

const (
	SessionPending          SessionState = "pending"
	SessionPunching         SessionState = "punching"
	SessionEstablishedP2P   SessionState = "established_p2p"
	SessionEstablishedRelay SessionState = "established_relay"
	SessionFailed           SessionState = "failed"
)

// Session tracks one attempt to establish a data channel between two peers.
type Session struct {
	ID           string
	InitiatorID  PeerID
	ResponderID  PeerID
	State        SessionState
	ResolvedAddr netip.AddrPort // set once punching or relay establishes a path
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Established reports whether the session has a usable data path, via
// either direct P2P or relay fallback.
func (s Session) Established() bool {
	return s.State == SessionEstablishedP2P || s.State == SessionEstablishedRelay
}
