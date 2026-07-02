package wgmesh_test

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	wgconn "golang.zx2c4.com/wireguard/conn"

	"github.com/fu1se/spur/internal/adapter/wgmesh"
	"github.com/fu1se/spur/internal/domain"
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
	t.Cleanup(func() { bindA.Close() })

	bindB := wgmesh.NewBind()
	require.NoError(t, bindB.AddPeer("peer-a", serverConn, false))
	t.Cleanup(func() { bindB.Close() })

	fnsB, _, err := bindB.Open(0)
	require.NoError(t, err)
	require.Len(t, fnsB, 1)

	// net.Pipe() is fully synchronous (unlike the real QUIC/yamux streams
	// this stands in for, which buffer): AddPeer's priming write for the
	// dialer blocks until something reads it, so it must run concurrently
	// with the listener side actually being ready to read — same as in
	// real usage, where each side's connectOne runs in its own goroutine
	// (or process) independently.
	addPeerErrCh := make(chan error, 1)
	go func() { addPeerErrCh <- bindA.AddPeer(peerID, clientConn, true) }()
	require.NoError(t, <-addPeerErrCh) // must finish (incl. registering the stream) before Send below

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

// TestBind_SurvivesCloseOpenCycle is a regression test for a real bug
// found during live TUN testing: wireguard-go calls Close then Open on
// its Bind every time listen_port is set (device/uapi.go's BindUpdate
// fires unconditionally), including once during the very first
// configuration — before any peer was even added. The original
// implementation closed one permanent gating channel in Close and never
// recreated it, so every peer went silently unreachable the moment the
// WireGuard interface came up. A Bind must keep working across repeated
// Close/Open cycles, the same way a real UDP-backed Bind's socket
// survives a rebind.
func TestBind_SurvivesCloseOpenCycle(t *testing.T) {
	const peerID = domain.PeerID("peer-b")

	clientConn, serverConn := net.Pipe()

	bindA := wgmesh.NewBind()
	t.Cleanup(func() { bindA.Close() })

	bindB := wgmesh.NewBind()
	require.NoError(t, bindB.AddPeer("peer-a", serverConn, false))
	t.Cleanup(func() { bindB.Close() })

	// See TestBind_SendReceiveRoundTrip's comment: the dialer's priming
	// write blocks on net.Pipe() until the listener side is reading, so
	// it must run concurrently with (not before) bindB's AddPeer above.
	addPeerErrCh := make(chan error, 1)
	go func() { addPeerErrCh <- bindA.AddPeer(peerID, clientConn, true) }()
	require.NoError(t, <-addPeerErrCh)

	// Simulate the exact sequence wireguard-go runs on every
	// listen_port= update, including the first one at device startup:
	// Open, then Close, then Open again — all before any real traffic.
	_, _, err := bindB.Open(0)
	require.NoError(t, err)
	require.NoError(t, bindB.Close())
	fnsB, _, err := bindB.Open(0)
	require.NoError(t, err)
	require.Len(t, fnsB, 1)

	payload := []byte("still alive after rebind")
	errCh := make(chan error, 1)
	go func() {
		errCh <- bindA.Send([][]byte{payload}, wgmesh.Endpoint{Peer: peerID})
	}()

	packets := [][]byte{make([]byte, 2000)}
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
		t.Fatal("timed out waiting for received packet after Close/Open cycle")
	}

	require.NoError(t, recvErr)
	require.Equal(t, 1, n)
	require.Equal(t, payload, packets[0][:sizes[0]])
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

func TestBind_HasPeer(t *testing.T) {
	const peerID = domain.PeerID("peer-a")
	_, stream := net.Pipe()

	bind := wgmesh.NewBind()
	t.Cleanup(func() { bind.Close() })

	require.False(t, bind.HasPeer(peerID))
	require.NoError(t, bind.AddPeer(peerID, stream, false))
	require.True(t, bind.HasPeer(peerID))
}

func TestBind_RemovePeer(t *testing.T) {
	const peerID = domain.PeerID("peer-a")
	_, stream := net.Pipe()

	bind := wgmesh.NewBind()
	t.Cleanup(func() { bind.Close() })

	require.NoError(t, bind.AddPeer(peerID, stream, false))
	require.True(t, bind.HasPeer(peerID))

	bind.RemovePeer(peerID)
	require.False(t, bind.HasPeer(peerID))
}

