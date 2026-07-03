package domain

import "errors"

var (
	ErrPeerNotFound        = errors.New("domain: peer not found")
	ErrNetworkNotFound     = errors.New("domain: network not found")
	ErrSessionNotFound     = errors.New("domain: session not found")
	ErrInvalidInviteToken  = errors.New("domain: invalid or missing invite token")
	ErrPairingCodeNotFound = errors.New("domain: pairing code not found or expired")
	ErrRoomNotFound        = errors.New("domain: room not found")
	ErrRoomAlreadyExists   = errors.New("domain: room name already taken")
	ErrRoomFull            = errors.New("domain: room already has two members")
	ErrRoomNotReady        = errors.New("domain: the other participant hasn't joined this room yet")
	ErrNotRoomMember       = errors.New("domain: caller is not a member of this room")
)
