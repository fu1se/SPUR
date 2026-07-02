package controlclient

import (
	"context"
	"errors"
	"fmt"
	"net/netip"

	"github.com/fu1se/spur/internal/adapter/controlproto"
	"github.com/fu1se/spur/internal/domain"
)

// JoinNetwork implements port.NetworkJoiner.
func (c *Client) JoinNetwork(ctx context.Context, networkName, inviteToken string, pub domain.PublicKey) (domain.Network, error) {
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
		InviteToken: inviteToken,
	}); err != nil {
		return domain.Network{}, err
	}

	var resp controlproto.JoinNetworkResponse
	if err := controlproto.ReadFrame(stream, &resp); err != nil {
		return domain.Network{}, err
	}
	if resp.GetError() != "" {
		return domain.Network{}, errors.New(resp.GetError())
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
		Name:        networkName,
		CIDR:        cidr,
		InviteToken: resp.GetInviteToken(),
		Members:     members,
	}, nil
}
