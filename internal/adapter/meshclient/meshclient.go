// Package meshclient holds the per-peer tunnel orchestration shared by
// every mesh (WireGuard) client — desktop's "spur join" and, per
// CLAUDE.md's Android roadmap, the Android facade's mesh join. Both
// drive the exact same membership-polling and per-peer-tunnel logic;
// only how the TUN device itself gets created differs (wgmesh.NewDevice
// on desktop needs CAP_NET_ADMIN to create the interface itself,
// wgmesh.NewDeviceFromFD on Android just wraps a file descriptor
// android.net.VpnService already created and configured) — so that part
// stays with each caller, not here.
package meshclient

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"sync"

	"github.com/fu1se/spur/internal/adapter/rendezvous"
	"github.com/fu1se/spur/internal/adapter/wgmesh"
	"github.com/fu1se/spur/internal/domain"
	"github.com/fu1se/spur/internal/usecase/port"
)

// Peers tracks which mesh members already have an established tunnel, so
// repeated calls to ConnectToNewMembers only act on newly seen ones, and
// owns cleanup of every tunnel it opened.
type Peers struct {
	ServerAddr, StunAddr, IdentityPath, TrustStorePath, ClientVersion string
	Self                                                              domain.PeerID
	Bind                                                              *wgmesh.Bind
	Dev                                                               *wgmesh.Device

	mu        sync.Mutex
	connected map[domain.PeerID]*rendezvous.Tunnel
}

// NewPeers builds a Peers ready to track connections for one mesh join.
// trustStorePath is forwarded to every rendezvous.Establish call this
// Peers makes — empty string means "use the default"
// (os.UserConfigDir()-based), which is what desktop's "spur join" wants
// but breaks on Android, where the app process has neither $HOME nor
// $XDG_CONFIG_HOME set (see android/spurmobile.Client's app-private
// config dir).
func NewPeers(serverAddr, stunAddr, identityPath, trustStorePath, clientVersion string, self domain.PeerID, bind *wgmesh.Bind, dev *wgmesh.Device) *Peers {
	return &Peers{
		ServerAddr:     serverAddr,
		StunAddr:       stunAddr,
		IdentityPath:   identityPath,
		TrustStorePath: trustStorePath,
		ClientVersion:  clientVersion,
		Self:           self,
		Bind:           bind,
		Dev:            dev,
		connected:      make(map[domain.PeerID]*rendezvous.Tunnel),
	}
}

// ConnectToNewMembers rendezvous-es (concurrently) with every member of
// network not already connected, registers each resulting stream with
// Bind, and incrementally adds it to the WireGuard device. A peer that
// can't be reached right now (punch and relay both fail, or it's
// offline) is simply skipped — the caller's next poll tries again, so
// this degrades gracefully rather than failing the whole join.
func (m *Peers) ConnectToNewMembers(ctx context.Context, network domain.Network) {
	var wg sync.WaitGroup

	for _, member := range network.Members {
		if member.PeerID == m.Self {
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
// Peers.connected still thinks it's connected but the underlying stream
// has since died (wgmesh.Bind forgets a peer's stream once its own read
// loop exits — see Bind.readLoop), the stale tunnel is closed and this
// reports false instead, so the caller retries connectOne on the next
// call rather than leaving that peer permanently unreachable until the
// whole process restarts.
func (m *Peers) reapDeadConnection(peer domain.PeerID) bool {
	m.mu.Lock()
	tun, ok := m.connected[peer]
	stale := ok && !m.Bind.HasPeer(peer)
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

func (m *Peers) connectOne(ctx context.Context, mem domain.MeshMember) {
	resolve := rendezvous.FixedCounterpart(mem.PeerID)
	tun, _, _, err := rendezvous.Establish(ctx, m.ServerAddr, m.StunAddr, m.IdentityPath, m.TrustStorePath, m.ClientVersion, resolve, func(string) {}, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "spur: mesh: rendezvous with %s failed: %v\n", mem.PeerID, err)
		return
	}

	isDialer := domain.IsDialer(m.Self, mem.PeerID)
	stream, err := meshStream(ctx, tun.Conn, m.Self, mem.PeerID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "spur: mesh: open stream to %s failed: %v\n", mem.PeerID, err)
		tun.Close()
		return
	}
	if err := m.Bind.AddPeer(mem.PeerID, stream, isDialer); err != nil {
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
	if err := m.Dev.IpcSet(wgmesh.BuildPeersConfig([]wgmesh.PeerConfig{cfg})); err != nil {
		// Don't mark this peer connected: doing so unconditionally used
		// to permanently exclude it from every future retry the moment
		// IpcSet failed even once, since ConnectToNewMembers' liveness
		// check would then see it as already connected forever, with no
		// log line anywhere explaining why that peer never showed up.
		fmt.Fprintf(os.Stderr, "spur: mesh: configure wireguard peer %s failed: %v\n", mem.PeerID, err)
		m.Bind.RemovePeer(mem.PeerID)
		tun.Close()
		return
	}

	m.mu.Lock()
	m.connected[mem.PeerID] = tun
	m.mu.Unlock()
}

// CloseAll tears down every tunnel this Peers opened.
func (m *Peers) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, tun := range m.connected {
		tun.Close()
	}
}

// meshStream opens (if we're the dialer) or accepts (otherwise) the
// single stream a mesh peer's WireGuard traffic rides on, using the same
// domain.IsDialer convention as port-forward mode.
func meshStream(ctx context.Context, tunnelConn port.TunnelConn, self, counterpart domain.PeerID) (port.Stream, error) {
	if domain.IsDialer(self, counterpart) {
		return tunnelConn.OpenStream(ctx)
	}
	return tunnelConn.AcceptStream(ctx)
}
