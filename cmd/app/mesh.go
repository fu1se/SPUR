package main

import (
	"context"
	"fmt"
	"net/netip"
	"sync"

	"golang.zx2c4.com/wireguard/device"

	"github.com/fu1se/localizator/internal/adapter/controlclient"
	"github.com/fu1se/localizator/internal/adapter/controlproto"
	"github.com/fu1se/localizator/internal/adapter/wgmesh"
	"github.com/fu1se/localizator/internal/domain"
	"github.com/fu1se/localizator/internal/infra"
	"github.com/fu1se/localizator/internal/usecase/port"
)

// join is "app join": coordinates mesh network membership with the
// server, establishes a tunnel (P2P or relay, same EstablishSession/
// adapter/tunnel machinery as port-forward mode) to every other member,
// dedicates one stream per peer to carrying WireGuard traffic, and routes
// it through a real TUN interface.
//
// Requires elevated privileges (root/CAP_NET_ADMIN on Linux): creating the
// TUN device and assigning it an address changes real system network
// state. See CLAUDE.md's Phase 6 note for what was and wasn't verified
// with a live interface.
func join(ctx context.Context, serverAddr, stunAddr, networkName, identityPath string, onSelfID func(string)) error {
	resolvedIdentityPath, err := resolveIdentityPath(identityPath)
	if err != nil {
		return err
	}
	id, err := infra.LoadOrCreateIdentity(resolvedIdentityPath)
	if err != nil {
		return fmt.Errorf("app: load identity: %w", err)
	}
	self := domain.DerivePeerID(id.PublicKey)
	onSelfID(string(self))

	joinClient, err := controlclient.Dial(ctx, serverAddr, infra.InsecureClientTLSConfig(controlproto.ALPN), infra.DefaultQUICConfig())
	if err != nil {
		return fmt.Errorf("app: dial control-plane: %w", err)
	}
	defer joinClient.Close()

	network, err := joinClient.JoinNetwork(ctx, networkName, id.PublicKey)
	if err != nil {
		return fmt.Errorf("app: join network: %w", err)
	}

	var selfMeshIP netip.Addr
	found := false
	for _, m := range network.Members {
		if m.PeerID == self {
			selfMeshIP = m.MeshIP
			found = true
		}
	}
	if !found {
		return fmt.Errorf("app: server did not return our own mesh membership")
	}

	bind := wgmesh.NewBind()

	tunnels, peerCfgs := establishMeshPeers(ctx, serverAddr, stunAddr, resolvedIdentityPath, self, network, bind)
	defer func() {
		for _, tun := range tunnels {
			tun.Close()
		}
	}()
	if len(peerCfgs) == 0 && len(network.Members) > 1 {
		return fmt.Errorf("app: could not reach any of %d other network members", len(network.Members)-1)
	}

	logger := device.NewLogger(device.LogLevelError, "localizator: ")
	dev, err := wgmesh.NewDevice(bind, selfMeshIP, network.CIDR.Bits(), logger)
	if err != nil {
		return fmt.Errorf("app: create tun device: %w", err)
	}
	defer dev.Close()

	if err := dev.IpcSet(wgmesh.BuildUAPIConfig(id.PrivateKey, peerCfgs)); err != nil {
		return fmt.Errorf("app: configure wireguard device: %w", err)
	}
	if err := dev.Up(); err != nil {
		return fmt.Errorf("app: bring up tun device: %w", err)
	}

	<-ctx.Done()
	return ctx.Err()
}

// establishMeshPeers rendezvous-es with every other member of network
// concurrently and registers each resulting stream with bind. A peer that
// can't be reached (punch and relay both fail, or it's simply offline) is
// skipped rather than failing the whole join — best-effort mesh
// connectivity, matching how a real VPN mesh degrades when one node is
// down.
func establishMeshPeers(
	ctx context.Context,
	serverAddr, stunAddr, identityPath string,
	self domain.PeerID,
	network domain.Network,
	bind *wgmesh.Bind,
) ([]*establishedTunnel, []wgmesh.PeerConfig) {
	var (
		mu       sync.Mutex
		tunnels  []*establishedTunnel
		peerCfgs []wgmesh.PeerConfig
		wg       sync.WaitGroup
	)

	for _, member := range network.Members {
		if member.PeerID == self {
			continue
		}

		wg.Add(1)
		go func(m domain.MeshMember) {
			defer wg.Done()

			tun, _, err := rendezvous(ctx, serverAddr, stunAddr, identityPath, m.PeerID, func(string) {})
			if err != nil {
				return
			}

			stream, err := meshStream(ctx, tun.conn, self, m.PeerID)
			if err != nil {
				tun.Close()
				return
			}
			bind.AddPeer(m.PeerID, stream)

			mu.Lock()
			tunnels = append(tunnels, tun)
			peerCfgs = append(peerCfgs, wgmesh.PeerConfig{
				PublicKey: m.PublicKey,
				AllowedIP: netip.PrefixFrom(m.MeshIP, m.MeshIP.BitLen()),
				Endpoint:  string(m.PeerID),
			})
			mu.Unlock()
		}(member)
	}

	wg.Wait()
	return tunnels, peerCfgs
}

// meshStream opens (if we're the dialer) or accepts (otherwise) the single
// stream a mesh peer's WireGuard traffic rides on, using the same
// domain.IsDialer convention as port-forward mode.
func meshStream(ctx context.Context, tunnelConn port.TunnelConn, self, counterpart domain.PeerID) (port.Stream, error) {
	if domain.IsDialer(self, counterpart) {
		return tunnelConn.OpenStream(ctx)
	}
	return tunnelConn.AcceptStream(ctx)
}
