package usecase

import (
	"context"
	"net/netip"

	"github.com/fu1se/localizator/internal/domain"
	"github.com/fu1se/localizator/internal/usecase/port"
)

// defaultMeshCIDR is used to auto-create a network the first time anyone
// joins it by name. There is no separate "create network"/admin step or
// invite-token gating yet — anyone who knows (or guesses) a network name
// can join it. That's an intentional MVP simplification for Phase 6;
// invite tokens (domain.Network.InviteToken, already modeled, just unused)
// are Phase 7's job.
const defaultMeshCIDR = "100.64.0.0/10"

// JoinNetwork is the server-side use case backing mesh network membership:
// auto-create the network on first join, assign the caller a mesh IP if
// it's not already a member, and return the full membership list so the
// caller knows every peer it needs to set up a tunnel to.
type JoinNetwork struct {
	Networks port.NetworkRepository
}

func (uc JoinNetwork) Execute(ctx context.Context, networkName string, peer domain.PeerID, pub domain.PublicKey) (domain.Network, error) {
	return uc.Networks.Update(ctx, networkName, func(network domain.Network) (domain.Network, error) {
		if !network.CIDR.IsValid() {
			network = domain.Network{Name: networkName, CIDR: netip.MustParsePrefix(defaultMeshCIDR)}
		}

		if network.HasMember(peer) {
			return network, nil
		}

		ip, err := network.NextAvailableIP()
		if err != nil {
			return domain.Network{}, err
		}
		network.Members = append(network.Members, domain.MeshMember{PeerID: peer, PublicKey: pub, MeshIP: ip})
		return network, nil
	})
}

// JoinMeshNetwork is the client-side use case: ask the server to join a
// network. Backed by port.NetworkJoiner, which speaks to JoinNetwork above
// over the control channel.
type JoinMeshNetwork struct {
	Joiner port.NetworkJoiner
}

func (uc JoinMeshNetwork) Execute(ctx context.Context, networkName string, pub domain.PublicKey) (domain.Network, error) {
	return uc.Joiner.JoinNetwork(ctx, networkName, pub)
}
