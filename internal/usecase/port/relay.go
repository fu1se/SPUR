package port

import (
	"context"
	"io"
)

// Relay provides a byte-forwarding fallback path through the rendezvous
// server for sessions where hole punching did not succeed (e.g. symmetric
// NAT on either side). Bytes flowing through a Relay are still expected to
// be end-to-end encrypted by the data plane above it — the relay must not
// be able to read plaintext.
type Relay interface {
	OpenChannel(ctx context.Context, sessionID string) (io.ReadWriteCloser, error)
}
