package usecase_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/spur/internal/domain"
	"github.com/fu1se/spur/internal/usecase"
)

// memPairingCodeStore is a minimal in-memory port.PairingCodeStore mock,
// good enough to exercise the usecase layer without a real server —
// per the project's "port + usecase with mock tests first" rule.
type memPairingCodeStore struct {
	mu    sync.Mutex
	codes map[string]domain.PeerID
	used  map[string]chan domain.PeerID
}

func newMemPairingCodeStore() *memPairingCodeStore {
	return &memPairingCodeStore{
		codes: make(map[string]domain.PeerID),
		used:  make(map[string]chan domain.PeerID),
	}
}

func (s *memPairingCodeStore) Register(_ context.Context, code string, host domain.PeerID, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.codes[code] = host
	s.used[code] = make(chan domain.PeerID, 1)
	return nil
}

func (s *memPairingCodeStore) Resolve(_ context.Context, code string, guest domain.PeerID) (domain.PeerID, error) {
	s.mu.Lock()
	host, ok := s.codes[code]
	ch := s.used[code]
	s.mu.Unlock()
	if !ok {
		return "", domain.ErrPairingCodeNotFound
	}
	select {
	case ch <- guest:
	default:
	}
	return host, nil
}

func (s *memPairingCodeStore) AwaitUse(ctx context.Context, code string) (domain.PeerID, error) {
	s.mu.Lock()
	ch, ok := s.used[code]
	s.mu.Unlock()
	if !ok {
		return "", domain.ErrPairingCodeNotFound
	}
	select {
	case guest := <-ch:
		return guest, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func TestRegisterPairingCode_ProducesAUsableCode(t *testing.T) {
	store := newMemPairingCodeStore()
	uc := usecase.RegisterPairingCode{Store: store}

	code, err := uc.Execute(context.Background(), "host-peer")
	require.NoError(t, err)
	require.Len(t, code, 6)

	got, err := usecase.ResolvePairingCode{Store: store}.Execute(context.Background(), code, "guest-peer")
	require.NoError(t, err)
	require.Equal(t, domain.PeerID("host-peer"), got)
}

func TestRegisterPairingCode_ProducesDifferentCodesEachTime(t *testing.T) {
	store := newMemPairingCodeStore()
	uc := usecase.RegisterPairingCode{Store: store}

	seen := make(map[string]bool)
	for i := 0; i < 20; i++ {
		code, err := uc.Execute(context.Background(), "host-peer")
		require.NoError(t, err)
		seen[code] = true
	}
	require.Greater(t, len(seen), 1, "20 draws from a ~2^30 space should not collide down to a single value")
}

func TestResolvePairingCode_UnknownCodeFails(t *testing.T) {
	store := newMemPairingCodeStore()
	_, err := usecase.ResolvePairingCode{Store: store}.Execute(context.Background(), "NOPE99", "guest-peer")
	require.ErrorIs(t, err, domain.ErrPairingCodeNotFound)
}

// TestAwaitPairingCodeUse_UnblocksWhenResolved is the core round trip the
// single-command connect flow depends on: the host registers a code and
// blocks in AwaitPairingCodeUse; some time later a guest resolves that
// code; the host's blocked call must return the guest's peer ID, not just
// time out.
func TestAwaitPairingCodeUse_UnblocksWhenResolved(t *testing.T) {
	store := newMemPairingCodeStore()

	code, err := usecase.RegisterPairingCode{Store: store}.Execute(context.Background(), "host-peer")
	require.NoError(t, err)

	resultCh := make(chan domain.PeerID, 1)
	go func() {
		guest, err := usecase.AwaitPairingCodeUse{Store: store}.Execute(context.Background(), code)
		require.NoError(t, err)
		resultCh <- guest
	}()

	time.Sleep(20 * time.Millisecond) // let AwaitUse actually start blocking
	host, err := usecase.ResolvePairingCode{Store: store}.Execute(context.Background(), code, "guest-peer")
	require.NoError(t, err)
	require.Equal(t, domain.PeerID("host-peer"), host)

	select {
	case guest := <-resultCh:
		require.Equal(t, domain.PeerID("guest-peer"), guest)
	case <-time.After(time.Second):
		t.Fatal("AwaitPairingCodeUse should have unblocked once Resolve was called")
	}
}

func TestAwaitPairingCodeUse_CancelledContext(t *testing.T) {
	store := newMemPairingCodeStore()
	code, err := usecase.RegisterPairingCode{Store: store}.Execute(context.Background(), "host-peer")
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err = usecase.AwaitPairingCodeUse{Store: store}.Execute(ctx, code)
	require.Error(t, err)
}
