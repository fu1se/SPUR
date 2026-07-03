package main

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/device"

	"github.com/fu1se/spur/internal/adapter/cli"
	"github.com/fu1se/spur/internal/adapter/controlclient"
	"github.com/fu1se/spur/internal/adapter/rendezvous"
	"github.com/fu1se/spur/internal/adapter/wgmesh"
	"github.com/fu1se/spur/internal/domain"
	"github.com/fu1se/spur/internal/infra"
	"github.com/fu1se/spur/internal/usecase/port"
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
// state.
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

	mesh := &meshPeers{
		serverAddr:   serverAddr,
		stunAddr:     stunAddr,
		identityPath: resolvedIdentityPath,
		self:         self,
		bind:         bind,
		dev:          dev,
		connected:    make(map[domain.PeerID]*rendezvous.Tunnel),
	}
	defer mesh.closeAll()

	mesh.connectToNewMembers(ctx, network)

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
			mesh.connectToNewMembers(ctx, network)
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

// meshPeers tracks which mesh members already have an established tunnel,
// so repeated calls to connectToNewMembers only act on newly seen ones,
// and owns cleanup of every tunnel it opened.
type meshPeers struct {
	serverAddr, stunAddr, identityPath string
	self                               domain.PeerID
	bind                               *wgmesh.Bind
	dev                                *wgmesh.Device

	mu        sync.Mutex
	connected map[domain.PeerID]*rendezvous.Tunnel
}

// connectToNewMembers rendezvous-es (concurrently) with every member of
// network not already connected, registers each resulting stream with
// bind, and incrementally adds it to the WireGuard device. A peer that
// can't be reached right now (punch and relay both fail, or it's offline)
// is simply skipped — the next tick tries again, so this degrades
// gracefully rather than failing the whole join.
func (m *meshPeers) connectToNewMembers(ctx context.Context, network domain.Network) {
	var wg sync.WaitGroup

	for _, member := range network.Members {
		if member.PeerID == m.self {
			continue
		}

		if m.reapDeadConnection(member.PeerID) {
			continue // still alive, leave it alone
		}

		wg.Add(1)
		go func(mem domain.MeshMember) {
			defer wg.Done()
			m.connectOne(ctx, mem)
		}(member)
	}

	wg.Wait()
}

// reapDeadConnection reports whether peer already has a live tunnel. If
// mesh.connected still thinks it's connected but the underlying stream has
// since died (wgmesh.Bind forgets a peer's stream once its own read loop
// exits — see Bind.readLoop), the stale tunnel is closed and this reports
// false instead, so the caller retries connectOne on the next call rather
// than leaving that peer permanently unreachable until the whole process
// restarts.
func (m *meshPeers) reapDeadConnection(peer domain.PeerID) bool {
	m.mu.Lock()
	tun, ok := m.connected[peer]
	stale := ok && !m.bind.HasPeer(peer)
	if stale {
		delete(m.connected, peer)
	}
	m.mu.Unlock()

	if stale {
		tun.Close()
		return false
	}
	return ok
}

func (m *meshPeers) connectOne(ctx context.Context, mem domain.MeshMember) {
	resolve := rendezvous.FixedCounterpart(mem.PeerID)
	tun, _, _, err := rendezvous.Establish(ctx, m.serverAddr, m.stunAddr, m.identityPath, "", cli.Version(), resolve, func(string) {}, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "spur: mesh: rendezvous with %s failed: %v\n", mem.PeerID, err)
		return
	}

	isDialer := domain.IsDialer(m.self, mem.PeerID)
	stream, err := meshStream(ctx, tun.Conn, m.self, mem.PeerID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "spur: mesh: open stream to %s failed: %v\n", mem.PeerID, err)
		tun.Close()
		return
	}
	if err := m.bind.AddPeer(mem.PeerID, stream, isDialer); err != nil {
		fmt.Fprintf(os.Stderr, "spur: mesh: register stream for %s failed: %v\n", mem.PeerID, err)
		tun.Close()
		return
	}

	cfg := wgmesh.PeerConfig{
		PublicKey: mem.PublicKey,
		AllowedIP: netip.PrefixFrom(mem.MeshIP, mem.MeshIP.BitLen()),
		Endpoint:  string(mem.PeerID),
	}
	// No listen_port here — see BuildDeviceConfig's doc comment for why
	// that matters: only adds/updates mem, existing peers untouched.
	if err := m.dev.IpcSet(wgmesh.BuildPeersConfig([]wgmesh.PeerConfig{cfg})); err != nil {
		// Don't mark this peer connected: doing so unconditionally used
		// to permanently exclude it from every future retry the moment
		// IpcSet failed even once, since connectToNewMembers' liveness
		// check would then see it as already connected forever, with no
		// log line anywhere explaining why that peer never showed up.
		fmt.Fprintf(os.Stderr, "spur: mesh: configure wireguard peer %s failed: %v\n", mem.PeerID, err)
		m.bind.RemovePeer(mem.PeerID)
		tun.Close()
		return
	}

	m.mu.Lock()
	m.connected[mem.PeerID] = tun
	m.mu.Unlock()
}

func (m *meshPeers) closeAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, tun := range m.connected {
		tun.Close()
	}
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
