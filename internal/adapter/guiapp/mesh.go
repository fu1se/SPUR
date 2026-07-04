package guiapp

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

// meshRefreshInterval mirrors cmd/spur/mesh.go's constant of the same
// name — see its doc comment for why periodic re-polling of membership
// is necessary, not just an optimization.
const meshRefreshInterval = 5 * time.Second

// JoinNetwork registers with a mesh network on the server and returns its
// current membership, without touching TUN/WireGuard at all — mirrors
// cmd/spur's "spur join-network": a control-plane-only diagnostic, and
// also how a GUI can preview a network's membership/CIDR before actually
// starting a MeshSession.
func (c *Client) JoinNetwork(ctx context.Context, serverAddr, networkName, inviteToken string, onVersionMismatch cli.VersionMismatchFunc) (cli.JoinNetworkResult, error) {
	client, id, err := rendezvous.DialAndRegister(ctx, serverAddr, c.identityPath, "", cli.Version(), rendezvous.VersionMismatchFunc(onVersionMismatch))
	if err != nil {
		return cli.JoinNetworkResult{}, err
	}
	defer client.Close()

	network, err := client.JoinNetwork(ctx, networkName, inviteToken, id.PublicKey)
	if err != nil {
		return cli.JoinNetworkResult{}, err
	}

	members := make([]cli.MeshMemberResult, 0, len(network.Members))
	for _, m := range network.Members {
		members = append(members, cli.MeshMemberResult{PeerID: string(m.PeerID), MeshIP: m.MeshIP.String()})
	}
	return cli.JoinNetworkResult{CIDR: network.CIDR.String(), Members: members, InviteToken: network.InviteToken}, nil
}

// MeshSession is a running "join" (mesh VPN) session — see
// Client.StartMesh. Owns the real TUN interface and WireGuard device for
// as long as the session runs; Stop tears everything down (every peer
// tunnel, the device, the control-plane connection).
type MeshSession struct {
	cancel context.CancelFunc
	done   chan error
}

// Stop tears down the mesh session. Safe to call more than once.
func (s *MeshSession) Stop() { s.cancel() }

// Wait blocks until the session ends — either Stop was called, or the
// background membership loop's context was otherwise cancelled.
func (s *MeshSession) Wait() error { return <-s.done }

// StartMesh is "spur join": joins networkName on the server, creates a
// real TUN interface (requires root/CAP_NET_ADMIN on Linux — the same
// requirement cmd/spur's "join" has), and tunnels to every other member,
// refreshing membership every meshRefreshInterval in the background.
// Blocks until the network is joined and the device is up (before the
// first membership sync) — call this from a background goroutine, not
// the GUI's event-dispatch thread, since bringing up TUN plus the first
// round of per-peer tunnel establishment can take a while.
func (c *Client) StartMesh(ctx context.Context, serverAddr, stunAddr, networkName, inviteToken string, verbose bool, onSelfID func(string), onVersionMismatch cli.VersionMismatchFunc) (*MeshSession, error) {
	resolvedIdentityPath, err := rendezvous.ResolveIdentityPath(c.identityPath)
	if err != nil {
		return nil, err
	}
	id, err := infra.LoadOrCreateIdentity(resolvedIdentityPath)
	if err != nil {
		return nil, fmt.Errorf("guiapp: load identity: %w", err)
	}
	self := domain.DerivePeerID(id.PublicKey)
	if onSelfID != nil {
		onSelfID(string(self))
	}

	controlTLSConf, err := rendezvous.ControlClientTLS(serverAddr, "")
	if err != nil {
		return nil, err
	}
	joinClient, err := controlclient.Dial(ctx, serverAddr, controlTLSConf, infra.DefaultQUICConfig())
	if err != nil {
		return nil, fmt.Errorf("guiapp: dial control-plane: %w", err)
	}

	regResult, err := joinClient.Register(ctx, id.PublicKey, cli.Version())
	if err != nil {
		joinClient.Close()
		return nil, fmt.Errorf("guiapp: register: %w", err)
	}
	rendezvous.WarnIfVersionMismatch(cli.Version(), regResult.ServerVersion, rendezvous.VersionMismatchFunc(onVersionMismatch))

	network, err := joinClient.JoinNetwork(ctx, networkName, inviteToken, id.PublicKey)
	if err != nil {
		joinClient.Close()
		return nil, fmt.Errorf("guiapp: join network: %w", err)
	}

	selfMeshIP, ok := memberMeshIP(network, self)
	if !ok {
		joinClient.Close()
		return nil, fmt.Errorf("guiapp: server did not return our own mesh membership")
	}

	bind := wgmesh.NewBind()
	logLevel := device.LogLevelError
	if verbose {
		logLevel = device.LogLevelVerbose
	}
	logger := device.NewLogger(logLevel, "spur: ")
	dev, err := wgmesh.NewDevice(bind, selfMeshIP, network.CIDR.Bits(), logger)
	if err != nil {
		joinClient.Close()
		return nil, fmt.Errorf("guiapp: create tun device: %w", err)
	}

	if err := dev.IpcSet(wgmesh.BuildDeviceConfig(id.PrivateKey)); err != nil {
		dev.Close()
		joinClient.Close()
		return nil, fmt.Errorf("guiapp: configure wireguard device: %w", err)
	}
	if err := dev.Up(); err != nil {
		dev.Close()
		joinClient.Close()
		return nil, fmt.Errorf("guiapp: bring up tun device: %w", err)
	}

	mesh := meshclient.NewPeers(serverAddr, stunAddr, resolvedIdentityPath, "", cli.Version(), self, bind, dev)
	mesh.ConnectToNewMembers(ctx, network)

	sessionCtx, cancel := context.WithCancel(context.Background())
	session := &MeshSession{cancel: cancel, done: make(chan error, 1)}

	go func() {
		defer joinClient.Close()
		defer dev.Close()
		defer mesh.CloseAll()

		ticker := time.NewTicker(meshRefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-sessionCtx.Done():
				session.done <- nil
				return
			case <-ticker.C:
				network, err := joinClient.JoinNetwork(sessionCtx, networkName, inviteToken, id.PublicKey)
				if err != nil {
					continue // transient — retry next tick
				}
				mesh.ConnectToNewMembers(sessionCtx, network)
			}
		}
	}()

	return session, nil
}

func memberMeshIP(network domain.Network, peer domain.PeerID) (netip.Addr, bool) {
	for _, m := range network.Members {
		if m.PeerID == peer {
			return m.MeshIP, true
		}
	}
	return netip.Addr{}, false
}
