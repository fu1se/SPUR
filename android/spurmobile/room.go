package spurmobile

import (
	"context"
)

// CreateRoom is "spur room create": creates a new, persistent two-member
// room and returns the invite token to hand to the second participant
// (see rendezvous.RoomCounterpart's doc comment for what a room is for
// — a standing counterpart binding, unlike a one-shot pairing code).
func (c *Client) CreateRoom(serverAddr, roomName string) (string, error) {
	client, id, err := dialAndRegister(serverAddr, c)
	if err != nil {
		return "", err
	}
	defer client.Close()

	return client.CreateRoom(context.Background(), roomName, id.PublicKey)
}

// JoinRoom is "spur room join": joins a room created by someone else,
// using the invite token they shared. Already-known members can rejoin
// without a token (idempotent) — see usecase.JoinRoom's doc comment.
func (c *Client) JoinRoom(serverAddr, roomName, inviteToken string) error {
	client, id, err := dialAndRegister(serverAddr, c)
	if err != nil {
		return err
	}
	defer client.Close()

	return client.JoinRoom(context.Background(), roomName, inviteToken, id.PublicKey)
}
