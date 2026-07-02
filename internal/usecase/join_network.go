package usecase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/netip"

	"github.com/fu1se/localizator/internal/domain"
	"github.com/fu1se/localizator/internal/usecase/port"
)

// defaultMeshCIDR is used to auto-create a network the first time anyone
// joins it by name. There is still no separate "create network"/admin
// step — whoever joins a not-yet-existing name creates it — but joining
// an *existing* network now requires its invite token (see Execute),
// closing the "anyone who knows the name can join" gap from Phase 6.
const defaultMeshCIDR = "100.64.0.0/10"

// JoinNetwork is the server-side use case backing mesh network membership:
// auto-create the network (and its invite token) on first join, require
// the correct token from every subsequent new member, assign the caller a
// mesh IP, and return the full membership list so the caller knows every
// peer it needs to set up a tunnel to. Already-known members can rejoin
// (idempotent) without presenting the token again.
type JoinNetwork struct {
	Networks port.NetworkRepository
}

func (uc JoinNetwork) Execute(ctx context.Context, networkName string, peer domain.PeerID, pub domain.PublicKey, inviteToken string) (domain.Network, error) {
	return uc.Networks.Update(ctx, networkName, func(network domain.Network) (domain.Network, error) {
		switch {
		case !network.CIDR.IsValid():
			token, err := generateInviteToken()
			if err != nil {
				return domain.Network{}, err
			}
			network = domain.Network{Name: networkName, CIDR: netip.MustParsePrefix(defaultMeshCIDR), InviteToken: token}
		case !network.HasMember(peer):
			if inviteToken == "" || inviteToken != network.InviteToken {
				return domain.Network{}, domain.ErrInvalidInviteToken
			}
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

func generateInviteToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("usecase: generate invite token: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// JoinMeshNetwork is the client-side use case: ask the server to join a
// network. Backed by port.NetworkJoiner, which speaks to JoinNetwork above
// over the control channel.
type JoinMeshNetwork struct {
	Joiner port.NetworkJoiner
}

func (uc JoinMeshNetwork) Execute(ctx context.Context, networkName, inviteToken string, pub domain.PublicKey) (domain.Network, error) {
	return uc.Joiner.JoinNetwork(ctx, networkName, inviteToken, pub)
}
