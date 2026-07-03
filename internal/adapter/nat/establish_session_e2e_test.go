package nat_test

import (
	"context"
	"io"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/spur/internal/adapter/controlclient"
	"github.com/fu1se/spur/internal/adapter/controlproto"
	"github.com/fu1se/spur/internal/adapter/nat"
	"github.com/fu1se/spur/internal/domain"
	"github.com/fu1se/spur/internal/infra"
	"github.com/fu1se/spur/internal/usecase"
)

// TestEstablishSession_FallsBackToRelay verifies the Phase 4 flow end to
// end: candidates that don't correspond to any real listener make punching
// time out, and EstablishSession automatically falls back to a relayed
// stream through the rendezvous server. Confirms the fallback by actually
// exchanging bytes over the resulting stream with a peer that did the same
// thing on the other end.
func TestEstablishSession_FallsBackToRelay(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	controlAddr := startControlServer(t, ctx)

	// Reserve two ports and immediately close them: nothing listens on
	// these UDP addresses, so punching against them can only time out.
	deadA := reserveDeadUDPPort(t)
	deadB := reserveDeadUDPPort(t)

	pubA := randomPublicKey(t)
	pubB := randomPublicKey(t)
	peerA := domain.DerivePeerID(pubA)
	peerB := domain.DerivePeerID(pubB)
	sessionID := domain.SessionIDFor(peerA, peerB)

	type outcome struct {
		session domain.Session
		stream  io.ReadWriteCloser
		err     error
	}
	resultA := make(chan outcome, 1)
	resultB := make(chan outcome, 1)

	go func() {
		s, stream, err := runFallbackPeer(ctx, controlAddr, pubA, []netip.AddrPort{deadA}, sessionID)
		resultA <- outcome{s, stream, err}
	}()
	go func() {
		s, stream, err := runFallbackPeer(ctx, controlAddr, pubB, []netip.AddrPort{deadB}, sessionID)
		resultB <- outcome{s, stream, err}
	}()

	oa := <-resultA
	ob := <-resultB

	require.NoError(t, oa.err)
	require.NoError(t, ob.err)
	require.Equal(t, domain.SessionEstablishedRelay, oa.session.State)
	require.Equal(t, domain.SessionEstablishedRelay, ob.session.State)
	require.NotNil(t, oa.stream)
	require.NotNil(t, ob.stream)
	defer oa.stream.Close()
	defer ob.stream.Close()

	_, err := oa.stream.Write([]byte("ping"))
	require.NoError(t, err)

	buf := make([]byte, 4)
	_, err = io.ReadFull(ob.stream, buf)
	require.NoError(t, err)
	require.Equal(t, "ping", string(buf))
}

func runFallbackPeer(
	ctx context.Context,
	controlAddr string,
	self domain.PublicKey,
	deadCandidateAddrs []netip.AddrPort,
	sessionID string,
) (domain.Session, io.ReadWriteCloser, error) {
	// client is intentionally not deferred-closed here: when EstablishSession
	// falls back to relay, the returned stream is a *quic.Stream that lives
	// on this connection, so closing it prematurely would tear the stream
	// down too. Ownership is handed to relayStreamWithConn below instead.
	client, err := controlclient.Dial(ctx, controlAddr, infra.InsecureClientTLSConfig(controlproto.ALPN), infra.DefaultQUICConfig())
	if err != nil {
		return domain.Session{}, nil, err
	}

	if _, err := client.Register(ctx, self, "test"); err != nil {
		client.Close()
		return domain.Session{}, nil, err
	}

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		client.Close()
		return domain.Session{}, nil, err
	}
	defer conn.Close()

	var deadCandidates []domain.Candidate
	for _, a := range deadCandidateAddrs {
		deadCandidates = append(deadCandidates, domain.Candidate{Kind: domain.CandidateHost, Addr: a})
	}

	establish := usecase.EstablishSession{
		Puncher:      &nat.UDPPuncher{Conn: conn, SessionID: sessionID},
		Relay:        client,
		PunchTimeout: 500 * time.Millisecond,
	}

	session, stream, err := establish.Execute(ctx, sessionID, deadCandidates)
	if stream == nil {
		// Either P2P succeeded (control connection no longer needed) or
		// everything failed — either way there's no stream tied to client
		// for the caller to keep using.
		client.Close()
		return session, nil, err
	}

	return session, relayStreamWithConn{ReadWriteCloser: stream, conn: client}, err
}

// relayStreamWithConn ties the relay stream's lifetime to its control
// connection: the stream is only usable as long as the connection is open,
// so Close tears down both together.
type relayStreamWithConn struct {
	io.ReadWriteCloser
	conn *controlclient.Client
}

func (r relayStreamWithConn) Close() error {
	err := r.ReadWriteCloser.Close()
	_ = r.conn.Close()
	return err
}

// reserveDeadUDPPort returns a loopback UDP address that is guaranteed not
// to have anything listening on it: it binds, reads the assigned port, and
// closes immediately.
func reserveDeadUDPPort(t *testing.T) netip.AddrPort {
	t.Helper()

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	require.NoError(t, err)
	addr := conn.LocalAddr().(*net.UDPAddr).AddrPort() //nolint:forcetypeassert // net.ListenUDP always returns *net.UDPAddr
	require.NoError(t, conn.Close())
	return addr
}
