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
// aren't both set.
func connect(ctx context.Context, serverAddr, stunAddr, counterpartID, roomName, identityPath string, localPort int, onSelfID func(string), onCode cli.OnCodeFunc, onVersionMismatch cli.VersionMismatchFunc) error {
	resolve := rendezvous.CounterpartResolverFor(counterpartID, roomName, rendezvous.OnCodeFunc(onCode))
	tun, _, _, err := rendezvous.Establish(ctx, serverAddr, stunAddr, identityPath, "", cli.Version(), resolve, onSelfID, rendezvous.VersionMismatchFunc(onVersionMismatch))
	if err != nil {
		return err
	}
	defer tun.Close()

	listener, err := localnet.ListenTCP(fmt.Sprintf(":%d", localPort))
	if err != nil {
		return fmt.Errorf("app: listen locally: %w", err)
	}
	defer listener.Close()

	return usecase.ForwardPort{Listener: listener, Tunnel: tun.Conn}.Run(ctx)
}

// expose is "spur expose": accept tunnel streams from counterpart and
// forward each to targetPort on the local machine. counterpartID,
// roomName: see connect.
func expose(ctx context.Context, serverAddr, stunAddr, counterpartID, roomName, identityPath string, targetPort int, onSelfID func(string), onCode cli.OnCodeFunc, onVersionMismatch cli.VersionMismatchFunc) error {
	resolve := rendezvous.CounterpartResolverFor(counterpartID, roomName, rendezvous.OnCodeFunc(onCode))
	tun, _, _, err := rendezvous.Establish(ctx, serverAddr, stunAddr, identityPath, "", cli.Version(), resolve, onSelfID, rendezvous.VersionMismatchFunc(onVersionMismatch))
	if err != nil {
		return err
	}
	defer tun.Close()

	dialer := localnet.TCPDialer{Addr: fmt.Sprintf("127.0.0.1:%d", targetPort)}

	return usecase.ServeExposedPort{Dialer: dialer, Tunnel: tun.Conn}.Run(ctx)
}
