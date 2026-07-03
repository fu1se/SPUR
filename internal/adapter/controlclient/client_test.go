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

func TestRegister_EndToEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	serverTLS, err := infra.SelfSignedServerTLSConfig(controlproto.ALPN)
	require.NoError(t, err)

	peers := memory.NewPeerRepository()
	srv := &controlserver.Server{RegisterPeer: usecase.RegisterPeer{Peers: peers}, Version: "v1.2.3"}

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := conn.LocalAddr().String()

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ctx, conn, serverTLS, infra.DefaultQUICConfig()) }()

	var pub domain.PublicKey
	_, err = rand.Read(pub[:])
	require.NoError(t, err)

	client, err := controlclient.Dial(ctx, addr, infra.InsecureClientTLSConfig(controlproto.ALPN), infra.DefaultQUICConfig())
	require.NoError(t, err)
	defer client.Close()

	result, err := client.Register(ctx, pub, "v9.9.9")
	require.NoError(t, err)

	require.Equal(t, domain.DerivePeerID(pub), result.PeerID)
	require.Contains(t, result.ObservedAddress, "127.0.0.1:")
	require.Equal(t, "v1.2.3", result.ServerVersion)

	stored, err := peers.FindByID(ctx, result.PeerID)
	require.NoError(t, err)
	require.Equal(t, pub, stored.PublicKey)
	require.Len(t, stored.Candidates, 1)
	require.Equal(t, domain.CandidateServerReflexive, stored.Candidates[0].Kind)

	cancel()
	err = <-serveErr
	require.NoError(t, err)
}
