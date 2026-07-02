package domain_test

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/localizator/internal/domain"
)

func TestNetwork_NextAvailableIP(t *testing.T) {
	network := domain.Network{
		Name: "home",
		CIDR: netip.MustParsePrefix("100.64.0.0/29"), // 100.64.0.0 - 100.64.0.7
	}

	first, err := network.NextAvailableIP()
	require.NoError(t, err)
	require.Equal(t, netip.MustParseAddr("100.64.0.1"), first)

	network.Members = append(network.Members, domain.MeshMember{
		PeerID: "peerA",
		MeshIP: first,
	})

	second, err := network.NextAvailableIP()
	require.NoError(t, err)
	require.Equal(t, netip.MustParseAddr("100.64.0.2"), second)
}

func TestNetwork_NextAvailableIP_Exhausted(t *testing.T) {
	network := domain.Network{
		CIDR: netip.MustParsePrefix("100.64.0.0/30"), // usable: .1, .2 (skips .0 and .3 stays unused by our scheme too)
	}
	network.Members = append(network.Members,
		domain.MeshMember{PeerID: "a", MeshIP: netip.MustParseAddr("100.64.0.1")},
		domain.MeshMember{PeerID: "b", MeshIP: netip.MustParseAddr("100.64.0.2")},
		domain.MeshMember{PeerID: "c", MeshIP: netip.MustParseAddr("100.64.0.3")},
	)

	_, err := network.NextAvailableIP()
	require.ErrorIs(t, err, domain.ErrNetworkAddressSpaceExhausted)
}

func TestNetwork_HasMember(t *testing.T) {
	network := domain.Network{
		Members: []domain.MeshMember{{PeerID: "peerA"}},
	}

	require.True(t, network.HasMember("peerA"))
	require.False(t, network.HasMember("peerB"))
}
