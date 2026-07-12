package main

import (
	"context"
	"fmt"

	"github.com/fu1se/spur/internal/adapter/cli"
	"github.com/fu1se/spur/internal/adapter/localnet"
	"github.com/fu1se/spur/internal/adapter/rendezvous"
	"github.com/fu1se/spur/internal/usecase"
)

// connect is "spur connect": forward every local connection on localPort
// through a tunnel to counterpart, who must be running "spur expose".
// counterpartID may be empty (host mode: register and print a pairing
// code, wait for "spur expose <code>" to use it), a full peer ID, or a
// pairing code the counterpart printed; roomName, if non-empty, resolves
// the counterpart via a persistent room instead (see
// rendezvous.RoomCounterpart) — the CLI layer already ensures the two
// aren't both set. The tunnel is persistent: a network drop re-establishes
// it (see rendezvous.RunPersistent) while the local listener stays open,
// so already-configured local clients just retry their connection instead
// of finding the port gone.
func connect(ctx context.Context, serverAddr, stunAddr, counterpartID, roomName, identityPath string, localPort int, onSelfID func(string), onCode cli.OnCodeFunc, onVersionMismatch cli.VersionMismatchFunc, onReconnect cli.OnReconnectFunc) error {
	// Listen before the first rendezvous: a busy local port is a
	// configuration error worth failing on immediately, not after a
	// potentially minute-long NAT punch.
	listener, err := localnet.ListenTCP(fmt.Sprintf(":%d", localPort))
	if err != nil {
		return fmt.Errorf("app: listen locally: %w", err)
	}
	defer listener.Close()

	resolve := rendezvous.CounterpartResolverFor(counterpartID, roomName, rendezvous.OnCodeFunc(onCode))
	return rendezvous.RunPersistent(ctx, serverAddr, stunAddr, identityPath, "", cli.Version(), resolve, onSelfID, rendezvous.VersionMismatchFunc(onVersionMismatch), rendezvous.OnReconnectFunc(onReconnect),
		func(ctx context.Context, tun *rendezvous.Tunnel) error {
			return usecase.ForwardPort{Listener: listener, Tunnel: tun.Conn}.Run(ctx)
		})
}

// expose is "spur expose": accept tunnel streams from counterpart and
// forward each to targetPort on the local machine. counterpartID,
// roomName: see connect. Persistent across network drops, like connect.
func expose(ctx context.Context, serverAddr, stunAddr, counterpartID, roomName, identityPath string, targetPort int, onSelfID func(string), onCode cli.OnCodeFunc, onVersionMismatch cli.VersionMismatchFunc, onReconnect cli.OnReconnectFunc) error {
	dialer := localnet.TCPDialer{Addr: fmt.Sprintf("127.0.0.1:%d", targetPort)}

	resolve := rendezvous.CounterpartResolverFor(counterpartID, roomName, rendezvous.OnCodeFunc(onCode))
	return rendezvous.RunPersistent(ctx, serverAddr, stunAddr, identityPath, "", cli.Version(), resolve, onSelfID, rendezvous.VersionMismatchFunc(onVersionMismatch), rendezvous.OnReconnectFunc(onReconnect),
		func(ctx context.Context, tun *rendezvous.Tunnel) error {
			return usecase.ServeExposedPort{Dialer: dialer, Tunnel: tun.Conn}.Run(ctx)
		})
}
