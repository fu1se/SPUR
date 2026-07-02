package e2e_test

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/binary"
	"io"
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/spur/internal/adapter/e2e"
	"github.com/fu1se/spur/internal/domain"
	"github.com/fu1se/spur/internal/usecase/port"
)

// fakeConn is a minimal port.TunnelConn backed by a single net.Conn (from
// net.Pipe), enough to exercise WrapConn without any real transport.
type fakeConn struct {
	net.Conn
}

func (c fakeConn) OpenStream(context.Context) (port.Stream, error)   { return c.Conn, nil }
func (c fakeConn) AcceptStream(context.Context) (port.Stream, error) { return c.Conn, nil }

// multiStreamFakeConn is a port.TunnelConn backed by several independent
// net.Pipe()s, one per OpenStream/AcceptStream call — simulating several
// real, independent streams multiplexed over one connection (as QUIC/yamux
// streams are in production), unlike fakeConn's single shared net.Conn.
type multiStreamFakeConn struct {
	mu   sync.Mutex
	ends []net.Conn
	next int
}

func (c *multiStreamFakeConn) nextEnd() net.Conn {
	c.mu.Lock()
	defer c.mu.Unlock()
	conn := c.ends[c.next]
	c.next++
	return conn
}

func (c *multiStreamFakeConn) OpenStream(context.Context) (port.Stream, error) {
	return c.nextEnd(), nil
}
func (c *multiStreamFakeConn) AcceptStream(context.Context) (port.Stream, error) {
	return c.nextEnd(), nil
}
func (c *multiStreamFakeConn) Close() error { return nil }

// recordingConn wraps a net.Conn and copies every Write into buf, so a test
// can inspect the raw ciphertext frames that crossed the wire.
type recordingConn struct {
	net.Conn
	mu  *sync.Mutex
	buf *[]byte
}

func (c recordingConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	*c.buf = append(*c.buf, p...)
	c.mu.Unlock()
	return c.Conn.Write(p)
}

func genIdentity(t *testing.T) (*ecdh.PrivateKey, domain.PublicKey) {
	t.Helper()
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	require.NoError(t, err)
	var pub domain.PublicKey
	copy(pub[:], priv.PublicKey().Bytes())
	return priv, pub
}

func genSalt(t *testing.T) [32]byte {
	t.Helper()
	var salt [32]byte
	_, err := rand.Read(salt[:])
	require.NoError(t, err)
	return salt
}

func TestWrapConn_RoundTrip(t *testing.T) {
	dialerConn, listenerConn := net.Pipe()

	dialerPriv, dialerPub := genIdentity(t)
	listenerPriv, listenerPub := genIdentity(t)
	salt := genSalt(t)

	dialerTunnel, err := e2e.WrapConn(fakeConn{dialerConn}, dialerPriv, listenerPub, salt, true)
	require.NoError(t, err)
	listenerTunnel, err := e2e.WrapConn(fakeConn{listenerConn}, listenerPriv, dialerPub, salt, false)
	require.NoError(t, err)

	dialerStream, err := dialerTunnel.OpenStream(context.Background())
	require.NoError(t, err)
	listenerStream, err := listenerTunnel.AcceptStream(context.Background())
	require.NoError(t, err)

	const msgA = "hello from dialer"
	const msgB = "hello from listener"

	// net.Pipe() is fully synchronous: a Write blocks until a matching
	// Read happens on the other end. Reads and writes for both directions
	// must therefore all run concurrently (as they would in the real
	// pipe() usecase's io.Copy loops) — waiting for both writes to finish
	// before starting any read would deadlock, since nothing would ever
	// read the pending data.
	type result struct {
		buf []byte
		err error
	}
	recvA := make(chan result, 1)
	recvB := make(chan result, 1)

	go func() {
		buf := make([]byte, len(msgB))
		_, err := io.ReadFull(dialerStream, buf)
		recvA <- result{buf, err}
	}()
	go func() {
		buf := make([]byte, len(msgA))
		_, err := io.ReadFull(listenerStream, buf)
		recvB <- result{buf, err}
	}()

	_, err = dialerStream.Write([]byte(msgA))
	require.NoError(t, err)
	_, err = listenerStream.Write([]byte(msgB))
	require.NoError(t, err)

	rA := <-recvA
	require.NoError(t, rA.err)
	require.Equal(t, msgB, string(rA.buf))

	rB := <-recvB
	require.NoError(t, rB.err)
	require.Equal(t, msgA, string(rB.buf))
}

func TestWrapConn_WrongKeyFailsToDecrypt(t *testing.T) {
	dialerConn, listenerConn := net.Pipe()

	dialerPriv, _ := genIdentity(t)
	listenerPriv, listenerPub := genIdentity(t)
	_, attackerPub := genIdentity(t) // listener will (wrongly) expect this key
	salt := genSalt(t)

	dialerTunnel, err := e2e.WrapConn(fakeConn{dialerConn}, dialerPriv, listenerPub, salt, true)
	require.NoError(t, err)
	listenerTunnel, err := e2e.WrapConn(fakeConn{listenerConn}, listenerPriv, attackerPub, salt, false)
	require.NoError(t, err)

	dialerStream, err := dialerTunnel.OpenStream(context.Background())
	require.NoError(t, err)
	listenerStream, err := listenerTunnel.AcceptStream(context.Background())
	require.NoError(t, err)

	go func() { _, _ = dialerStream.Write([]byte("hello")) }()

	buf := make([]byte, 5)
	_, err = io.ReadFull(listenerStream, buf)
	require.Error(t, err)
}

