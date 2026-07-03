package infra_test

import (
	"context"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/spur/internal/infra"
)

func sendInterrupt(t *testing.T) {
	t.Helper()
	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGINT))
}

func TestContextWithConfirmedInterrupt_SinglePressDoesNotCancel(t *testing.T) {
	ctx, stop := infra.ContextWithConfirmedInterrupt(context.Background(), 200*time.Millisecond, nil)
	defer stop()

	sendInterrupt(t)
	time.Sleep(50 * time.Millisecond)

	select {
	case <-ctx.Done():
		t.Fatal("a single Ctrl+C must not cancel the context")
	default:
	}
}

func TestContextWithConfirmedInterrupt_SecondPressWithinWindowCancels(t *testing.T) {
	ctx, stop := infra.ContextWithConfirmedInterrupt(context.Background(), 500*time.Millisecond, nil)
	defer stop()

	sendInterrupt(t)
	time.Sleep(20 * time.Millisecond)
	sendInterrupt(t)

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("two presses within the window should cancel the context")
	}
}

func TestContextWithConfirmedInterrupt_SecondPressAfterWindowResets(t *testing.T) {
	ctx, stop := infra.ContextWithConfirmedInterrupt(context.Background(), 50*time.Millisecond, nil)
	defer stop()

	sendInterrupt(t)
	time.Sleep(150 * time.Millisecond) // let the window lapse
	sendInterrupt(t)                   // treated as a fresh "first press"
	time.Sleep(50 * time.Millisecond)

	select {
	case <-ctx.Done():
		t.Fatal("a press after the window lapsed should count as a new first press, not cancel immediately")
	default:
	}
}

func TestContextWithConfirmedInterrupt_WarnCalledOncePerFirstPress(t *testing.T) {
	var calls int32
	ctx, stop := infra.ContextWithConfirmedInterrupt(context.Background(), 500*time.Millisecond, func() {
		atomic.AddInt32(&calls, 1)
	})
	defer stop()

	sendInterrupt(t)
	time.Sleep(20 * time.Millisecond)
	sendInterrupt(t) // second press cancels, doesn't call warn again

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("expected cancellation")
	}
	require.EqualValues(t, 1, atomic.LoadInt32(&calls))
}

func TestContextWithConfirmedInterrupt_SIGTERMCancelsImmediately(t *testing.T) {
	ctx, stop := infra.ContextWithConfirmedInterrupt(context.Background(), time.Second, nil)
	defer stop()

	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGTERM))

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("SIGTERM should cancel on the first signal, no confirmation needed")
	}
}

func TestContextWithConfirmedInterrupt_StopCleansUpWithoutHanging(t *testing.T) {
	ctx, stop := infra.ContextWithConfirmedInterrupt(context.Background(), time.Second, nil)
	stop()

	select {
	case <-ctx.Done():
	default:
		t.Fatal("stop() should cancel the context")
	}
}
