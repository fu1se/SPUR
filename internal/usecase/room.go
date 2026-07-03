package usecase

import (
	"context"
	"crypto/subtle"

	"github.com/fu1se/spur/internal/domain"
	"github.com/fu1se/spur/internal/usecase/port"
)

// CreateRoom is the server-side use case backing `spur room create`: it
// creates a brand-new, persistent room with the caller as its first
// member and mints an invite token for whoever should join as the
// second. Fails if the name is already taken — unlike JoinNetwork, room
// creation is a deliberate, explicit step, not implicit on first join:
// a room names a specific pair of people, so silently letting anyone who
// guesses a free name "create" it on join would be indistinguishable
// from a name collision between two unrelated pairs.
type CreateRoom struct {
	Rooms port.RoomRepository
}

func (uc CreateRoom) Execute(ctx context.Context, name string, creator domain.PeerID) (domain.Room, error) {
	return uc.Rooms.Update(ctx, name, func(room domain.Room) (domain.Room, error) {
		if room.InviteToken != "" {
			return domain.Room{}, domain.ErrRoomAlreadyExists
		}
		token, err := generateInviteToken()
		if err != nil {
			return domain.Room{}, err
		}
		return domain.Room{Name: name, InviteToken: token, Members: []domain.PeerID{creator}}, nil
	})
}

// JoinRoom is the server-side use case backing `spur room join`: adds
// the caller as the room's second member if inviteToken matches, or
// simply confirms membership if the caller already belongs to the room
// (idempotent rejoin, same convention as JoinNetwork).
type JoinRoom struct {
	Rooms port.RoomRepository
}

func (uc JoinRoom) Execute(ctx context.Context, name string, peer domain.PeerID, inviteToken string) (domain.Room, error) {
	return uc.Rooms.Update(ctx, name, func(room domain.Room) (domain.Room, error) {
		if room.InviteToken == "" {
			return domain.Room{}, domain.ErrRoomNotFound
		}
		if room.HasMember(peer) {
			return room, nil
		}
		if len(room.Members) >= 2 {
			return domain.Room{}, domain.ErrRoomFull
		}
		// Constant-time comparison: see JoinNetwork's identical rationale
		// — the invite token is a bearer credential, a timing side
		// channel would help an attacker brute-force it faster.
		if inviteToken == "" || subtle.ConstantTimeCompare([]byte(inviteToken), []byte(room.InviteToken)) != 1 {
			return domain.Room{}, domain.ErrInvalidInviteToken
		}
		room.Members = append(room.Members, peer)
		return room, nil
	})
}

// ResolveRoom is the server-side use case behind connect/expose/send/
// receive's --room flag: given a room name and the caller's own identity,
// return the other member's peer ID. Requires the caller to already be a
// member and the room to be full (both participants joined) — there is
// nothing to resolve to otherwise.
type ResolveRoom struct {
	Rooms port.RoomRepository
}

func (uc ResolveRoom) Execute(ctx context.Context, name string, peer domain.PeerID) (domain.PeerID, error) {
	room, err := uc.Rooms.FindByName(ctx, name)
	if err != nil {
		return "", err
	}
	if !room.HasMember(peer) {
		return "", domain.ErrNotRoomMember
	}
	other, ok := room.OtherMember(peer)
	if !ok {
		return "", domain.ErrRoomNotReady
	}
	return other, nil
}
