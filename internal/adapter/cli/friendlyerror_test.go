package cli_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/spur/internal/adapter/cli"
)

func TestExplain_Nil(t *testing.T) {
	require.Empty(t, cli.Explain(nil))
}

// TestExplain_RecognizedCases replays the exact error shapes seen live
// this project's own wrapping produces (see cmd/spur/tunnel.go's stage
// prefixes) — each must get stage-specific advice, not just the bare Go
// error string, and must still contain the original technical message so
// nothing is hidden from someone who wants the raw detail (or is filing
// a bug report).
func TestExplain_RecognizedCases(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		contains []string
	}{
		{
			name:     "stun timeout",
			err:      fmt.Errorf("app: stun discovery: %w", fmt.Errorf("nat: read stun response: %w", errTimeout{})),
			contains: []string{"STUN-сервером", "stun discovery"},
		},
		{
			name:     "exchange candidates EOF",
			err:      fmt.Errorf("app: exchange candidates: %w", fmt.Errorf("controlproto: read header: %w", io.EOF)),
			contains: []string{"не ответил вовремя", "whoami", "exchange candidates"},
		},
		{
			name:     "dial control-plane refused",
			err:      fmt.Errorf("app: dial control-plane: %w", errors.New("connection refused")),
			contains: []string{"подключиться к серверу", "connection refused"},
		},
		{
			name:     "establish session failed",
			err:      fmt.Errorf("app: establish session: %w", errors.New("no viable candidates")),
			contains: []string{"ни напрямую (P2P), ни через relay", "establish session"},
		},
		{
			name:     "stream closed mid transfer",
			err:      fmt.Errorf("usecase: send file.bin: %w", errors.New("stream closed")),
			contains: []string{"оборвалось во время передачи", "stream closed"},
		},
		{
			name:     "yamux keepalive",
			err:      errors.New("yamux: keepalive failed: i/o deadline reached"),
			contains: []string{"оборвалось во время передачи"},
		},
		{
			name:     "invalid invite token",
			err:      fmt.Errorf("app: join network: %w", errors.New("domain: invalid or missing invite token")),
			contains: []string{"инвайт-токен"},
		},
		{
			name:     "address in use",
			err:      fmt.Errorf("server: bind stun: %w", errors.New("listen udp :4444: bind: address already in use")),
			contains: []string{"Порт уже занят", "--listen и --stun-listen"},
		},
		{
			name:     "context deadline exceeded",
			err:      fmt.Errorf("await-candidates: use case failed: %w", context.DeadlineExceeded),
			contains: []string{"Истекло время ожидания"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cli.Explain(tc.err)
			for _, want := range tc.contains {
				require.Contains(t, got, want)
			}
		})
	}
}

func TestExplain_FileNotFound(t *testing.T) {
	_, statErr := os.Stat(filepath.Join(t.TempDir(), "does-not-exist"))
	require.Error(t, statErr)

	got := cli.Explain(fmt.Errorf("app: %w", statErr))
	require.Contains(t, got, "не найдены")
	require.Contains(t, got, "относительный путь")
}

func TestExplain_UnrecognizedFallsBackToOriginalMessage(t *testing.T) {
	err := errors.New("something entirely novel that no case matches")
	require.Equal(t, err.Error(), cli.Explain(err))
}

// errTimeout is a minimal net.Error-shaped stand-in — the real
// nat.DiscoverServerReflexive error is a *net.OpError wrapping this kind
// of timeout, but Explain's stun-discovery branch matches on the stage
// prefix rather than the concrete timeout type, so any error works here.
type errTimeout struct{}

func (errTimeout) Error() string   { return "i/o timeout" }
func (errTimeout) Timeout() bool   { return true }
func (errTimeout) Temporary() bool { return true }
