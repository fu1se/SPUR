package usecase_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/localizator/internal/adapter/repository/memory"
	"github.com/fu1se/localizator/internal/domain"
	"github.com/fu1se/localizator/internal/usecase"
)

// TestJoinNetwork_ConcurrentJoinsDontLoseMembers is a regression test for
// a real race found during Phase 6's live TUN testing: two peers joining
// the same network at once could both read the network before either had
// saved its own membership, so both computed the same "next available IP"
// and the second Save silently overwrote the first peer's membership
// entry — one peer would join a mesh where it, itself, wasn't a member
// according to the other side. Fixed by making the read-mutate-write
// atomic via NetworkRepository.Update; this test drives enough concurrent
// joins that the old Save-based implementation reliably lost members.
func TestJoinNetwork_ConcurrentJoinsDontLoseMembers(t *testing.T) {
	networks := memory.NewNetworkRepository()
	uc := usecase.JoinNetwork{Networks: networks}

	const peerCount = 20

	var wg sync.WaitGroup
	for i := 0; i < peerCount; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var pub domain.PublicKey
			pub[0] = byte(i)
			_, err := uc.Execute(context.Background(), "home", domain.PeerID(fmt.Sprintf("peer-%d", i)), pub)
			require.NoError(t, err)
		}(i)
	}
	wg.Wait()

	final, err := networks.FindByName(context.Background(), "home")
	require.NoError(t, err)
	require.Len(t, final.Members, peerCount, "some concurrent joins were lost")

	seenIPs := make(map[string]bool, peerCount)
	for _, m := range final.Members {
		require.False(t, seenIPs[m.MeshIP.String()], "duplicate mesh IP assigned: %s", m.MeshIP)
		seenIPs[m.MeshIP.String()] = true
	}
}
