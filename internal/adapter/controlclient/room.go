package controlclient

import (
	"context"
	"errors"
	"fmt"

	"github.com/fu1se/spur/internal/adapter/controlproto"
	"github.com/fu1se/spur/internal/domain"
)

// CreateRoom asks the server to create a brand-new, persistent room named
// roomName with the caller (derived server-side from pub) as its first
// member, returning an invite token for the second participant.
func (c *Client) CreateRoom(ctx context.Context, roomName string, pub domain.PublicKey) (string, error) {
	stream, err := c.conn.OpenStreamSync(ctx)
	if err != nil {
		return "", fmt.Errorf("controlclient: open stream: %w", err)
	}
	defer stream.Close()

	if err := controlproto.WriteMethod(stream, controlproto.MethodCreateRoom); err != nil {
		return "", err
	}
	if err := controlproto.WriteFrame(stream, &controlproto.CreateRoomRequest{
		RoomName:  roomName,
		PublicKey: pub[:],
	}); err != nil {
		return "", err
	}

	var resp controlproto.CreateRoomResponse
	if err := controlproto.ReadFrame(stream, &resp); err != nil {
		return "", err
	}
	if resp.GetError() != "" {
		return "", errors.New(resp.GetError())
	}
	return resp.GetInviteToken(), nil
}

// JoinRoom asks the server to add the caller as roomName's second member.
// inviteToken must match what CreateRoom returned, unless the caller is
// already a member (idempotent rejoin).
func (c *Client) JoinRoom(ctx context.Context, roomName, inviteToken string, pub domain.PublicKey) error {
	stream, err := c.conn.OpenStreamSync(ctx)
	if err != nil {
		return fmt.Errorf("controlclient: open stream: %w", err)
	}
	defer stream.Close()

	if err := controlproto.WriteMethod(stream, controlproto.MethodJoinRoom); err != nil {
		return err
	}
	if err := controlproto.WriteFrame(stream, &controlproto.JoinRoomRequest{
		RoomName:    roomName,
		PublicKey:   pub[:],
		InviteToken: inviteToken,
	}); err != nil {
		return err
	}

	var resp controlproto.JoinRoomResponse
	if err := controlproto.ReadFrame(stream, &resp); err != nil {
		return err
	}
	if resp.GetError() != "" {
		return errors.New(resp.GetError())
	}
	return nil
}

// ResolveRoom asks the server who the caller's counterpart is in
// roomName, once both members have joined.
func (c *Client) ResolveRoom(ctx context.Context, roomName string, pub domain.PublicKey) (domain.PeerID, error) {
	stream, err := c.conn.OpenStreamSync(ctx)
	if err != nil {
		return "", fmt.Errorf("controlclient: open stream: %w", err)
	}
	defer stream.Close()

	if err := controlproto.WriteMethod(stream, controlproto.MethodResolveRoom); err != nil {
		return "", err
	}
	if err := controlproto.WriteFrame(stream, &controlproto.ResolveRoomRequest{
		RoomName:  roomName,
		PublicKey: pub[:],
	}); err != nil {
		return "", err
	}

	var resp controlproto.ResolveRoomResponse
	if err := controlproto.ReadFrame(stream, &resp); err != nil {
		return "", err
	}
	if resp.GetError() != "" {
		return "", errors.New(resp.GetError())
	}
	return domain.PeerID(resp.GetPeerId()), nil
}
