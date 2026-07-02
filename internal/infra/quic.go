package infra

import (
	"time"

	"github.com/quic-go/quic-go"
)

// DefaultQUICConfig is shared by every QUIC listener/dialer in the project
// (control-plane and data-plane alike).
//
// Without an explicit KeepAlivePeriod, a QUIC connection that sees no
// traffic for MaxIdleTimeout (30s by default) is torn down by the
// transport itself — independent of any application-level timeout. That
// bit hard during manual testing: establishing a session (STUN, candidate
// exchange, punching) legitimately involves stretches with no packets on
// a given connection, especially the control connection while
// AwaitCandidates blocks waiting on the counterpart. KeepAlivePeriod
// packets also happen to serve a second purpose on the data-plane
// connection: they keep the just-punched NAT mapping from expiring during
// idle periods between bursts of tunnel traffic.
func DefaultQUICConfig() *quic.Config {
	return &quic.Config{
		// Default is 5s, tight enough to occasionally misfire under real
		// load: mesh mode (Phase 6) opens several concurrent QUIC dials
		// (the network-join connection plus one rendezvous connection per
		// peer) competing for CPU, and this got hit during live testing
		// with two `app join` processes running at once.
		HandshakeIdleTimeout: 30 * time.Second,
		MaxIdleTimeout:       5 * time.Minute,
		KeepAlivePeriod:      15 * time.Second,
	}
}
