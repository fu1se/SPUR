package e2e_test

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"io"
	"net"
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

func genIdentity(t *testing.T) (*ecdh.PrivateKey, domain.PublicKey) {
	t.Helper()
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	require.NoError(t, err)
	var pub domain.PublicKey
	copy(pub[:], priv.PublicKey().Bytes())
	return priv, pub
}

func TestWrapConn_RoundTrip(t *testing.T) {
	dialerConn, listenerConn := net.Pipe()

	dialerPriv, dialerPub := genIdentity(t)
	listenerPriv, listenerPub := genIdentity(t)

	dialerTunnel, err := e2e.WrapConn(fakeConn{dialerConn}, dialerPriv, listenerPub, true)
	require.NoError(t, err)
	listenerTunnel, err := e2e.WrapConn(fakeConn{listenerConn}, listenerPriv, dialerPub, false)
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

	dialerTunnel, err := e2e.WrapConn(fakeConn{dialerConn}, dialerPriv, listenerPub, true)
	require.NoError(t, err)
	listenerTunnel, err := e2e.WrapConn(fakeConn{listenerConn}, listenerPriv, attackerPub, false)
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