// TestBind_SelfHealsWhenStreamDies guards against the gap found in a
// security/reliability audit: a peer's stream dying used to leave it
// wedged in Bind's map forever with no way for a caller to notice and
// retry, since nothing ever removed a dead entry. HasPeer must start
// reporting false once the underlying stream is gone, without anyone
// having to call RemovePeer explicitly.
func TestBind_SelfHealsWhenStreamDies(t *testing.T) {
	const peerID = domain.PeerID("peer-a")
	other, stream := net.Pipe()

	bind := wgmesh.NewBind()
	t.Cleanup(func() { bind.Close() })

	require.NoError(t, bind.AddPeer(peerID, stream, false))
	require.True(t, bind.HasPeer(peerID))

	require.NoError(t, other.Close()) // the "remote" end going away kills readLoop's next Read

	require.Eventually(t, func() bool {
		return !bind.HasPeer(peerID)
	}, 2*time.Second, 10*time.Millisecond, "Bind should forget a peer once its stream dies")
}

// TestBind_AddPeerReplacesAndClosesOldStream guards against the goroutine/
// map-overwrite trap found in a security audit: a second AddPeer call for
// a peer ID already registered used to silently overwrite the map entry
// while the first stream's readLoop kept running underneath, leaking it
// forever (nothing ever closed the superseded stream).
func TestBind_AddPeerReplacesAndClosesOldStream(t *testing.T) {
	const peerID = domain.PeerID("peer-a")
	_, oldStream := net.Pipe()
	_, newStream := net.Pipe()

	bind := wgmesh.NewBind()
	t.Cleanup(func() { bind.Close() })

	require.NoError(t, bind.AddPeer(peerID, oldStream, false))
	require.NoError(t, bind.AddPeer(peerID, newStream, false))

	buf := make([]byte, 1)
	_, err := oldStream.Read(buf)
	require.Error(t, err, "the superseded stream should have been closed")
}

// TestBind_OnePeerFullChannelDoesNotBlockAnother guards against the
// head-of-line blocking gap found in a security audit: all peers used to
// share a single inbound channel, so one bursty (or malicious) peer
// filling it would backpressure every other peer's readLoop too, since
// they'd all block trying to write into the very same full channel. Each
// peer now gets its own dedicated buffer, so filling peer A's completely
// must not stop peer B's delivery.
func TestBind_OnePeerFullChannelDoesNotBlockAnother(t *testing.T) {
	const peerA = domain.PeerID("peer-a")
	const peerB = domain.PeerID("peer-b")
	// Mirrors the unexported peerChanBuffer constant in bind.go.
	const peerChanBuffer = 128

	otherA, streamA := net.Pipe()
	otherB, streamB := net.Pipe()
	t.Cleanup(func() { otherA.Close(); otherB.Close() })

	bind := wgmesh.NewBind()
	t.Cleanup(func() { bind.Close() })

	require.NoError(t, bind.AddPeer(peerA, streamA, false))
	require.NoError(t, bind.AddPeer(peerB, streamB, false))

	// Fill peer A's channel to capacity. Nothing is draining it (Open/
	// receive is never called in this test), but each of these writes
	// still completes synchronously: readLoop reads the frame off the
	// wire and buffers it in its own channel, which has room for exactly
	// this many.
	for range peerChanBuffer {
		writeFramedPacket(t, otherA, []byte("a"))
	}

	// Peer B's channel is untouched by peer A's state: this write must
	// complete promptly regardless.
	writeDone := make(chan struct{})
	go func() {
		writeFramedPacket(t, otherB, []byte("b"))
		close(writeDone)
	}()

	select {
	case <-writeDone:
	case <-time.After(2 * time.Second):
		t.Fatal("peer B's delivery was blocked by peer A's full channel")
	}
}

func writeFramedPacket(t *testing.T, w io.Writer, payload []byte) {
	t.Helper()
	var lenBuf [2]byte
	binary.BigEndian.PutUint16(lenBuf[:], uint16(len(payload)))
	_, err := w.Write(lenBuf[:])
	require.NoError(t, err)
	_, err = w.Write(payload)
	require.NoError(t, err)
}
