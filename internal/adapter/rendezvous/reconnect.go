package rendezvous

import (
	"context"
	"time"

	"github.com/fu1se/spur/internal/domain"
)

// OnReconnectFunc is told (nil-safe) that the tunnel was lost and a new
// rendezvous attempt starts after delay — mirroring cli.OnReconnectFunc,
// same reasoning as OnCodeFunc (see this package's doc comment).
type OnReconnectFunc func(cause error, delay time.Duration)

// reconnectConfig parameterizes runReconnectLoop's timing so tests can
// run it with microsecond delays; RunPersistent always uses
// defaultReconnectConfig.
type reconnectConfig struct {
	// initialBackoff is the delay before the first re-establish attempt;
	// it doubles per consecutive failure up to maxBackoff.
	initialBackoff time.Duration
	maxBackoff     time.Duration
	// stableAfter: when the data-plane op ran at least this long before
	// dying, the failure is treated as a fresh outage (backoff resets to
	// initialBackoff) rather than a continuation of a failing streak —
	// without this, one long-lived session bracketed by two quick
	// failures would keep escalating the delay forever; with a naive
	// "reset on every successful establish" instead, a tunnel that
	// establishes fine but dies instantly (e.g. the counterpart's local
	// service loops crashing) would hammer the server with zero effective
	// backoff, since every establish "succeeds".
	stableAfter time.Duration
}

var defaultReconnectConfig = reconnectConfig{
	initialBackoff: 2 * time.Second,
	maxBackoff:     time.Minute,
	stableAfter:    30 * time.Second,
}

// establishFunc abstracts Establish for runReconnectLoop so its retry
// policy is unit-testable without a real server/STUN/NAT stack behind it.
type establishFunc func(ctx context.Context, resolve CounterpartResolver) (*Tunnel, domain.PeerID, error)

// RunPersistent keeps op running over an established tunnel for as long
// as ctx lives: whenever the tunnel dies (op returns an error) or a
// re-establish attempt fails, it waits with exponential backoff and
// rendezvous-es again, so a network drop mid-session recovers by itself
// instead of killing the command.
//
// Two deliberate asymmetries:
//
//   - A failure on the very FIRST establish is returned as-is, with no
//     retry: at that point an error is far more likely a configuration
//     mistake (typoed pairing code, wrong --room, unreachable server
//     address) than a transient outage, and silently retrying a typo
//     forever would look like a hang.
//
//   - After the first success the counterpart is pinned
//     (FixedCounterpart) for every reconnect: a pairing code is a
//     one-shot, short-TTL secret that was consumed by the first
//     rendezvous — re-resolving it would fail (host mode would even mint
//     a brand-new code nobody is looking at), while the peer ID behind
//     it is stable across the drop. onSelfID/onVersionMismatch fire only
//     on the first attempt for the same reason: nothing about them
//     changes on reconnect.
//
// op returning nil ends the loop successfully (a finite operation, e.g.
// a completed file transfer); ctx cancellation ends it with ctx's error.
func RunPersistent(ctx context.Context, serverAddr, stunAddr, identityPath, trustStorePath, clientVersion string, resolve CounterpartResolver, onSelfID func(string), onVersionMismatch VersionMismatchFunc, onReconnect OnReconnectFunc, op func(context.Context, *Tunnel) error) error {
	first := true
	est := func(ctx context.Context, r CounterpartResolver) (*Tunnel, domain.PeerID, error) {
		selfID, mismatch := onSelfID, onVersionMismatch
		if !first {
			selfID, mismatch = func(string) {}, nil
		}
		tun, _, counterpart, err := Establish(ctx, serverAddr, stunAddr, identityPath, trustStorePath, clientVersion, r, selfID, mismatch)
		if err == nil {
			first = false
		}
		return tun, counterpart, err
	}
	return runReconnectLoop(ctx, defaultReconnectConfig, est, resolve, onReconnect, op)
}

func runReconnectLoop(ctx context.Context, cfg reconnectConfig, establish establishFunc, resolve CounterpartResolver, onReconnect OnReconnectFunc, op func(context.Context, *Tunnel) error) error {
	backoff := cfg.initialBackoff
	established := false

	for {
		tun, counterpart, err := establish(ctx, resolve)
		if err != nil {
			if !established {
				return err
			}
			if waitErr := waitBeforeReconnect(ctx, onReconnect, err, backoff); waitErr != nil {
				return waitErr
			}
			backoff = min(backoff*2, cfg.maxBackoff)
			continue
		}
		established = true
		resolve = FixedCounterpart(counterpart)

		started := time.Now()
		opErr := op(ctx, tun)
		tun.Close()
		if opErr == nil {
			return nil
		}
		if ctx.Err() != nil {
			// The op died because the caller is shutting down, not
			// because the tunnel broke — surface what the op said (it's
			// conventionally ctx.Err() itself, matching what the
			// non-persistent flow used to return on Ctrl+C).
			return opErr
		}
		if time.Since(started) >= cfg.stableAfter {
			backoff = cfg.initialBackoff
		}
		if waitErr := waitBeforeReconnect(ctx, onReconnect, opErr, backoff); waitErr != nil {
			return waitErr
		}
		backoff = min(backoff*2, cfg.maxBackoff)
	}
}

func waitBeforeReconnect(ctx context.Context, onReconnect OnReconnectFunc, cause error, delay time.Duration) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if onReconnect != nil {
		onReconnect(cause, delay)
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
