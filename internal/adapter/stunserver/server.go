// Package stunserver is a minimal RFC 5389 STUN Binding responder. It runs
// on its own UDP port, separate from the QUIC control-plane port: QUIC's
// framing can't be safely demultiplexed from raw STUN packets on the same
// socket without extra work, so CLAUDE.md accepts two ports as a
// deliberate simplification (see "Требования окружения для сборки").
package stunserver

import (
	"context"
	"net"
	"net/netip"

	"github.com/pion/stun/v3"
)

// Serve runs the STUN Binding responder on conn until ctx is cancelled. It
// blocks, and it takes ownership of conn: conn is closed both when ctx is
// done and when Serve returns. Callers bind conn themselves (typically via
// net.ListenUDP) so they know the actual bound address before Serve is
// called — important for tests using ephemeral ports.
func Serve(ctx context.Context, conn *net.UDPConn) error {
	defer conn.Close()

	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	buf := make([]byte, 1500)
	for {
		n, from, err := conn.ReadFromUDPAddrPort(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			continue
		}
		respondBinding(conn, from, buf[:n])
	}
}

func respondBinding(conn *net.UDPConn, from netip.AddrPort, raw []byte) {
	req := &stun.Message{Raw: append([]byte(nil), raw...)}
	if err := req.Decode(); err != nil || req.Type != stun.BindingRequest {
		return
	}

	res, err := stun.Build(
		stun.NewTransactionIDSetter(req.TransactionID),
		stun.BindingSuccess,
		&stun.XORMappedAddress{IP: net.IP(from.Addr().AsSlice()), Port: int(from.Port())},
		stun.Fingerprint,
	)
	if err != nil {
		return
	}

	_, _ = conn.WriteToUDPAddrPort(res.Raw, from)
}
