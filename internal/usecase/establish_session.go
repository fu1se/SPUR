package usecase

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/fu1se/spur/internal/domain"
	"github.com/fu1se/spur/internal/usecase/port"
)

// defaultPunchTimeout bounds how long EstablishSession waits for hole
// punching before giving up and falling back to relay.
const defaultPunchTimeout = 5 * time.Second

// EstablishSession is the client-side use case that tries direct P2P via
// hole punching first and automatically falls back to the server relay if
// punching doesn't succeed within PunchTimeout. Either path produces a
// domain.Session recording which one was used.
//
// The P2P and relay outcomes are represented differently on purpose: P2P
// only resolves an address (candidates.ResolvedAddr) — turning that into
// an actual data-plane connection is a later phase's concern (the
// transport differs: multiplexed QUIC streams for port-forward, raw
// WireGuard UDP datagrams for mesh). Relay, by contrast, already produces
// a ready-to-use duplex byte stream, since there's no NAT-visible address
// to dial in relay mode — the server itself is the rendezvous point.
type EstablishSession struct {
	Puncher      port.Puncher
	Relay        port.Relay
	PunchTimeout time.Duration
}

// Execute returns the resulting Session plus, only when it fell back to
// relay, the spliced byte stream to use as the data plane. relayStream is
// nil when State == SessionEstablishedP2P.
func (uc EstablishSession) Execute(ctx context.Context, sessionID string, candidates []domain.Candidate) (session domain.Session, relayStream io.ReadWriteCloser, err error) {
	now := time.Now()
	session = domain.Session{
		ID:        sessionID,
		State:     domain.SessionPunching,
		CreatedAt: now,
		UpdatedAt: now,
	}

	timeout := uc.PunchTimeout
	if timeout == 0 {
		timeout = defaultPunchTimeout
	}

	punchCtx, cancel := context.WithTimeout(ctx, timeout)
	addr, punchErr := uc.Puncher.Punch(punchCtx, candidates)
	cancel()

	if punchErr == nil {
		session.State = domain.SessionEstablishedP2P
		session.ResolvedAddr = addr
		session.UpdatedAt = time.Now()
		return session, nil, nil
	}

	stream, relayErr := uc.Relay.OpenChannel(ctx, sessionID)
	if relayErr != nil {
		session.State = domain.SessionFailed
		session.UpdatedAt = time.Now()
		return session, nil, fmt.Errorf("usecase: punch failed (%w) and relay fallback failed: %w", punchErr, relayErr)
	}

	session.State = domain.SessionEstablishedRelay
	session.UpdatedAt = time.Now()
	return session, stream, nil
}
