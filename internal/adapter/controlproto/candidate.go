package controlproto

import (
	"fmt"
	"net/netip"

	"github.com/fu1se/localizator/internal/domain"
)

// CandidateToProto translates a domain.Candidate to its wire form.
func CandidateToProto(c domain.Candidate) *Candidate {
	return &Candidate{
		Kind:    string(c.Kind),
		Address: c.Addr.String(),
	}
}

// CandidateFromProto translates a wire Candidate back to domain.Candidate.
func CandidateFromProto(c *Candidate) (domain.Candidate, error) {
	addr, err := netip.ParseAddrPort(c.GetAddress())
	if err != nil {
		return domain.Candidate{}, fmt.Errorf("controlproto: parse candidate address %q: %w", c.GetAddress(), err)
	}
	return domain.Candidate{
		Kind: domain.CandidateKind(c.GetKind()),
		Addr: addr,
	}, nil
}

// CandidatesToProto translates a slice of domain.Candidate to wire form.
func CandidatesToProto(cs []domain.Candidate) []*Candidate {
	out := make([]*Candidate, 0, len(cs))
	for _, c := range cs {
		out = append(out, CandidateToProto(c))
	}
	return out
}

// CandidatesFromProto translates a slice of wire Candidates to domain form.
func CandidatesFromProto(cs []*Candidate) ([]domain.Candidate, error) {
	out := make([]domain.Candidate, 0, len(cs))
	for _, c := range cs {
		dc, err := CandidateFromProto(c)
		if err != nil {
			return nil, err
		}
		out = append(out, dc)
	}
	return out, nil
}
