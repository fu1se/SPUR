package nat_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/pion/stun/v3"
	"github.com/stretchr/testify/require"

	"github.com/fu1se/spur/internal/adapter/nat"
	"github.com/fu1se/spur/internal/adapter/stunserver"
)

// TestDiscoverServerReflexive_IgnoresForgedResponse guards against the gap
// found in a security audit: DiscoverServerReflexive used to accept a STUN
// response from any source and never checked the transaction ID, so
// anyone able to race a packet onto the client's socket before the real
// STUN server replied could poison the client's belief about its own
// public ip:port. A forged response from an unrelated address, using an
// unrelated transaction ID, must be ignored in favor of the real server's
// (slightly delayed) reply.
func TestDiscoverServerReflexive_IgnoresForgedResponse(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	serverConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	require.NoError(t, err)
	serverAddr := serverConn.LocalAddr().(*net.UDPAddr).AddrPort() //nolint:forcetypeassert
	go func() { _ = stunserver.Serve(ctx, serverConn) }()

	attackerConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	require.NoError(t, err)
	defer attackerConn.Close()

	clientConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	require.NoError(t, err)
	defer clientConn.Close()

	// Forged response: a fabricated transaction ID (the client hasn't
	// even sent its request yet, so there's no way the attacker could
	// know the real one) and a bogus mapped address, sent from an address
	// that isn't the STUN server the client asked.
	forged, err := stun.Build(stun.TransactionID, stun.BindingSuccess,
		&stun.XORMappedAddress{IP: net.ParseIP("203.0.113.66"), Port: 6666},
		stun.Fingerprint,
	)
	require.NoError(t, err)
	_, err = attackerConn.WriteToUDPAddrPort(forged.Raw, clientConn.LocalAddr().(*net.UDPAddr).AddrPort()) //nolint:forcetypeassert
	require.NoError(t, err)

	candidate, err := nat.DiscoverServerReflexive(ctx, clientConn, serverAddr)
	require.NoError(t, err)

	require.Equal(t, "127.0.0.1", candidate.Addr.Addr().String())
	require.NotEqual(t, uint16(6666), candidate.Addr.Port())
}
