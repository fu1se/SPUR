package nat_test

import (
	"context"
	"crypto/rand"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/localizator/internal/adapter/controlclient"
	"github.com/fu1se/localizator/internal/adapter/controlproto"
	"github.com/fu1se/localizator/internal/adapter/controlserver"
	"github.com/fu1se/localizator/internal/adapter/nat"
	"github.com/fu1se/localizator/internal/adapter/repository/memory"
	"github.com/fu1se/localizator/internal/adapter/stunserver"
	"github.com/fu1se/localizator/internal/domain"
	"github.com/fu1se/localizator/internal/infra"
	"github.com/fu1se/localizator/internal/usecase"
)

// TestPunch_EndToEnd exercises the whole Phase 3 flow between two
// simulated clients: register with the control-plane, gather host + STUN
// server-reflexive candidates on the same socket that will punch, exchange
// candidates through the rendezvous server, then hole-punch each other.
// Both clients run as local processes (goroutines) talking over real
// loopback UDP sockets — there's no actual NAT in this environment, but
// every piece of the mechanism (sockets, STUN, signaling, punching) is
// exercised for real.
func TestPunch_EndToEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	controlAddr := startControlServer(t, ctx)
	stunServerAddr := startSTUNServer(t, ctx)

	pubA := randomPublicKey(t)
	pubB := randomPublicKey(t)
	peerA := domain.DerivePeerID(pubA)
	peerB := domain.DerivePeerID(pubB)
	sessionID := domain.SessionIDFor(peerA, peerB)

	type outcome struct {
		resolved netip.AddrPort
		err      error
	}
	resultA := make(chan outcome, 1)
	resultB := make(chan outcome, 1)

	go func() {
		addr, err := runPeer(ctx, controlAddr, stunServerAddr, pubA, peerB, sessionID)
		resultA <- outcome{addr, err}
	}()
	go func() {
		addr, err := runPeer(ctx, controlAddr, stunServerAddr, pubB, peerA, sessionID)
		resultB <- outcome{addr, err}
	}()

	oa := <-resultA
	ob := <-resultB

	require.NoError(t, oa.err)
	require.NoError(t, ob.err)

	// A resolved a path to B's actual UDP socket, and vice versa: both
	// sides agree on the loopback address, and the ports are non-zero.
	require.Equal(t, "127.0.0.1", oa.resolved.Addr().String())
	require.Equal(t, "127.0.0.1", ob.resolved.Addr().String())
	require.NotZero(t, oa.resolved.Port())
	require.NotZero(t, ob.resolved.Port())
}

// runPeer performs one client's full Phase 3 flow and returns the address
// it punched through to.
func runPeer(
	ctx context.Context,
	controlAddr string,
	stunServerAddr netip.AddrPort,
	self domain.PublicKey,
	counterpart domain.PeerID,
	sessionID string,
) (netip.AddrPort, error) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		return netip.AddrPort{}, err
	}
	defer conn.Close()

	client, err := controlclient.Dial(ctx, controlAddr, infra.InsecureClientTLSConfig(controlproto.ALPN))
	if err != nil {
		return netip.AddrPort{}, err
	}
	defer client.Close()

	if _, err := client.Register(ctx, self); err != nil {
		return netip.AddrPort{}, err
	}

	hostCandidates, err := nat.HostCandidates(conn)
	if err != nil {
		return netip.AddrPort{}, err
	}
	reflexive, err := nat.DiscoverServerReflexive(ctx, conn, stunServerAddr)
	if err != nil {
		return netip.AddrPort{}, err
	}

	ownCandidates := append(hostCandidates, reflexive)

	exchange := usecase.ExchangeCandidates{Signaler: client}
	peerCandidates, err := exchange.Execute(ctx, sessionID, domain.DerivePeerID(self), counterpart, ownCandidates)
	if err != nil {
		return netip.AddrPort{}, err
	}

	puncher := &nat.UDPPuncher{Conn: conn, SessionID: sessionID}
	return puncher.Punch(ctx, peerCandidates)
}

// startControlServer binds an ephemeral UDP port synchronously (so the
// returned address is immediately dialable, no race with the accept loop
// starting up) and runs the control-plane server on it until ctx is done.
func startControlServer(t *testing.T, ctx context.Context) string {
	t.Helper()

	tlsConf, err := infra.SelfSignedServerTLSConfig(controlproto.ALPN)
	require.NoError(t, err)

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)

	peers := memory.NewPeerRepository()
	broker := memory.NewCandidateBroker()

	srv := &controlserver.Server{
		RegisterPeer:      usecase.RegisterPeer{Peers: peers},
		PublishCandidates: usecase.PublishCandidates{Store: broker},
		AwaitCandidates:   usecase.AwaitCandidates{Store: broker},
	}

	go func() {
		_ = srv.Serve(ctx, conn, tlsConf)
	}()

	return conn.LocalAddr().String()
}

// startSTUNServer binds an ephemeral UDP port synchronously and runs the
// STUN responder on it until ctx is done.
func startSTUNServer(t *testing.T, ctx context.Context) netip.AddrPort {
	t.Helper()

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	require.NoError(t, err)

	addr := conn.LocalAddr().(*net.UDPAddr).AddrPort() //nolint:forcetypeassert // net.ListenUDP always returns *net.UDPAddr

	go func() {
		_ = stunserver.Serve(ctx, conn)
	}()

	return addr
}

func randomPublicKey(t *testing.T) domain.PublicKey {
	t.Helper()

	var pub domain.PublicKey
	_, err := rand.Read(pub[:])
	require.NoError(t, err)
	return pub
}
