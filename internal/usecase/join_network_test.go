package usecase_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/spur/internal/adapter/repository/memory"
	"github.com/fu1se/spur/internal/domain"
	"github.com/fu1se/spur/internal/usecase"
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
//
// The first join happens alone to create the network and learn its
// invite token — with token gating (Phase 7) in place, that's the only
// realistic way many peers end up joining "at once" in practice, since
// joining an existing network now requires already knowing the token.
func TestJoinNetwork_ConcurrentJoinsDontLoseMembers(t *testing.T) {
	networks := memory.NewNetworkRepository()
	uc := usecase.JoinNetwork{Networks: networks}

	var firstPub domain.PublicKey
	firstPub[0] = 0xFF
	creator, err := uc.Execute(context.Background(), "home", "peer-creator", firstPub, "")
	require.NoError(t, err)

	const peerCount = 20

	var wg sync.WaitGroup
	for i := 0; i < peerCount; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var pub domain.PublicKey
			pub[0] = byte(i)
			_, err := uc.Execute(context.Background(), "home", domain.PeerID(fmt.Sprintf("peer-%d", i)), pub, creator.InviteToken)
			require.NoError(t, err)
		}(i)
	}
	wg.Wait()

	final, err := networks.FindByName(context.Background(), "home")
	require.NoError(t, err)
	require.Len(t, final.Members, peerCount+1, "some concurrent joins were lost") // +1 for the creator

	seenIPs := make(map[string]bool, peerCount+1)
	for _, m := range final.Members {
		require.False(t, seenIPs[m.MeshIP.String()], "duplicate mesh IP assigned: %s", m.MeshIP)
		seenIPs[m.MeshIP.String()] = true
	}
}

func TestJoinNetwork_RejectsWrongInviteToken(t *testing.T) {
	networks := memory.NewNetworkRepository()
	uc := usecase.JoinNetwork{Networks: networks}

	var creatorPub domain.PublicKey
	creatorPub[0] = 1
	_, err := uc.Execute(context.Background(), "home", "creator", creatorPub, "")
	require.NoError(t, err)

	var otherPub domain.PublicKey
	otherPub[0] = 2
	_, err = uc.Execute(context.Background(), "home", "other", otherPub, "wrong-token")
	require.ErrorIs(t, err, domain.ErrInvalidInviteToken)

	_, err = uc.Execute(context.Background(), "home", "other", otherPub, "")
	require.ErrorIs(t, err, domain.ErrInvalidInviteToken)
}

func TestJoinNetwork_RejoinDoesNotNeedToken(t *testing.T) {
	networks := memory.NewNetworkRepository()
	uc := usecase.JoinNetwork{Networks: networks}

	var pub domain.PublicKey
	pub[0] = 1
	first, err := uc.Execute(context.Background(), "home", "peer", pub, "")
	require.NoError(t, err)

	// Same peer, no token this time: already a member, so it's let
	// through without re-checking.
	second, err := uc.Execute(context.Background(), "home", "peer", pub, "")
	require.NoError(t, err)
	require.Equal(t, first.Members, second.Members)
}
