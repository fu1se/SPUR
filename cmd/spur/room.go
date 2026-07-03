package main

import (
	"context"
	"fmt"

	"github.com/fu1se/spur/internal/adapter/cli"
	"github.com/fu1se/spur/internal/adapter/controlclient"
	"github.com/fu1se/spur/internal/infra"
)

// createRoom is "spur room create": mirrors joinNetwork's shape — dial,
// register (purely to learn the server's version and pin the caller's
// own identity), then the room-specific RPC.
func createRoom(ctx context.Context, serverAddr, roomName, identityPath string, onVersionMismatch cli.VersionMismatchFunc) (cli.RoomResult, error) {
	client, id, err := dialAndRegisterForRoom(ctx, serverAddr, identityPath, onVersionMismatch)
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
	client, id, err := dialAndRegisterForRoom(ctx, serverAddr, identityPath, onVersionMismatch)
	if err != nil {
		return err
	}
	defer client.Close()

	return client.JoinRoom(ctx, roomName, inviteToken, id.PublicKey)
}

func dialAndRegisterForRoom(ctx context.Context, serverAddr, identityPath string, onVersionMismatch cli.VersionMismatchFunc) (*controlclient.Client, infra.Identity, error) {
	resolvedIdentityPath, err := resolveIdentityPath(identityPath)
	if err != nil {
		return nil, infra.Identity{}, err
	}
	id, err := infra.LoadOrCreateIdentity(resolvedIdentityPath)
	if err != nil {
		return nil, infra.Identity{}, fmt.Errorf("app: load identity: %w", err)
	}

	tlsConf, err := controlClientTLS(serverAddr)
	if err != nil {
		return nil, infra.Identity{}, err
	}
	client, err := controlclient.Dial(ctx, serverAddr, tlsConf, infra.DefaultQUICConfig())
	if err != nil {
		return nil, infra.Identity{}, err
	}

	regResult, err := client.Register(ctx, id.PublicKey, cli.Version())
	if err != nil {
		client.Close()
		return nil, infra.Identity{}, err
	}
	warnIfVersionMismatch(cli.Version(), regResult.ServerVersion, onVersionMismatch)

	return client, id, nil
}
