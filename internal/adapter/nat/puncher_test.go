package nat_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/spur/internal/adapter/nat"
	"github.com/fu1se/spur/internal/domain"
)

// punchMagic mirrors the unexported constant in puncher.go: it's not a
// secret (any spur binary embeds it), so an attacker who knows a session ID
// (itself a public, deterministic function of two known peer IDs) can
// trivially construct a forged punch payload. This test builds one the
// same way to prove the puncher only accepts it from an address it
// actually offered as a candidate.
const punchMagic = "spur-punch1:"

// TestPunch_IgnoresResponseFromUnlistedAddress guards against the
// candidate-spoofing gap found in a security audit: recvLoop used to
// accept a punch response from any UDP source that echoed the right
// magic+session-id payload, regardless of whether that source was ever
// offered as a candidate. A forged response injected from an address the
// caller never listed must not resolve the punch.
func TestPunch_IgnoresResponseFromUnlistedAddress(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	puncherConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	require.NoError(t, err)
	defer puncherConn.Close()

	legitConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	require.NoError(t, err)
	defer legitConn.Close()

	attackerConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	require.NoError(t, err)
	defer attackerConn.Close()

	const sessionID = "test-session"
	payload := []byte(punchMagic + sessionID)

	legitAddr := legitConn.LocalAddr().(*net.UDPAddr).AddrPort()     //nolint:forcetypeassert
	puncherAddr := puncherConn.LocalAddr().(*net.UDPAddr).AddrPort() //nolint:forcetypeassert

	puncher := &nat.UDPPuncher{Conn: puncherConn, SessionID: sessionID}
	candidates := []domain.Candidate{{Kind: domain.CandidateHost, Addr: legitAddr}}

	resultCh := make(chan struct {
		addr string
		err  error
	}, 1)
	go func() {
		addr, err := puncher.Punch(ctx, candidates)
		resultCh <- struct {
			addr string
			err  error
		}{addr.String(), err}
	}()

	// The attacker, whose address was never offered as a candidate, races
	// a forged response in first.
	_, err = attackerConn.WriteToUDPAddrPort(payload, puncherAddr)
	require.NoError(t, err)

	// Give the forged packet a moment to be (wrongly, if the bug were
	// present) accepted before the legitimate candidate ever responds.
	time.Sleep(300 * time.Millisecond)

	// Now the real candidate replies, as the puncher's own sendLoop would
	// have prompted it to in a real handshake.
	_, err = legitConn.WriteToUDPAddrPort(payload, puncherAddr)
	require.NoError(t, err)

	res := <-resultCh
	require.NoError(t, res.err)
	require.Equal(t, legitAddr.String(), res.addr)
}
