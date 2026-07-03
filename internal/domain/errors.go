package domain

import "errors"

var (
	ErrPeerNotFound        = errors.New("domain: peer not found")
	ErrNetworkNotFound     = errors.New("domain: network not found")
	ErrSessionNotFound     = errors.New("domain: session not found")
	ErrInvalidInviteToken  = errors.New("domain: invalid or missing invite token")
	ErrPairingCodeNotFound = errors.New("domain: pairing code not found or expired")
)
