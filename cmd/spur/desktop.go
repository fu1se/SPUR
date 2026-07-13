package main

import (
	"context"
	"fmt"

	"github.com/fu1se/spur/internal/adapter/cli"
	"github.com/fu1se/spur/internal/adapter/desktopshare"
	"github.com/fu1se/spur/internal/adapter/localnet"
	"github.com/fu1se/spur/internal/adapter/rendezvous"
	"github.com/fu1se/spur/internal/usecase"
)

// desktopShare is "spur desktop share": start a local desktop (VNC)
// server for this session (see adapter/desktopshare for how the backend
// is picked), then serve it to the counterpart through the same
// persistent tunnel machinery as "spur expose" — plus one extra step per
// established tunnel: a DesktopOffer stream telling the viewer which
// protocol is on the other end and the session password, so the user
// never exchanges anything beyond the pairing code/room. The desktop
// server's lifetime is the command's, not the tunnel's — reconnects
// don't restart it.
func desktopShare(ctx context.Context, serverAddr, stunAddr, counterpartID, roomName, identityPath string, viewOnly bool, onSelfID func(string), onCode cli.OnCodeFunc, onReady cli.DesktopShareReadyFunc, onVersionMismatch cli.VersionMismatchFunc, onReconnect cli.OnReconnectFunc) error {
	srv, err := desktopshare.StartServer(ctx, viewOnly)
	if err != nil {
		return fmt.Errorf("app: start desktop server: %w", err)
	}
	defer srv.Stop()
	onReady(srv.Backend, srv.Port)

	offer := usecase.DesktopOffer{Protocol: "vnc", Password: srv.Password, ViewOnly: viewOnly}
	dialer := localnet.TCPDialer{Addr: fmt.Sprintf("127.0.0.1:%d", srv.Port)}

	resolve := rendezvous.CounterpartResolverFor(counterpartID, roomName, rendezvous.OnCodeFunc(onCode))
	return rendezvous.RunPersistent(ctx, serverAddr, stunAddr, identityPath, "", cli.Version(), resolve, onSelfID, rendezvous.VersionMismatchFunc(onVersionMismatch), rendezvous.OnReconnectFunc(onReconnect),
		func(ctx context.Context, tun *rendezvous.Tunnel) error {
			if err := usecase.SendDesktopOffer(ctx, tun.Conn, offer); err != nil {
				return err
			}
			return usecase.ServeExposedPort{Dialer: dialer, Tunnel: tun.Conn}.Run(ctx)
		})
}

// desktopView is "spur desktop view": the mirror side — listen on a
// loopback port (in VNC's traditional 5900+ range, see
// listenDesktopViewerPort), forward it through the tunnel to the sharing
// side's desktop server, and launch whatever VNC viewer is installed
// once the first tunnel is up. Loopback-only on purpose, unlike "spur
// connect"'s all-interfaces listener: a desktop viewed here shouldn't
// implicitly become reachable by everyone on this machine's LAN.
func desktopView(ctx context.Context, serverAddr, stunAddr, counterpartID, roomName, identityPath string, localPort int, onSelfID func(string), onCode cli.OnCodeFunc, onReady cli.DesktopViewReadyFunc, onVersionMismatch cli.VersionMismatchFunc, onReconnect cli.OnReconnectFunc) error {
	listener, port, err := listenDesktopViewerPort(localPort)
	if err != nil {
		return fmt.Errorf("app: listen locally: %w", err)
	}
	defer listener.Close()

	viewerLaunched := false
	resolve := rendezvous.CounterpartResolverFor(counterpartID, roomName, rendezvous.OnCodeFunc(onCode))
	return rendezvous.RunPersistent(ctx, serverAddr, stunAddr, identityPath, "", cli.Version(), resolve, onSelfID, rendezvous.VersionMismatchFunc(onVersionMismatch), rendezvous.OnReconnectFunc(onReconnect),
		func(ctx context.Context, tun *rendezvous.Tunnel) error {
			offer, err := usecase.ReceiveDesktopOffer(ctx, tun.Conn)
			if err != nil {
				return err
			}
			// First successful tunnel only: on reconnects the viewer the
			// user already has open just retries against the same local
			// port — spawning a second window would be worse than
			// useless.
			if !viewerLaunched {
				viewerLaunched = true
				viewer, _ := desktopshare.LaunchViewer(ctx, "127.0.0.1", port)
				onReady(desktopshare.ViewerAddr("127.0.0.1", port), offer.Password, viewer)
			}
			return usecase.ForwardPort{Listener: listener, Tunnel: tun.Conn}.Run(ctx)
		})
}

// listenDesktopViewerPort binds the local loopback port the VNC viewer
// connects to. With no explicit port it scans VNC's traditional display
// range (5900..5999) instead of taking an ephemeral one: gvncviewer can
// only address ports as "display numbers" (port-5900), and any human
// typing an address into a manual client expects :590x anyway.
func listenDesktopViewerPort(explicit int) (*localnet.TCPListener, int, error) {
	if explicit != 0 {
		l, err := localnet.ListenTCP(fmt.Sprintf("127.0.0.1:%d", explicit))
		return l, explicit, err
	}
	var lastErr error
	for port := 5900; port < 6000; port++ {
		l, err := localnet.ListenTCP(fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			return l, port, nil
		}
		lastErr = err
	}
	return nil, 0, fmt.Errorf("no free port in 5900-5999: %w", lastErr)
}
