package controlproto

import (
	"fmt"
	"net/netip"

	"github.com/fu1se/spur/internal/domain"
)

// MeshMemberToProto translates a domain.MeshMember to its wire form.
func MeshMemberToProto(m domain.MeshMember) *MeshMember {
	return &MeshMember{
		PeerId:    string(m.PeerID),
		PublicKey: m.PublicKey[:],
		MeshIp:    m.MeshIP.String(),
	}
}

// MeshMemberFromProto translates a wire MeshMember back to domain form.
func MeshMemberFromProto(m *MeshMember) (domain.MeshMember, error) {
	if len(m.GetPublicKey()) != len(domain.PublicKey{}) {
		return domain.MeshMember{}, fmt.Errorf("controlproto: mesh member has bad public key length %d", len(m.GetPublicKey()))
	}
	var pub domain.PublicKey
	copy(pub[:], m.GetPublicKey())

	ip, err := netip.ParseAddr(m.GetMeshIp())
	if err != nil {
		return domain.MeshMember{}, fmt.Errorf("controlproto: parse mesh ip %q: %w", m.GetMeshIp(), err)
	}

	return domain.MeshMember{
		PeerID:    domain.PeerID(m.GetPeerId()),
		PublicKey: pub,
		MeshIP:    ip,
	}, nil
}

// MeshMembersToProto translates a slice of domain.MeshMember to wire form.
func MeshMembersToProto(ms []domain.MeshMember) []*MeshMember {
	out := make([]*MeshMember, 0, len(ms))
	for _, m := range ms {
		out = append(out, MeshMemberToProto(m))
	}
	return out
}

// MeshMembersFromProto translates a slice of wire MeshMembers to domain form.
func MeshMembersFromProto(ms []*MeshMember) ([]domain.MeshMember, error) {
	out := make([]domain.MeshMember, 0, len(ms))
	for _, m := range ms {
		dm, err := MeshMemberFromProto(m)
		if err != nil {
			return nil, err
		}
		out = append(out, dm)
	}
	return out, nil
}
