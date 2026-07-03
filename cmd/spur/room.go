package main

import (
	"context"

	"github.com/fu1se/spur/internal/adapter/cli"
	"github.com/fu1se/spur/internal/adapter/rendezvous"
)

// createRoom is "spur room create": mirrors joinNetwork's shape — dial,
// register (purely to learn the server's version and pin the caller's
// own identity), then the room-specific RPC.
func createRoom(ctx context.Context, serverAddr, roomName, identityPath string, onVersionMismatch cli.VersionMismatchFunc) (cli.RoomResult, error) {
	client, id, err := rendezvous.DialAndRegister(ctx, serverAddr, identityPath, "", cli.Version(), rendezvous.VersionMismatchFunc(onVersionMismatch))
	if err != nil {
		return cli.RoomResult{}, err
	}
	defer client.Close()

	inviteToken, err := client.CreateRoom(ctx, roomName, id.PublicKey)
	if err != nil {
		return cli.RoomResult{}, err
	}
	return cli.RoomResult{InviteToken: inviteToken}, nil
}

// joinRoom is "spur room join": see createRoom.
func joinRoom(ctx context.Context, serverAddr, roomName, inviteToken, identityPath string, onVersionMismatch cli.VersionMismatchFunc) error {
	client, id, err := rendezvous.DialAndRegister(ctx, serverAddr, identityPath, "", cli.Version(), rendezvous.VersionMismatchFunc(onVersionMismatch))
	if err != nil {
		return err
	}
	defer client.Close()

	return client.JoinRoom(ctx, roomName, inviteToken, id.PublicKey)
}
