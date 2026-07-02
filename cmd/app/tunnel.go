package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"

	"github.com/fu1se/localizator/internal/adapter/controlclient"
	"github.com/fu1se/localizator/internal/adapter/controlproto"
	"github.com/fu1se/localizator/internal/adapter/localnet"
	"github.com/fu1se/localizator/internal/adapter/nat"
	"github.com/fu1se/localizator/internal/adapter/tunnel"
	"github.com/fu1se/localizator/internal/domain"
	"github.com/fu1se/localizator/internal/infra"
	"github.com/fu1se/localizator/internal/usecase"
	"github.com/fu1se/localizator/internal/usecase/port"
)

// establishedTunnel bundles a ready-to-use TunnelConn with the resources it
// depends on staying open (the control connection and the punched UDP
// socket) — see CLAUDE.md's "Время жизни relay-стрима" for why the control
// connection can't be closed early.
type establishedTunnel struct {
	conn          port.TunnelConn
	controlClient *controlclient.Client
	udpConn       *net.UDPConn
}

func (t *establishedTunnel) Close() {
	_ = t.conn.Close()
	_ = t.controlClient.Close()
	_ = t.udpConn.Close()
}

// resolveIdentityPath falls back to infra.DefaultIdentityPath when the user
// didn't pass --identity.
func resolveIdentityPath(identityPath string) (string, error) {
	if identityPath != "" {
		return identityPath, nil
	}
	return infra.DefaultIdentityPath()
}

// rendezvous runs the full client-side flow shared by "app connect" and
// "app expose": load (or create) a persisted identity, register, gather
// and exchange NAT candidates, establish a session (punch or relay
// fallback), and build the resulting data-plane TunnelConn.
//
// onSelfID fires as soon as self is known — right after registration,
// well before the counterpart-dependent steps (candidate exchange can
// block for up to a minute; see controlserver's awaitCandidatesTimeout).
// That ordering matters for bootstrapping: with no discovery mechanism
// yet (Phase 7), the only way a user learns their own peer ID is to run
// connect/expose once, read it from this callback, and Ctrl+C — the
// persisted identity (see infra.LoadOrCreateIdentity) means the ID is
// unchanged on the next, correctly-addressed run. Documented as a known
// interim limitation in CLAUDE.md.
func rendezvous(ctx context.Context, serverAddr, stunAddr, identityPath string, counterpart domain.PeerID, onSelfID func(string)) (tun *establishedTunnel, self domain.PeerID, err error) {
	resolvedIdentityPath, err := resolveIdentityPath(identityPath)
	if err != nil {
		return nil, "", err
	}
	pub, err := infra.LoadOrCreateIdentity(resolvedIdentityPath)
	if err != nil {
		return nil, "", fmt.Errorf("app: load identity: %w", err)
	}
	self = domain.DerivePeerID(pub)

	client, err := controlclient.Dial(ctx, serverAddr, infra.InsecureClientTLSConfig(controlproto.ALPN), infra.DefaultQUICConfig())
	if err != nil {
		return nil, "", fmt.Errorf("app: dial control-plane: %w", err)
	}
	defer func() {
		if err != nil {
			client.Close()
		}
	}()

	if _, err = client.Register(ctx, pub); err != nil {
		return nil, "", fmt.Errorf("app: register: %w", err)
	}
	onSelfID(string(self))

	udpConn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return nil, "", fmt.Errorf("app: bind data socket: %w", err)
	}
	defer func() {
		if err != nil {
			udpConn.Close()
		}
	}()

	stunUDPAddr, err := net.ResolveUDPAddr("udp", stunAddr)
	if err != nil {
		return nil, "", fmt.Errorf("app: resolve stun addr: %w", err)
	}

	hostCandidates, err := nat.HostCandidates(udpConn)
	if err != nil {
		return nil, "", fmt.Errorf("app: gather host candidates: %w", err)
	}
	reflexive, err := nat.DiscoverServerReflexive(ctx, udpConn, stunUDPAddr.AddrPort())
	if err != nil {
		return nil, "", fmt.Errorf("app: stun discovery: %w", err)
	}
	ownCandidates := append(hostCandidates, reflexive)

	sessionID := domain.SessionIDFor(self, counterpart)

	exchange := usecase.ExchangeCandidates{Signaler: client}
	peerCandidates, err := exchange.Execute(ctx, sessionID, self, counterpart, ownCandidates)
	if err != nil {
		return nil, "", fmt.Errorf("app: exchange candidates: %w", err)
	}

	establish := usecase.EstablishSession{
		Puncher: &nat.UDPPuncher{Conn: udpConn, SessionID: sessionID},
		Relay:   client,
	}
	session, relayStream, err := establish.Execute(ctx, sessionID, peerCandidates)
	if err != nil {
		return nil, "", fmt.Errorf("app: establish session: %w", err)
	}

	isDialer := domain.IsDialer(self, counterpart)

	var dataTLSConf *tls.Config
	if !isDialer {
		dataTLSConf, err = infra.SelfSignedServerTLSConfig(tunnel.DataALPN)
		if err != nil {
			return nil, "", fmt.Errorf("app: data-plane tls config: %w", err)
		}
	}

	transport := &tunnel.Transport{Conn: udpConn, TLSConf: dataTLSConf, QUICConf: infra.DefaultQUICConfig()}
	tunnelConn, err := transport.EstablishConn(ctx, session, relayStream, isDialer)
	if err != nil {
		return nil, "", fmt.Errorf("app: establish data-plane transport: %w", err)
	}

	return &establishedTunnel{conn: tunnelConn, controlClient: client, udpConn: udpConn}, self, nil
}

// connect is "app connect": forward every local connection on localPort
// through a tunnel to counterpart, who must be running "app expose".
func connect(ctx context.Context, serverAddr, stunAddr, counterpartID, identityPath string, localPort int, onSelfID func(string)) error {
	tun, _, err := rendezvous(ctx, serverAddr, stunAddr, identityPath, domain.PeerID(counterpartID), onSelfID)
	if err != nil {
		return err
	}
	defer tun.Close()

	listener, err := localnet.ListenTCP(fmt.Sprintf(":%d", localPort))
	if err != nil {
		return fmt.Errorf("app: listen locally: %w", err)
	}
	defer listener.Close()

	return usecase.ForwardPort{Listener: listener, Tunnel: tun.conn}.Run(ctx)
}

// expose is "app expose": accept tunnel streams from counterpart and
// forward each to targetPort on the local machine.
func expose(ctx context.Context, serverAddr, stunAddr, counterpartID, identityPath string, targetPort int, onSelfID func(string)) error {
	tun, _, err := rendezvous(ctx, serverAddr, stunAddr, identityPath, domain.PeerID(counterpartID), onSelfID)
	if err != nil {
		return err
	}
	defer tun.Close()

	dialer := localnet.TCPDialer{Addr: fmt.Sprintf("127.0.0.1:%d", targetPort)}

	return usecase.ServeExposedPort{Dialer: dialer, Tunnel: tun.conn}.Run(ctx)
}
