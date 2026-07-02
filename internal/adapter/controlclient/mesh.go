package controlclient

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/fu1se/localizator/internal/adapter/controlproto"
	"github.com/fu1se/localizator/internal/domain"
)

// JoinNetwork implements port.NetworkJoiner.
func (c *Client) JoinNetwork(ctx context.Context, networkName string, pub domain.PublicKey) (domain.Network, error) {
	stream, err := c.conn.OpenStreamSync(ctx)
	if err != nil {
		return domain.Network{}, fmt.Errorf("controlclient: open stream: %w", err)
	}
	defer stream.Close()

	if err := controlproto.WriteMethod(stream, controlproto.MethodJoinNetwork); err != nil {
		return domain.Network{}, err
	}
	if err := controlproto.WriteFrame(stream, &controlproto.JoinNetworkRequest{
		NetworkName: networkName,
		PublicKey:   pub[:],
	}); err != nil {
		return domain.Network{}, err
	}

	var resp controlproto.JoinNetworkResponse
	if err := controlproto.ReadFrame(stream, &resp); err != nil {
		return domain.Network{}, err
	}

	cidr, err := netip.ParsePrefix(resp.GetCidr())
	if err != nil {
		return domain.Network{}, fmt.Errorf("controlclient: parse network cidr %q: %w", resp.GetCidr(), err)
	}
	members, err := controlproto.MeshMembersFromProto(resp.GetMembers())
	if err != nil {
		return domain.Network{}, err
	}

	return domain.Network{
		Name:    networkName,
		CIDR:    cidr,
		Members: members,
	}, nil
}
