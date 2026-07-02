package domain

import "net/netip"

// CandidateKind classifies how an address was discovered, mirroring the
// simplified ICE model described in CLAUDE.md.
type CandidateKind string

const (
	CandidateHost            CandidateKind = "host"
	CandidateServerReflexive CandidateKind = "server_reflexive"
	CandidateRelay           CandidateKind = "relay"
)

// Candidate is one address a peer might be reachable at.
type Candidate struct {
	Kind CandidateKind
	Addr netip.AddrPort
}
