package wgmesh_test

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	wgconn "golang.zx2c4.com/wireguard/conn"

	"github.com/fu1se/localizator/internal/adapter/wgmesh"
	"github.com/fu1se/localizator/internal/domain"
)

// TestBind_SendReceiveRoundTrip verifies the framing/dispatch logic in
// isolation from any real network or TUN device: two Binds are wired
// together with an in-memory net.Pipe() standing in for an
// EstablishSession-established Stream, and a packet sent through one
// Bind's Send (addressed by Endpoint/peer ID) arrives at the other's
// receive function attributed to the right peer.
func TestBind_SendReceiveRoundTrip(t *testing.T) {
	const peerID = domain.PeerID("peer-b")

	clientConn, serverConn := net.Pipe()

	bindA := wgmesh.NewBind()
	bindA.AddPeer(peerID, clientConn)
	t.Cleanup(func() { bindA.Close() })

	bindB := wgmesh.NewBind()
	bindB.AddPeer("peer-a", serverConn)
	t.Cleanup(func() { bindB.Close() })

	fnsB, _, err := bindB.Open(0)
	require.NoError(t, err)
	require.Len(t, fnsB, 1)

	payload := []byte("wireguard packet payload")
	errCh := make(chan error, 1)
	go func() {
		errCh <- bindA.Send([][]byte{payload}, wgmesh.Endpoint{Peer: peerID})
	}()

	packets := make([][]byte, 1)
	packets[0] = make([]byte, 2000)
	sizes := make([]int, 1)
	eps := make([]wgconn.Endpoint, 1)

	done := make(chan struct{})
	var n int
	var recvErr error
	go func() {
		n, recvErr = fnsB[0](packets, sizes, eps)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for received packet")
	}

	require.NoError(t, recvErr)
	require.Equal(t, 1, n)
	require.Equal(t, payload, packets[0][:sizes[0]])

	ep, ok := eps[0].(wgmesh.Endpoint)
	require.True(t, ok)
	require.Equal(t, domain.PeerID("peer-a"), ep.Peer)

	require.NoError(t, <-errCh)
}

func TestBind_SendToUnknownPeer(t *testing.T) {
	bind := wgmesh.NewBind()
	t.Cleanup(func() { bind.Close() })

	err := bind.Send([][]byte{[]byte("x")}, wgmesh.Endpoint{Peer: "nobody"})
	require.Error(t, err)
}

func TestBind_ParseEndpoint(t *testing.T) {
	bind := wgmesh.NewBind()
	t.Cleanup(func() { bind.Close() })

	ep, err := bind.ParseEndpoint("some-peer-id")
	require.NoError(t, err)
	require.Equal(t, wgmesh.Endpoint{Peer: "some-peer-id"}, ep)
}
