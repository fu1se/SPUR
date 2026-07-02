package domain

import (
	"net/netip"
	"time"
)

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
