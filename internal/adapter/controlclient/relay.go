package controlclient

import (
	"context"
	"fmt"
	"io"

	"github.com/fu1se/spur/internal/adapter/controlproto"
)

// OpenChannel implements port.Relay: it opens a stream, sends the relay
// header, and returns the stream itself as the raw duplex byte channel —
// no more framing happens on it after this point.
func (c *Client) OpenChannel(ctx context.Context, sessionID string) (io.ReadWriteCloser, error) {
	stream, err := c.conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, fmt.Errorf("controlclient: open stream: %w", err)
	}

	// Bind ctx only around the header writes: unlike every other RPC
	// here, the stream itself outlives this call (it becomes the relay
	// data channel), so the deadline must be cleared again before the
	// stream is handed off — release does exactly that (see
	// bindStreamToContext's doc comment).
	release := bindStreamToContext(ctx, stream)
	defer release()

	if err := controlproto.WriteMethod(stream, controlproto.MethodRelay); err != nil {
		_ = stream.Close()
		return nil, err
	}
	if err := controlproto.WriteFrame(stream, &controlproto.RelayOpenRequest{SessionId: sessionID}); err != nil {
		_ = stream.Close()
		return nil, err
	}

	return stream, nil
}
