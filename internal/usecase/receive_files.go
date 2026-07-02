package usecase

import (
	"context"
	"fmt"
	"io"

	"github.com/fu1se/spur/internal/usecase/port"
)

// ReceiveFiles is the "spur receive" side of file transfer mode: it
// accepts the one tunnel stream SendFiles opens, reads headers and file
// content until the end-of-transfer marker, writing each file through
// Sink. Like SendFiles, this is a one-shot operation with a defined end,
// not a loop.
type ReceiveFiles struct {
	Sink   port.FileSink
	Tunnel port.TunnelConn
}

// Run blocks until every file has been received, the sender signals the
// end of the transfer, or an error occurs.
func (uc ReceiveFiles) Run(ctx context.Context) error {
	stream, err := uc.Tunnel.AcceptStream(ctx)
	if err != nil {
		return fmt.Errorf("usecase: accept stream: %w", err)
	}
	defer stream.Close()

	for {
		entry, end, err := readFileHeader(stream)
		if err != nil {
			return err
		}
		if end {
			// Ack so the sender knows it's safe to tear down the tunnel
			// — see SendFiles.Run's doc comment for the race this closes.
			if _, err := stream.Write([]byte{1}); err != nil {
				return fmt.Errorf("usecase: send ack: %w", err)
			}
			// The same race applies symmetrically here: closing our own
			// tunnel right after Write returns could tear the connection
			// down before that single ack byte actually reaches the
			// sender. Block until the sender closes its side (which it
			// only does after successfully reading the ack) — any
			// outcome here, clean EOF or a connection-level error, means
			// the sender is done and it's now safe for us to close too.
			var discard [1]byte
			_, _ = stream.Read(discard[:])
			return nil
		}

		w, err := uc.Sink.Create(entry)
		if err != nil {
			return fmt.Errorf("usecase: create %s: %w", entry.RelPath, err)
		}
		_, err = io.CopyN(w, stream, entry.Size)
		closeErr := w.Close()
		if err != nil {
			return fmt.Errorf("usecase: receive %s: %w", entry.RelPath, err)
		}
		if closeErr != nil {
			return fmt.Errorf("usecase: close %s: %w", entry.RelPath, closeErr)
		}
	}
}
