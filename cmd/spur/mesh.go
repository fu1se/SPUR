package main

import (
	"context"
	"fmt"
	"net/netip"
	"time"

	"golang.zx2c4.com/wireguard/device"

	"github.com/fu1se/spur/internal/adapter/cli"
	"github.com/fu1se/spur/internal/adapter/controlclient"
	"github.com/fu1se/spur/internal/adapter/meshclient"
	"github.com/fu1se/spur/internal/adapter/rendezvous"
	"github.com/fu1se/spur/internal/adapter/wgmesh"
	"github.com/fu1se/spur/internal/domain"
	"github.com/fu1se/spur/internal/infra"
)

// meshRefreshInterval bounds how long a peer that joins later can stay
// invisible to peers that joined earlier. JoinNetwork returns a one-time
// snapshot of membership, not a live feed — see join's doc comment for why
// that makes periodic re-polling necessary rather than optional.
const meshRefreshInterval = 5 * time.Second

// join is "spur join": coordinates mesh network membership with the
// server, establishes a tunnel (P2P or relay, same EstablishSession/
// adapter/tunnel machinery as port-forward mode) to every other member,
// dedicates one stream per peer to carrying WireGuard traffic, and routes
// it through a real TUN interface.
//
// Membership is eventually consistent, not instantaneous: JoinNetwork
// gives back whoever had already joined by the time it's called, so a
// peer that joins first would never learn about one that joins seconds
// later without re-checking. This was a real bug found during live
// testing — two peers joining at nearly the same time, the first one
// simply never noticed the second existed and sat there with zero
// tunnels, while the second waited forever for candidates the first
// never published. join() re-polls JoinNetwork every meshRefreshInterval
// and connects to whatever's new; already-connected peers are left alone
// (dev.IpcSet without replace_peers only adds/updates, per WireGuard's
// UAPI).
//
// Requires elevated privileges (root/CAP_NET_ADMIN on Linux): creating the
// TUN device and assigning it an address changes real system network
// state. The per-peer tunnel orchestration itself (meshclient.Peers) is
// shared with the Android facade (android/spurmobile) — only TUN
// creation differs, see wgmesh.NewDevice vs NewDeviceFromFD.
func join(ctx context.Context, serverAddr, stunAddr, networkName, inviteToken, identityPath string, onSelfID func(string), onVersionMismatch cli.VersionMismatchFunc) error {
	resolvedIdentityPath, err := rendezvous.ResolveIdentityPath(identityPath)
	if err != nil {
		return err
	}
	id, err := infra.LoadOrCreateIdentity(resolvedIdentityPath)
	if err != nil {
		return fmt.Errorf("app: load identity: %w", err)
	}
	self := domain.DerivePeerID(id.PublicKey)
	onSelfID(string(self))

	controlTLSConf, err := rendezvous.ControlClientTLS(serverAddr, "")
	if err != nil {
		return err
	}
	joinClient, err := controlclient.Dial(ctx, serverAddr, controlTLSConf, infra.DefaultQUICConfig())
	if err != nil {
		return fmt.Errorf("app: dial control-plane: %w", err)
	}
	defer joinClient.Close()

	regResult, err := joinClient.Register(ctx, id.PublicKey, cli.Version())
	if err != nil {
		return fmt.Errorf("app: register: %w", err)
	}
	rendezvous.WarnIfVersionMismatch(cli.Version(), regResult.ServerVersion, rendezvous.VersionMismatchFunc(onVersionMismatch))

	network, err := joinClient.JoinNetwork(ctx, networkName, inviteToken, id.PublicKey)
	if err != nil {
		return fmt.Errorf("app: join network: %w", err)
	}

	selfMeshIP, ok := memberMeshIP(network, self)
	if !ok {
		return fmt.Errorf("app: server did not return our own mesh membership")
	}

	bind := wgmesh.NewBind()
	logger := device.NewLogger(device.LogLevelError, "spur: ")
	dev, err := wgmesh.NewDevice(bind, selfMeshIP, network.CIDR.Bits(), logger)
	if err != nil {
		return fmt.Errorf("app: create tun device: %w", err)
	}
	defer dev.Close()

	if err := dev.IpcSet(wgmesh.BuildDeviceConfig(id.PrivateKey)); err != nil {
		return fmt.Errorf("app: configure wireguard device: %w", err)
	}
	if err := dev.Up(); err != nil {
		return fmt.Errorf("app: bring up tun device: %w", err)
	}

	mesh := meshclient.NewPeers(serverAddr, stunAddr, resolvedIdentityPath, cli.Version(), self, bind, dev)
	defer mesh.CloseAll()

	mesh.ConnectToNewMembers(ctx, network)

	ticker := time.NewTicker(meshRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Already a member by now, so the token isn't re-checked
			// server-side — reusing it here is just convenient, not
			// required.
			network, err := joinClient.JoinNetwork(ctx, networkName, inviteToken, id.PublicKey)
			if err != nil {
				continue // transient — retry next tick
			}
			mesh.ConnectToNewMembers(ctx, network)
		}
	}
}

func memberMeshIP(network domain.Network, peer domain.PeerID) (netip.Addr, bool) {
	for _, m := range network.Members {
		if m.PeerID == peer {
			return m.MeshIP, true
		}
	}
	return netip.Addr{}, false
}
