package rendezvous

import (
	"context"
	"fmt"

	"github.com/fu1se/spur/internal/adapter/controlclient"
	"github.com/fu1se/spur/internal/domain"
	"github.com/fu1se/spur/internal/infra"
)

// CounterpartResolver learns the counterpart's peer ID once the
// control-plane connection is registered — either it's already known
// (see ResolveCounterpartArg, the "guest" side: a raw peer ID or a short
// pairing code the guest was handed) or it has to be learned by
// registering a fresh pairing code and waiting for someone to use it (see
// HostViaPairingCode, the "host" side) — both sides funnel through the
// exact same Establish logic downstream regardless of which one they are.
type CounterpartResolver func(ctx context.Context, client *controlclient.Client, id infra.Identity) (domain.PeerID, error)

// ResolveCounterpartArg treats raw as a full peer ID if it's already
// shaped like one (see LooksLikePeerID), or resolves it as a short
// pairing code against the server otherwise — the "guest" side of both
// the classic --to <peer-id> flow and the newer --to <code> flow.
func ResolveCounterpartArg(raw string) CounterpartResolver {
	return func(ctx context.Context, client *controlclient.Client, id infra.Identity) (domain.PeerID, error) {
		if LooksLikePeerID(raw) {
			return domain.PeerID(raw), nil
		}
		peer, err := client.ResolvePairingCode(ctx, raw, id.PublicKey)
		if err != nil {
			return "", fmt.Errorf("resolve pairing code %q: %w", raw, err)
		}
		return peer, nil
	}
}

// LooksLikePeerID reports whether s has the exact shape
// domain.DerivePeerID produces (32 lowercase hex characters — the first
// 16 bytes of a SHA-256 digest) as opposed to a short pairing code (drawn
// from a smaller, uppercase alphabet) — the two formats never overlap, so
// this is enough to tell them apart without a round trip to the server.
func LooksLikePeerID(s string) bool {
	if len(s) != 32 {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

// FixedCounterpart is a CounterpartResolver for callers that already have
// a real domain.PeerID in hand and need no resolution at all — e.g. mesh
// mode, where the counterpart comes from network membership, not user
// input, so there's nothing to parse or look up.
func FixedCounterpart(peer domain.PeerID) CounterpartResolver {
	return func(context.Context, *controlclient.Client, infra.Identity) (domain.PeerID, error) {
		return peer, nil
	}
}

// HostViaPairingCode is the "host" side of the single-command connect
// flow: register a fresh short code, hand it to onCode so the caller can
// surface it (e.g. print "Код для подключения: ABC123"), then block until
// some guest resolves it — see usecase.PairingCodeTTL for how long that
// can take before giving up.
func HostViaPairingCode(onCode OnCodeFunc) CounterpartResolver {
	return func(ctx context.Context, client *controlclient.Client, id infra.Identity) (domain.PeerID, error) {
		code, err := client.RegisterPairingCode(ctx, id.PublicKey)
		if err != nil {
			return "", fmt.Errorf("register pairing code: %w", err)
		}
		if onCode != nil {
			onCode(code)
		}
		guest, err := client.AwaitPairingCodeUse(ctx, code)
		if err != nil {
			return "", fmt.Errorf("await pairing code use: %w", err)
		}
		return guest, nil
	}
}

// RoomCounterpart is a CounterpartResolver for --room: the caller already
// set up a persistent, two-member room (see usecase.CreateRoom/JoinRoom),
// so the counterpart is whichever of the two members isn't the caller —
// resolved server-side, no code or peer ID to type in each time.
func RoomCounterpart(roomName string) CounterpartResolver {
	return func(ctx context.Context, client *controlclient.Client, id infra.Identity) (domain.PeerID, error) {
		peer, err := client.ResolveRoom(ctx, roomName, id.PublicKey)
		if err != nil {
			return "", fmt.Errorf("resolve room %q: %w", roomName, err)
		}
		return peer, nil
	}
}

// CounterpartResolverFor picks between three CounterpartResolver flavors:
// a non-empty room takes priority (see RoomCounterpart — callers already
// reject --to and --room being set together, so this ordering is just a
// tie-breaker for the impossible case); otherwise an empty --to means
// "host" (register a pairing code, wait for it to be used), non-empty
// means "guest" (the value is either a full peer ID or a pairing code —
// ResolveCounterpartArg tells them apart).
func CounterpartResolverFor(to, room string, onCode OnCodeFunc) CounterpartResolver {
	if room != "" {
		return RoomCounterpart(room)
	}
	if to == "" {
		return HostViaPairingCode(onCode)
	}
	return ResolveCounterpartArg(to)
}
