package controlclient_test

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/localizator/internal/adapter/controlclient"
	"github.com/fu1se/localizator/internal/adapter/controlproto"
	"github.com/fu1se/localizator/internal/adapter/controlserver"
	"github.com/fu1se/localizator/internal/adapter/repository/memory"
	"github.com/fu1se/localizator/internal/domain"
	"github.com/fu1se/localizator/internal/infra"
	"github.com/fu1se/localizator/internal/usecase"
)

func TestRegister_EndToEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	serverTLS, err := infra.SelfSignedServerTLSConfig(controlproto.ALPN)
	require.NoError(t, err)

	peers := memory.NewPeerRepository()
	srv := &controlserver.Server{RegisterPeer: usecase.RegisterPeer{Peers: peers}}

	const addr = "127.0.0.1:48443"

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ctx, addr, serverTLS) }()

	// give the listener a moment to come up before dialing.
	time.Sleep(100 * time.Millisecond)

	var pub domain.PublicKey
	_, err = rand.Read(pub[:])
	require.NoError(t, err)

	client, err := controlclient.Dial(ctx, addr, infra.InsecureClientTLSConfig(controlproto.ALPN))
	require.NoError(t, err)
	defer client.Close()

	result, err := client.Register(ctx, pub)
	require.NoError(t, err)

	require.Equal(t, domain.DerivePeerID(pub), result.PeerID)
	require.Contains(t, result.ObservedAddress, "127.0.0.1:")

	stored, err := peers.FindByID(ctx, result.PeerID)
	require.NoError(t, err)
	require.Equal(t, pub, stored.PublicKey)
	require.Len(t, stored.Candidates, 1)
	require.Equal(t, domain.CandidateServerReflexive, stored.Candidates[0].Kind)

	cancel()
	err = <-serveErr
	require.NoError(t, err)
}
