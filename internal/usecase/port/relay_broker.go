package port

import (
	"context"
	"io"
)

// RelayBroker is the server-side rendezvous point for relay fallback: it
// pairs the two peers' relay streams for the same session and splices
// them together (bidirectional copy). Join blocks until the counterpart
// joins the same sessionID and the spliced connection ends, or ctx is
// done. It is distinct from Relay, which is the client's view of opening
// a relay channel over the wire.
type RelayBroker interface {
	Join(ctx context.Context, sessionID string, conn io.ReadWriteCloser) error
}