func TestWrapConn_DifferentSaltFailsToDecrypt(t *testing.T) {
	dialerConn, listenerConn := net.Pipe()

	dialerPriv, dialerPub := genIdentity(t)
	listenerPriv, listenerPub := genIdentity(t)

	dialerTunnel, err := e2e.WrapConn(fakeConn{dialerConn}, dialerPriv, listenerPub, genSalt(t), true)
	require.NoError(t, err)
	// Listener derives with a different (not combined the same way) salt,
	// simulating the two sides disagreeing on CombineSalt's input.
	listenerTunnel, err := e2e.WrapConn(fakeConn{listenerConn}, listenerPriv, dialerPub, genSalt(t), false)
	require.NoError(t, err)

	dialerStream, err := dialerTunnel.OpenStream(context.Background())
	require.NoError(t, err)
	listenerStream, err := listenerTunnel.AcceptStream(context.Background())
	require.NoError(t, err)

	go func() { _, _ = dialerStream.Write([]byte("hello")) }()

	buf := make([]byte, 5)
	_, err = io.ReadFull(listenerStream, buf)
	require.Error(t, err)
}

// TestWrapConn_MultipleStreamsDontReuseNonce guards against the class of
// bug fixed alongside per-session salting: if each Stream's AEAD nonce
// counter started fresh at 0 instead of sharing a per-wrappedConn counter,
// two streams opened on the same connection (e.g. two forwarded TCP
// connections through one `spur connect`) would encrypt their first frame
// under the identical (key, nonce) pair. AES-GCM is deterministic given
// (key, nonce, plaintext), so identical plaintext through two streams
// would then produce byte-identical ciphertext — the observable symptom
// of a nonce-reuse bug, and exactly what this test checks does NOT
// happen.
func TestWrapConn_MultipleStreamsDontReuseNonce(t *testing.T) {
	dialerConnA, listenerConnA := net.Pipe()
	dialerConnB, listenerConnB := net.Pipe()

	var mu sync.Mutex
	var rawA, rawB []byte
	recA := recordingConn{Conn: dialerConnA, mu: &mu, buf: &rawA}
	recB := recordingConn{Conn: dialerConnB, mu: &mu, buf: &rawB}

	dialerPriv, dialerPub := genIdentity(t)
	listenerPriv, listenerPub := genIdentity(t)
	salt := genSalt(t)

	dialerFake := &multiStreamFakeConn{ends: []net.Conn{recA, recB}}
	listenerFake := &multiStreamFakeConn{ends: []net.Conn{listenerConnA, listenerConnB}}

	dialerTunnel, err := e2e.WrapConn(dialerFake, dialerPriv, listenerPub, salt, true)
	require.NoError(t, err)
	listenerTunnel, err := e2e.WrapConn(listenerFake, listenerPriv, dialerPub, salt, false)
	require.NoError(t, err)

	streamA, err := dialerTunnel.OpenStream(context.Background())
	require.NoError(t, err)
	streamB, err := dialerTunnel.OpenStream(context.Background())
	require.NoError(t, err)
	listenerA, err := listenerTunnel.AcceptStream(context.Background())
	require.NoError(t, err)
	listenerB, err := listenerTunnel.AcceptStream(context.Background())
	require.NoError(t, err)

	const msg = "identical-plaintext-on-both-streams"

	type result struct {
		buf []byte
		err error
	}
	recvA := make(chan result, 1)
	recvB := make(chan result, 1)
	go func() {
		buf := make([]byte, len(msg))
		_, err := io.ReadFull(listenerA, buf)
		recvA <- result{buf, err}
	}()
	go func() {
		buf := make([]byte, len(msg))
		_, err := io.ReadFull(listenerB, buf)
		recvB <- result{buf, err}
	}()

	_, err = streamA.Write([]byte(msg))
	require.NoError(t, err)
	_, err = streamB.Write([]byte(msg))
	require.NoError(t, err)

	rA := <-recvA
	require.NoError(t, rA.err)
	require.Equal(t, msg, string(rA.buf))
	rB := <-recvB
	require.NoError(t, rB.err)
	require.Equal(t, msg, string(rB.buf))

	mu.Lock()
	defer mu.Unlock()
	require.GreaterOrEqual(t, len(rawA), 4)
	require.GreaterOrEqual(t, len(rawB), 4)
	lenA := binary.BigEndian.Uint32(rawA[:4])
	lenB := binary.BigEndian.Uint32(rawB[:4])
	ciphertextA := rawA[4 : 4+lenA]
	ciphertextB := rawB[4 : 4+lenB]

	require.False(t, bytes.Equal(ciphertextA, ciphertextB),
		"identical plaintext on two streams of one connection produced identical ciphertext -- nonce was reused")
}
