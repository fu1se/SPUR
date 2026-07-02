package domain

import "errors"

var (
	ErrPeerNotFound    = errors.New("domain: peer not found")
	ErrNetworkNotFound = errors.New("domain: network not found")
	ErrSessionNotFound = errors.New("domain: session not found")
)
