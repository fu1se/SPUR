package rendezvous

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/spur/internal/adapter/controlclient"
	"github.com/fu1se/spur/internal/domain"
	"github.com/fu1se/spur/internal/infra"
)

// testReconnectConfig keeps every delay tiny so the loop's policy (not
// wall-clock time) is what the tests exercise.
var testReconnectConfig = reconnectConfig{
	initialBackoff: time.Millisecond,
	maxBackoff:     8 * time.Millisecond,
	stableAfter:    time.Hour, // effectively "never stable" unless a test overrides
}

func TestRunReconnectLoop_FirstEstablishFailureIsFatal(t *testing.T) {
	boom := errors.New("boom")
	calls := 0
	est := func(context.Context, CounterpartResolver) (*Tunnel, domain.PeerID, error) {
		calls++
		return nil, "", boom
	}

	err := runReconnectLoop(context.Background(), testReconnectConfig, est, FixedCounterpart("peer"), nil,
		func(context.Context, *Tunnel) error { t.Fatal("op must not run"); return nil })

	require.ErrorIs(t, err, boom)
	require.Equal(t, 1, calls, "a first-attempt failure must not be retried")
}

func TestRunReconnectLoop_ReestablishesAfterTunnelDeath(t *testing.T) {
	tunnelDied := errors.New("tunnel died")
	opRuns := 0
	op := func(context.Context, *Tunnel) error {
		opRuns++
		if opRuns < 3 {
			return tunnelDied
		}
		return nil
	}

	var reconnects []error
	est := func(context.Context, CounterpartResolver) (*Tunnel, domain.PeerID, error) {
		return &Tunnel{}, "peer-a", nil
	}

	err := runReconnectLoop(context.Background(), testReconnectConfig, est, FixedCounterpart("peer-a"),
		func(cause error, _ time.Duration) { reconnects = append(reconnects, cause) }, op)

	require.NoError(t, err)
	require.Equal(t, 3, opRuns)
	require.Len(t, reconnects, 2)
	require.ErrorIs(t, reconnects[0], tunnelDied)
}

func TestRunReconnectLoop_PinsCounterpartAfterFirstSuccess(t *testing.T) {
	// est records which peer the resolver it was handed resolves to; the
	// loop must switch from the caller's resolver to
	// FixedCounterpart(<first result>) after the first success — a
	// pairing code is one-shot, re-resolving it on reconnect would fail.
	resolverCalls := 0
	callerResolver := CounterpartResolver(func(context.Context, *controlclient.Client, infra.Identity) (domain.PeerID, error) {
		resolverCalls++
		return "peer-from-code", nil
	})

	var resolved []domain.PeerID
	est := func(ctx context.Context, r CounterpartResolver) (*Tunnel, domain.PeerID, error) {
		peer, err := r(ctx, nil, infra.Identity{})
		require.NoError(t, err)
		resolved = append(resolved, peer)
		return &Tunnel{}, peer, nil
	}

	opRuns := 0
	err := runReconnectLoop(context.Background(), testReconnectConfig, est, callerResolver, nil,
		func(context.Context, *Tunnel) error {
			opRuns++
			if opRuns < 3 {
				return errors.New("died")
			}
			return nil
		})

	require.NoError(t, err)
	require.Equal(t, 1, resolverCalls, "the original resolver must run exactly once")
	require.Equal(t, []domain.PeerID{"peer-from-code", "peer-from-code", "peer-from-code"}, resolved)
}

func TestRunReconnectLoop_BackoffGrowsAndIsCapped(t *testing.T) {
	var delays []time.Duration
	est := func(context.Context, CounterpartResolver) (*Tunnel, domain.PeerID, error) {
		return &Tunnel{}, "peer", nil
	}
	opRuns := 0
	err := runReconnectLoop(context.Background(), testReconnectConfig, est, FixedCounterpart("peer"),
		func(_ error, delay time.Duration) { delays = append(delays, delay) },
		func(context.Context, *Tunnel) error {
			opRuns++
			if opRuns <= 6 {
				return errors.New("died instantly")
			}
			return nil
		})

	require.NoError(t, err)
	require.Equal(t, []time.Duration{
		time.Millisecond, 2 * time.Millisecond, 4 * time.Millisecond,
		8 * time.Millisecond, 8 * time.Millisecond, 8 * time.Millisecond,
	}, delays, "backoff must double per consecutive quick failure and cap at maxBackoff")
}

func TestRunReconnectLoop_BackoffResetsAfterStableSession(t *testing.T) {
	cfg := testReconnectConfig
	cfg.stableAfter = 10 * time.Millisecond

	var delays []time.Duration
	est := func(context.Context, CounterpartResolver) (*Tunnel, domain.PeerID, error) {
		return &Tunnel{}, "peer", nil
	}
	opRuns := 0
	err := runReconnectLoop(context.Background(), cfg, est, FixedCounterpart("peer"),
		func(_ error, delay time.Duration) { delays = append(delays, delay) },
		func(context.Context, *Tunnel) error {
			opRuns++
			switch opRuns {
			case 1, 2:
				return errors.New("died instantly") // grows backoff to 2ms
			case 3:
				time.Sleep(cfg.stableAfter + time.Millisecond) // a "stable" session
				return errors.New("died after running a while")
			default:
				return nil
			}
		})

	require.NoError(t, err)
	require.Len(t, delays, 3)
	require.Equal(t, 2*time.Millisecond, delays[1])
	require.Equal(t, time.Millisecond, delays[2], "a session that ran past stableAfter must reset the backoff")
}

func TestRunReconnectLoop_CancelDuringBackoffReturnsCtxErr(t *testing.T) {
	cfg := testReconnectConfig
	cfg.initialBackoff = time.Hour // the test must not actually wait this out

	ctx, cancel := context.WithCancel(context.Background())
	est := func(context.Context, CounterpartResolver) (*Tunnel, domain.PeerID, error) {
		return &Tunnel{}, "peer", nil
	}

	done := make(chan error, 1)
	go func() {
		done <- runReconnectLoop(ctx, cfg, est, FixedCounterpart("peer"),
			func(error, time.Duration) { cancel() },
			func(context.Context, *Tunnel) error { return errors.New("died") })
	}()

	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("loop did not return after cancellation during backoff")
	}
}
