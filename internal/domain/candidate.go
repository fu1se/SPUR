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

// CandidateSet bundles one peer's candidates with its identity key —
// published together during candidate exchange so that, by the time a
// session is established, both sides already have what they need to
// derive an end-to-end shared secret (Phase 7), independent of whether
// the transport that ends up carrying data is P2P or relay.
type CandidateSet struct {
	Candidates []Candidate
	PublicKey  PublicKey
}
