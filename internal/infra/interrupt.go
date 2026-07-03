package infra

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// ContextWithConfirmedInterrupt returns a context cancelled only after two
// SIGINT (Ctrl+C) arrive within window of each other — guards a
// long-running tunnel or transfer against a single accidental keypress
// aborting it. warn (nil-safe) is called once, on the first Ctrl+C, so
// the caller can tell the user a second press is needed; if the second
// press doesn't come within window, the state resets and two presses are
// needed again from scratch. SIGTERM still cancels immediately on the
// first signal — it comes from a process manager or `kill`, not a stray
// keypress, so there's nothing to accidentally confirm.
//
// The returned cancel func must be called once the caller is done with
// ctx, same as any context.CancelFunc — it also stops the signal
// notification and waits for the background goroutine to exit, so it's
// safe to rely on cleanup having happened once it returns.
func ContextWithConfirmedInterrupt(parent context.Context, window time.Duration, warn func()) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer signal.Stop(sigCh)

		var lastInterrupt time.Time
		for {
			select {
			case <-ctx.Done():
				return
			case sig, ok := <-sigCh:
				if !ok {
					return
				}
				if sig == syscall.SIGTERM {
					cancel()
					return
				}
				now := time.Now()
				if !lastInterrupt.IsZero() && now.Sub(lastInterrupt) <= window {
					cancel()
					return
				}
				lastInterrupt = now
				if warn != nil {
					warn()
				}
			}
		}
	}()

	return ctx, func() {
		cancel()
		<-done
	}
}
