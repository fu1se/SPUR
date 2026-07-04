package spurmobile

import (
	"context"
	"fmt"
	"time"

	"golang.zx2c4.com/wireguard/device"

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

// MeshNetworkInfo is what a caller needs to configure a
// android.net.VpnService.Builder *before* a TUN file descriptor exists —
// Builder.addAddress/addRoute must be called before establish() returns
// the fd that JoinMesh needs, so resolving network membership has to
// happen in a separate, earlier step than JoinMesh itself. Calling
// JoinNetwork twice (once here, once inside JoinMesh) is safe: it's
// idempotent for an already-known member (see usecase.JoinNetwork's doc
// comment).
type MeshNetworkInfo struct {
	SelfMeshIP string
	CIDRBits   int
}

// ResolveMeshNetwork joins (or re-joins) networkName just far enough to
// learn this client's own mesh address and the network's prefix length —
// call this before building the VpnService, then call JoinMesh once the
// TUN fd is ready.
func (c *Client) ResolveMeshNetwork(serverAddr, networkName, inviteToken string) (*MeshNetworkInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), registerTimeout)
	defer cancel()

	client, id, err := rendezvous.DialAndRegister(ctx, serverAddr, c.identityPath, c.trustStorePath, Version(), nil)
	if err != nil {
		return nil, explain(err)
	}
	defer client.Close()

	network, err := client.JoinNetwork(ctx, networkName, inviteToken, id.PublicKey)
	if err != nil {
		return nil, explain(fmt.Errorf("spurmobile: join network: %w", err))
	}

	self := domain.DerivePeerID(id.PublicKey)
	for _, m := range network.Members {
		if m.PeerID == self {
			return &MeshNetworkInfo{SelfMeshIP: m.MeshIP.String(), CIDRBits: network.CIDR.Bits()}, nil
		}
	}
	return nil, explain(fmt.Errorf("spurmobile: server did not return our own mesh membership"))
}

// MeshSession is a running "join network" session (see Client.JoinMesh).
// Owns the WireGuard device wrapping the TUN fd handed in by the caller
// and keeps membership synced in a background goroutine until Stop.
type MeshSession struct {
	cancel context.CancelFunc
	done   chan error
}

// Stop tears down the mesh session: closes every peer tunnel, the
// WireGuard device, and the control-plane connection.
func (s *MeshSession) Stop() { s.cancel() }

// Await blocks until the session ends — either Stop was called, or it
// failed to even start configuring the device (see JoinMesh's doc
// comment: everything up to and including bringing the device up
// happens before JoinMesh returns, so Await only ever reports the
// clean-Stop case in practice, nil).
func (s *MeshSession) Await() error { return <-s.done }

// JoinMesh is "spur join" for Android: joins networkName on the
// control-plane server and configures the WireGuard device wrapping
// tunFd — a file descriptor android.net.VpnService.Builder.establish()
// already created and configured with the mesh address/routes (see
// ResolveMeshNetwork), since a regular app process can't create a TUN
// interface itself the way the desktop CLI does (see
// wgmesh.NewDeviceFromFD's doc comment). Blocks until the device is up
// and the first membership sync completes before returning; ongoing
// membership polling and per-peer tunnel setup then runs in the
// background until Stop — same shape as StartConnect/StartSend, and the
// same meshclient.Peers desktop's "spur join" uses.
func (c *Client) JoinMesh(serverAddr, stunAddr, networkName, inviteToken string, tunFd int) (*MeshSession, error) {
	ctx := context.Background()

	id, err := infra.LoadOrCreateIdentity(c.identityPath)
	if err != nil {
		return nil, explain(fmt.Errorf("spurmobile: load identity: %w", err))
	}
	self := domain.DerivePeerID(id.PublicKey)

	controlTLSConf, err := rendezvous.ControlClientTLS(serverAddr, c.trustStorePath)
	if err != nil {
		return nil, explain(err)
	}
	joinClient, err := controlclient.Dial(ctx, serverAddr, controlTLSConf, infra.DefaultQUICConfig())
	if err != nil {
		return nil, explain(fmt.Errorf("spurmobile: dial control-plane: %w", err))
	}

	if _, err := joinClient.Register(ctx, id.PublicKey, Version()); err != nil {
		joinClient.Close()
		return nil, explain(fmt.Errorf("spurmobile: register: %w", err))
	}

	network, err := joinClient.JoinNetwork(ctx, networkName, inviteToken, id.PublicKey)
	if err != nil {
		joinClient.Close()
		return nil, explain(fmt.Errorf("spurmobile: join network: %w", err))
	}

	bind := wgmesh.NewBind()
	logger := device.NewLogger(device.LogLevelError, "spur: ")
	dev, err := wgmesh.NewDeviceFromFD(bind, tunFd, logger)
	if err != nil {
		joinClient.Close()
		return nil, explain(fmt.Errorf("spurmobile: wrap tun device: %w", err))
	}

	if err := dev.IpcSet(wgmesh.BuildDeviceConfig(id.PrivateKey)); err != nil {
		dev.Close()
		joinClient.Close()
		return nil, explain(fmt.Errorf("spurmobile: configure wireguard device: %w", err))
	}
	if err := dev.Up(); err != nil {
		dev.Close()
		joinClient.Close()
		return nil, explain(fmt.Errorf("spurmobile: bring up tun device: %w", err))
	}

	mesh := meshclient.NewPeers(serverAddr, stunAddr, c.identityPath, c.trustStorePath, Version(), self, bind, dev)
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
				// Already a member by now, so the token isn't re-checked
				// server-side — reusing it here is just convenient, not
				// required (mirrors cmd/spur/mesh.go's join loop).
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
