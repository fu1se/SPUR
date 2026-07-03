package usecase

import (
	"context"
	"fmt"

	"github.com/fu1se/spur/internal/usecase/port"
)

// ReceiveFiles is the "spur receive" side of file transfer mode: it
// accepts the one tunnel stream SendFiles opens and runs the three-phase
// protocol described in file_transfer_wire.go — read the manifest, offer
// to resume any files the destination already has data for, send back a
// resume plan, then receive each file's remaining bytes. Like SendFiles,
// this is a one-shot operation with a defined end, not a loop.
type ReceiveFiles struct {
	Sink   port.FileSink
	Tunnel port.TunnelConn

	// OnProgress, if set, is called as bytes are received — see
	// TransferProgress's doc comment. overallTotal is always known here
	// (unlike before resume support): the manifest phase gives the full
	// file list up front now, instead of the sender streaming one header
	// at a time interleaved with content.
	OnProgress TransferProgress

	// OnResumeOffer, if set, is asked whether to resume a detected
	// partially-complete transfer — see ResumeOffer's doc comment. nil
	// means always start fresh, same as before resume support existed.
	OnResumeOffer ResumeOffer
}

// Run blocks until every file has been received, the sender signals the
// end of the transfer, or an error occurs.
func (uc ReceiveFiles) Run(ctx context.Context) error {
	stream, err := uc.Tunnel.AcceptStream(ctx)
	if err != nil {
		return fmt.Errorf("usecase: accept stream: %w", err)
	}
	defer stream.Close()

	// Phase 1: manifest.
	var entries []port.FileEntry
	for {
		entry, end, err := readFileHeader(stream)
		if err != nil {
			return err
		}
		if end {
			break
		}
		entries = append(entries, entry)
	}

	// Decide a resume plan: how much of each file the destination
	// already has, offered to the caller as one combined yes/no rather
	// than per file (see ResumeOffer's doc comment for why).
	existing := make([]int64, len(entries))
	var filesWithData int
	var alreadyHave, total int64
	for i, entry := range entries {
		size, err := uc.Sink.ExistingSize(entry.RelPath)
		if err != nil {
			return fmt.Errorf("usecase: check existing size for %s: %w", entry.RelPath, err)
		}
		if size > entry.Size {
			// Local file is bigger than what's coming -- not a valid
			// resume candidate (e.g. a same-named but unrelated file).
			// Treat as "nothing to resume" for this entry rather than
			// guessing.
			size = 0
		}
		if size > 0 {
			filesWithData++
		}
		existing[i] = size
		alreadyHave += size
		total += entry.Size
	}

	resumeFrom := make([]int64, len(entries))
	if filesWithData > 0 && uc.OnResumeOffer != nil && uc.OnResumeOffer(filesWithData, alreadyHave, total) {
		copy(resumeFrom, existing)
	}
	// Otherwise resumeFrom stays all-zero: start every file fresh, the
	// same behavior file transfer always had before resume support.

	// Phase 2: resume plan.
	for _, offset := range resumeFrom {
		if err := writeResumeOffset(stream, offset); err != nil {
			return err
		}
	}

	// Phase 3: content.
	var overallDone int64
	for _, offset := range resumeFrom {
		overallDone += offset
	}
	for i, entry := range entries {
		if entry.Size > 0 && resumeFrom[i] == entry.Size {
			continue // already complete, sender won't send anything for it
		}
		// entry.Size == 0 always falls through here (see send_files.go's
		// matching comment) so an empty file still gets Create()d, even
		// though there's no content to copy.

		w, err := uc.Sink.Create(entry, resumeFrom[i])
		if err != nil {
			return fmt.Errorf("usecase: create %s: %w", entry.RelPath, err)
		}
		entryStart := overallDone
		remaining := entry.Size - resumeFrom[i]
		err = copyWithProgress(w, stream, remaining, func(received int64) {
			if uc.OnProgress != nil {
				fileDone := resumeFrom[i] + received
				uc.OnProgress(entry.RelPath, fileDone, entry.Size, entryStart+received, total)
			}
		})
		closeErr := w.Close()
		if err != nil {
			return fmt.Errorf("usecase: receive %s: %w", entry.RelPath, err)
		}
		if closeErr != nil {
			return fmt.Errorf("usecase: close %s: %w", entry.RelPath, closeErr)
		}
		overallDone += remaining
	}

	// Ack so the sender knows it's safe to tear down the tunnel — see
	// SendFiles.Run's doc comment for the race this closes.
	if _, err := stream.Write([]byte{1}); err != nil {
		return fmt.Errorf("usecase: send ack: %w", err)
	}
	// The same race applies symmetrically here: closing our own tunnel
	// right after Write returns could tear the connection down before
	// that single ack byte actually reaches the sender. Block until the
	// sender closes its side (which it only does after successfully
	// reading the ack) — any outcome here, clean EOF or a connection-level
	// error, means the sender is done and it's now safe for us to close
	// too.
	var discard [1]byte
	_, _ = stream.Read(discard[:])
	return nil
}
