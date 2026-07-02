package controlclient_test

import (
	"context"
	"crypto/rand"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/spur/internal/adapter/controlclient"
	"github.com/fu1se/spur/internal/adapter/controlproto"
	"github.com/fu1se/spur/internal/adapter/controlserver"
	"github.com/fu1se/spur/internal/adapter/repository/memory"
	"github.com/fu1se/spur/internal/domain"
	"github.com/fu1se/spur/internal/infra"
	"github.com/fu1se/spur/internal/usecase"
)

func randomPublicKey(t *testing.T) domain.PublicKey {
	t.Helper()

	var pub domain.PublicKey
	_, err := rand.Read(pub[:])
	require.NoError(t, err)
	return pub
}

// TestJoinNetwork_TwoPeersSeeEachOther verifies the Phase 6 server-side
// coordination: the network is auto-created on first join, each peer gets
// a distinct mesh IP, and by the time the second peer joins it can already
// see the first (and vice versa, once it re-joins/refreshes) — the
// membership list returned is always the network's full current state.
func TestJoinNetwork_TwoPeersSeeEachOther(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tlsConf, err := infra.SelfSignedServerTLSConfig(controlproto.ALPN)
	require.NoError(t, err)

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := conn.LocalAddr().String()

	networks := memory.NewNetworkRepository()
	srv := &controlserver.Server{JoinNetwork: usecase.JoinNetwork{Networks: networks}}
	go func() { _ = srv.Serve(ctx, conn, tlsConf, infra.DefaultQUICConfig()) }()

	clientA, err := controlclient.Dial(ctx, addr, infra.InsecureClientTLSConfig(controlproto.ALPN), infra.DefaultQUICConfig())
	require.NoError(t, err)
	defer clientA.Close()

	clientB, err := controlclient.Dial(ctx, addr, infra.InsecureClientTLSConfig(controlproto.ALPN), infra.DefaultQUICConfig())
	require.NoError(t, err)
	defer clientB.Close()

	pubA := randomPublicKey(t)
	pubB := randomPublicKey(t)

	networkA, err := clientA.JoinNetwork(ctx, "home", "", pubA)
	require.NoError(t, err)
	require.Len(t, networkA.Members, 1)
	require.Equal(t, domain.DerivePeerID(pubA), networkA.Members[0].PeerID)
	require.NotEmpty(t, networkA.InviteToken, "creating a network must hand back its invite token")

	networkB, err := clientB.JoinNetwork(ctx, "home", networkA.InviteToken, pubB)
	require.NoError(t, err)
	require.Len(t, networkB.Members, 2)

	// Both peers appear, with distinct mesh IPs, and B's view includes A.
	var sawA, sawB bool
	for _, m := range networkB.Members {
		switch m.PeerID {
		case domain.DerivePeerID(pubA):
			sawA = true
		case domain.DerivePeerID(pubB):
			sawB = true
		}
	}
	require.True(t, sawA)
	require.True(t, sawB)
	require.NotEqual(t, networkB.Members[0].MeshIP, networkB.Members[1].MeshIP)

	// Re-joining is idempotent: same membership, no duplicate entry, and
	// no token needed since A is already a member.
	networkA2, err := clientA.JoinNetwork(ctx, "home", "", pubA)
	require.NoError(t, err)
	require.Len(t, networkA2.Members, 2)
}

// TestJoinNetwork_WrongInviteTokenRejected verifies the control-plane
// wire protocol surfaces a clear error (JoinNetworkResponse.error) rather
// than just dropping the connection when a join is rejected.
func TestJoinNetwork_WrongInviteTokenRejected(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tlsConf, err := infra.SelfSignedServerTLSConfig(controlproto.ALPN)
	require.NoError(t, err)

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := conn.LocalAddr().String()

	networks := memory.NewNetworkRepository()
	srv := &controlserver.Server{JoinNetwork: usecase.JoinNetwork{Networks: networks}}
	go func() { _ = srv.Serve(ctx, conn, tlsConf, infra.DefaultQUICConfig()) }()

	clientA, err := controlclient.Dial(ctx, addr, infra.InsecureClientTLSConfig(controlproto.ALPN), infra.DefaultQUICConfig())
	require.NoError(t, err)
	defer clientA.Close()

	clientB, err := controlclient.Dial(ctx, addr, infra.InsecureClientTLSConfig(controlproto.ALPN), infra.DefaultQUICConfig())
	require.NoError(t, err)
	defer clientB.Close()

	_, err = clientA.JoinNetwork(ctx, "home", "", randomPublicKey(t))
	require.NoError(t, err)

	_, err = clientB.JoinNetwork(ctx, "home", "not-the-right-token", randomPublicKey(t))
	require.Error(t, err)
}
