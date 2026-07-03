package usecase

import (
	"context"
	"fmt"
	"io"

	"github.com/fu1se/spur/internal/usecase/port"
)

// SendFiles is the "spur send" side of file transfer mode: it opens one
// dedicated tunnel stream, writes every file Source.List returns (each
// preceded by a small header carrying its relative path and size — see
// file_transfer_wire.go), then closes the stream. Unlike port-forward's
// use cases, this is a one-shot operation, not a loop accepting many
// connections — a transfer has a defined end.
type SendFiles struct {
	Source port.FileSource
	Tunnel port.TunnelConn

	// OnProgress, if set, is called as bytes are sent — see
	// TransferProgress's doc comment.
	OnProgress TransferProgress
}

// Run blocks until every file has been sent, acknowledged by the
// receiver, or an error occurs.
//
// Run waits for a one-byte ack (see receive_files.go) after the end
// marker before returning, instead of returning as soon as the last byte
// is handed to Stream.Write. Without it, the caller's usual next step
// (rendezvous's tun.Close(), tearing down the whole tunnel connection
// right after Run returns) can race a QUIC stream's graceful Close():
// Close() starts flushing and sends a FIN but does not itself block
// until the peer has actually received everything, so closing the parent
// connection immediately after can abort still-in-flight data. This was
// a real bug found live: a small file sent first came through intact, a
// larger one (500KB, spanning many packets) after it arrived truncated
// and the transfer errored out with a QUIC "Application error 0x0"
// reading a stream mid-flight — the connection had already been torn
// down out from under it. Port-forward/mesh never hit this because they
// keep the tunnel open for the process's whole lifetime; file transfer is
// the first thing in this codebase that deliberately closes right after
// one logical operation completes.
func (uc SendFiles) Run(ctx context.Context) error {
	stream, err := uc.Tunnel.OpenStream(ctx)
	if err != nil {
		return fmt.Errorf("usecase: open stream: %w", err)
	}
	defer stream.Close()

	entries, err := uc.Source.List()
	if err != nil {
		return fmt.Errorf("usecase: list files: %w", err)
	}

	var overallTotal int64
	for _, entry := range entries {
		overallTotal += entry.Size
	}

	var overallDone int64
	for _, entry := range entries {
		if err := writeFileHeader(stream, entry); err != nil {
			return err
		}

		r, err := uc.Source.Open(entry.RelPath)
		if err != nil {
			return fmt.Errorf("usecase: open %s: %w", entry.RelPath, err)
		}
		entryStart := overallDone
		err = copyWithProgress(stream, r, entry.Size, func(fileDone int64) {
			if uc.OnProgress != nil {
				uc.OnProgress(entry.RelPath, fileDone, entry.Size, entryStart+fileDone, overallTotal)
			}
		})
		closeErr := r.Close()
		if err != nil {
			return fmt.Errorf("usecase: send %s: %w", entry.RelPath, err)
		}
		if closeErr != nil {
			return fmt.Errorf("usecase: close %s: %w", entry.RelPath, closeErr)
		}
		overallDone += entry.Size
	}

	if err := writeEndMarker(stream); err != nil {
		return err
	}

	var ack [1]byte
	if _, err := io.ReadFull(stream, ack[:]); err != nil {
		return fmt.Errorf("usecase: wait for receiver ack: %w", err)
	}
	return nil
}
