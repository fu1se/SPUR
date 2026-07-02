package tunnel_test

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"io"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/localizator/internal/adapter/controlclient"
	"github.com/fu1se/localizator/internal/adapter/controlproto"
	"github.com/fu1se/localizator/internal/adapter/controlserver"
	"github.com/fu1se/localizator/internal/adapter/localnet"
	"github.com/fu1se/localizator/internal/adapter/nat"
	"github.com/fu1se/localizator/internal/adapter/repository/memory"
	"github.com/fu1se/localizator/internal/adapter/stunserver"
	"github.com/fu1se/localizator/internal/adapter/tunnel"
	"github.com/fu1se/localizator/internal/domain"
	"github.com/fu1se/localizator/internal/infra"
	"github.com/fu1se/localizator/internal/usecase"
	"github.com/fu1se/localizator/internal/usecase/port"
)

// TestPortForward_EndToEnd wires up the whole Phase 5 stack the same way
// cmd/app's connect/expose commands do (register, exchange candidates,
// establish a session, build a TunnelConn, run ForwardPort/
// ServeExposedPort) and drives real bytes through it: a local TCP echo
// server stands in for "the service being exposed", and the test dials the
// forwarded port and checks it gets its own bytes back.
func TestPortForward_EndToEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	controlAddr := startControlServer(t, ctx)
	stunAddr := startSTUNServer(t, ctx)

	echoAddr := startEchoServer(t)

	pubExposer := randomPublicKey(t)
	pubConnector := randomPublicKey(t)
	peerExposer := domain.DerivePeerID(pubExposer)
	peerConnector := domain.DerivePeerID(pubConnector)

	exposeErrCh := make(chan error, 1)
	go func() {
		tun, _, err := doRendezvous(ctx, controlAddr, stunAddr, pubExposer, peerConnector)
		if err != nil {
			exposeErrCh <- err
			return
		}
		defer tun.Close()

		dialer := localnet.TCPDialer{Addr: echoAddr}
		exposeErrCh <- usecase.ServeExposedPort{Dialer: dialer, Tunnel: tun.conn}.Run(ctx)
	}()

	connTun, _, err := doRendezvous(ctx, controlAddr, stunAddr, pubConnector, peerExposer)
	require.NoError(t, err)
	defer connTun.Close()

	listener, err := localnet.ListenTCP("127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	go func() {
		_ = usecase.ForwardPort{Listener: listener, Tunnel: connTun.conn}.Run(ctx)
	}()

	conn, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)
	defer conn.Close()

	const msg = "hello through the tunnel"
	_, err = conn.Write([]byte(msg))
	require.NoError(t, err)

	buf := make([]byte, len(msg))
	_, err = io.ReadFull(conn, buf)
	require.NoError(t, err)
	require.Equal(t, msg, string(buf))

	select {
	case err := <-exposeErrCh:
		t.Fatalf("expose side exited early: %v", err)
	default:
	}
}

// establishedTunnel mirrors cmd/app's private type of the same purpose:
// keeping the TunnelConn, control connection and punched UDP socket alive
// together (see CLAUDE.md's "Время жизни relay-стрима").
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

// doRendezvous replays cmd/app's rendezvous flow using the real adapters,
// so this test exercises the exact same wiring the CLI uses.
func doRendezvous(ctx context.Context, controlAddr string, stunAddr netip.AddrPort, self domain.PublicKey, counterpart domain.PeerID) (*establishedTunnel, domain.PeerID, error) {
	selfID := domain.DerivePeerID(self)

	client, err := controlclient.Dial(ctx, controlAddr, infra.InsecureClientTLSConfig(controlproto.ALPN), infra.DefaultQUICConfig())
	if err != nil {
		return nil, "", err
	}

	if _, err := client.Register(ctx, self); err != nil {
		client.Close()
		return nil, "", err
	}

	udpConn, err := net.ListenUDP("udp", &net.UDPAddr{})
	if err != nil {
		client.Close()
		return nil, "", err
	}

	hostCandidates, err := nat.HostCandidates(udpConn)
	if err != nil {
		client.Close()
		udpConn.Close()
		return nil, "", err
	}
	reflexive, err := nat.DiscoverServerReflexive(ctx, udpConn, stunAddr)
	if err != nil {
		client.Close()
		udpConn.Close()
		return nil, "", err
	}
	ownCandidates := append(hostCandidates, reflexive)

	sessionID := domain.SessionIDFor(selfID, counterpart)

	exchange := usecase.ExchangeCandidates{Signaler: client}
	peerCandidates, err := exchange.Execute(ctx, sessionID, selfID, counterpart, ownCandidates)
	if err != nil {
		client.Close()
		udpConn.Close()
		return nil, "", err
	}

	establish := usecase.EstablishSession{
		Puncher: &nat.UDPPuncher{Conn: udpConn, SessionID: sessionID},
		Relay:   client,
	}
	session, relayStream, err := establish.Execute(ctx, sessionID, peerCandidates)
	if err != nil {
		client.Close()
		udpConn.Close()
		return nil, "", err
	}

	isDialer := domain.IsDialer(selfID, counterpart)

	var dataTLSConf *tls.Config
	if !isDialer {
		dataTLSConf, err = infra.SelfSignedServerTLSConfig(tunnel.DataALPN)
		if err != nil {
			client.Close()
			udpConn.Close()
			return nil, "", err
		}
	}

	transport := &tunnel.Transport{Conn: udpConn, TLSConf: dataTLSConf, QUICConf: infra.DefaultQUICConfig()}
	tunnelConn, err := transport.EstablishConn(ctx, session, relayStream, isDialer)
	if err != nil {
		client.Close()
		udpConn.Close()
		return nil, "", err
	}

	return &establishedTunnel{conn: tunnelConn, controlClient: client, udpConn: udpConn}, selfID, nil
}

func startControlServer(t *testing.T, ctx context.Context) string {
	t.Helper()

	tlsConf, err := infra.SelfSignedServerTLSConfig(controlproto.ALPN)
	require.NoError(t, err)

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)

	peers := memory.NewPeerRepository()
	candidateBroker := memory.NewCandidateBroker()
	relayBroker := memory.NewRelayBroker()

	srv := &controlserver.Server{
		RegisterPeer:      usecase.RegisterPeer{Peers: peers},
		PublishCandidates: usecase.PublishCandidates{Store: candidateBroker},
		AwaitCandidates:   usecase.AwaitCandidates{Store: candidateBroker},
		RelayFallback:     usecase.RelayFallback{Broker: relayBroker},
	}

	go func() { _ = srv.Serve(ctx, conn, tlsConf, infra.DefaultQUICConfig()) }()

	return conn.LocalAddr().String()
}

func startSTUNServer(t *testing.T, ctx context.Context) netip.AddrPort {
	t.Helper()

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	require.NoError(t, err)

	addr := conn.LocalAddr().(*net.UDPAddr).AddrPort() //nolint:forcetypeassert // net.ListenUDP always returns *net.UDPAddr

	go func() { _ = stunserver.Serve(ctx, conn) }()

	return addr
}

// startEchoServer stands in for "the service being exposed": it echoes
// back whatever it receives on each connection.
func startEchoServer(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}()
		}
	}()

	return ln.Addr().String()
}

func randomPublicKey(t *testing.T) domain.PublicKey {
	t.Helper()

	var pub domain.PublicKey
	_, err := rand.Read(pub[:])
	require.NoError(t, err)
	return pub
}
