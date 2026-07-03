package usecase_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/spur/internal/adapter/repository/memory"
	"github.com/fu1se/spur/internal/domain"
	"github.com/fu1se/spur/internal/usecase"
)

func TestCreateRoom_RejectsDuplicateName(t *testing.T) {
	rooms := memory.NewRoomRepository()
	uc := usecase.CreateRoom{Rooms: rooms}

	_, err := uc.Execute(context.Background(), "friends", "creator")
	require.NoError(t, err)

	_, err = uc.Execute(context.Background(), "friends", "someone-else")
	require.ErrorIs(t, err, domain.ErrRoomAlreadyExists)
}

func TestJoinRoom_RejectsWrongInviteToken(t *testing.T) {
	rooms := memory.NewRoomRepository()
	created, err := usecase.CreateRoom{Rooms: rooms}.Execute(context.Background(), "friends", "creator")
	require.NoError(t, err)

	uc := usecase.JoinRoom{Rooms: rooms}
	_, err = uc.Execute(context.Background(), "friends", "guest", "wrong-token")
	require.ErrorIs(t, err, domain.ErrInvalidInviteToken)

	_, err = uc.Execute(context.Background(), "friends", "guest", "")
	require.ErrorIs(t, err, domain.ErrInvalidInviteToken)

	joined, err := uc.Execute(context.Background(), "friends", "guest", created.InviteToken)
	require.NoError(t, err)
	require.ElementsMatch(t, []domain.PeerID{"creator", "guest"}, joined.Members)
}

func TestJoinRoom_RejectsThirdMember(t *testing.T) {
	rooms := memory.NewRoomRepository()
	created, err := usecase.CreateRoom{Rooms: rooms}.Execute(context.Background(), "friends", "creator")
	require.NoError(t, err)

	uc := usecase.JoinRoom{Rooms: rooms}
	_, err = uc.Execute(context.Background(), "friends", "guest", created.InviteToken)
	require.NoError(t, err)

	_, err = uc.Execute(context.Background(), "friends", "intruder", created.InviteToken)
	require.ErrorIs(t, err, domain.ErrRoomFull)
}

func TestJoinRoom_RejoinDoesNotNeedToken(t *testing.T) {
	rooms := memory.NewRoomRepository()
	created, err := usecase.CreateRoom{Rooms: rooms}.Execute(context.Background(), "friends", "creator")
	require.NoError(t, err)

	uc := usecase.JoinRoom{Rooms: rooms}
	_, err = uc.Execute(context.Background(), "friends", "guest", created.InviteToken)
	require.NoError(t, err)

	// Already a member: no token needed, idempotent.
	rejoined, err := uc.Execute(context.Background(), "friends", "guest", "")
	require.NoError(t, err)
	require.ElementsMatch(t, []domain.PeerID{"creator", "guest"}, rejoined.Members)
}

func TestJoinRoom_UnknownRoomFails(t *testing.T) {
	rooms := memory.NewRoomRepository()
	_, err := usecase.JoinRoom{Rooms: rooms}.Execute(context.Background(), "nope", "guest", "any-token")
	require.ErrorIs(t, err, domain.ErrRoomNotFound)
}

func TestResolveRoom(t *testing.T) {
	rooms := memory.NewRoomRepository()
	created, err := usecase.CreateRoom{Rooms: rooms}.Execute(context.Background(), "friends", "creator")
	require.NoError(t, err)

	resolve := usecase.ResolveRoom{Rooms: rooms}

	// Not full yet: only the creator has joined.
	_, err = resolve.Execute(context.Background(), "friends", "creator")
	require.ErrorIs(t, err, domain.ErrRoomNotReady)

	_, err = usecase.JoinRoom{Rooms: rooms}.Execute(context.Background(), "friends", "guest", created.InviteToken)
	require.NoError(t, err)

	other, err := resolve.Execute(context.Background(), "friends", "creator")
	require.NoError(t, err)
	require.Equal(t, domain.PeerID("guest"), other)

	other, err = resolve.Execute(context.Background(), "friends", "guest")
	require.NoError(t, err)
	require.Equal(t, domain.PeerID("creator"), other)
}

func TestResolveRoom_RejectsNonMember(t *testing.T) {
	rooms := memory.NewRoomRepository()
	created, err := usecase.CreateRoom{Rooms: rooms}.Execute(context.Background(), "friends", "creator")
	require.NoError(t, err)
	_, err = usecase.JoinRoom{Rooms: rooms}.Execute(context.Background(), "friends", "guest", created.InviteToken)
	require.NoError(t, err)

	_, err = usecase.ResolveRoom{Rooms: rooms}.Execute(context.Background(), "friends", "stranger")
	require.ErrorIs(t, err, domain.ErrNotRoomMember)
}

func TestResolveRoom_UnknownRoomFails(t *testing.T) {
	rooms := memory.NewRoomRepository()
	_, err := usecase.ResolveRoom{Rooms: rooms}.Execute(context.Background(), "nope", "anyone")
	require.ErrorIs(t, err, domain.ErrRoomNotFound)
}
