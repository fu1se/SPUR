package guiapp

import (
	"context"

	"github.com/fu1se/spur/internal/adapter/cli"
	"github.com/fu1se/spur/internal/adapter/rendezvous"
)

// CreateRoom is "spur room create": creates a new, persistent two-member
// room and returns the invite token to hand to the second participant.
func (c *Client) CreateRoom(ctx context.Context, serverAddr, roomName string, onVersionMismatch cli.VersionMismatchFunc) (cli.RoomResult, error) {
	client, id, err := rendezvous.DialAndRegister(ctx, serverAddr, c.identityPath, "", cli.Version(), rendezvous.VersionMismatchFunc(onVersionMismatch))
	if err != nil {
		return cli.RoomResult{}, err
	}
	defer client.Close()

	token, err := client.CreateRoom(ctx, roomName, id.PublicKey)
	if err != nil {
		return cli.RoomResult{}, err
	}
	return cli.RoomResult{InviteToken: token}, nil
}

// JoinRoom is "spur room join": joins a room created by someone else,
// using the invite token they shared. Already-known members can rejoin
// without a token.
func (c *Client) JoinRoom(ctx context.Context, serverAddr, roomName, inviteToken string, onVersionMismatch cli.VersionMismatchFunc) error {
	client, id, err := rendezvous.DialAndRegister(ctx, serverAddr, c.identityPath, "", cli.Version(), rendezvous.VersionMismatchFunc(onVersionMismatch))
	if err != nil {
		return err
	}
	defer client.Close()

	return client.JoinRoom(ctx, roomName, inviteToken, id.PublicKey)
}
