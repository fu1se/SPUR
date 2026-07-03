package usecase

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"github.com/fu1se/spur/internal/usecase/port"
)

// The file transfer wire format has three phases on one stream:
//
//  1. Manifest (sender -> receiver): one header per file, terminated by a
//     zero-length path header (no size field after it).
//  2. Resume plan (receiver -> sender): one resume offset per file, in
//     the same order the manifest listed them — no length prefix needed,
//     both sides already agree on the count from phase 1.
//  3. Content (sender -> receiver): for each file, in order,
//     exactly (size - resumeFrom) bytes starting at offset resumeFrom in
//     the source file. No further per-file framing: order and the resume
//     plan from phase 2 fully determine where one file's bytes end and
//     the next's begin.
//
// Then a single ack byte from the receiver back to the sender (see
// SendFiles.Run's doc comment for why) — not part of this framing, just
// the next thing written on the same stream once phase 3 finishes.
//
//	header := pathLen(uint16) path(pathLen bytes) [size(uint64)]
//	pathLen == 0 means "end of manifest", no size field follows.
//	resumeOffset := uint64

func writeFileHeader(w io.Writer, e port.FileEntry) error {
	pathBytes := []byte(e.RelPath)
	if len(pathBytes) == 0 {
		return fmt.Errorf("usecase: empty relative path")
	}
	if len(pathBytes) > math.MaxUint16 {
		return fmt.Errorf("usecase: relative path too long: %d bytes", len(pathBytes))
	}
	if err := binary.Write(w, binary.BigEndian, uint16(len(pathBytes))); err != nil {
		return fmt.Errorf("usecase: write path length: %w", err)
	}
	if _, err := w.Write(pathBytes); err != nil {
		return fmt.Errorf("usecase: write path: %w", err)
	}
	if err := binary.Write(w, binary.BigEndian, uint64(e.Size)); err != nil {
		return fmt.Errorf("usecase: write size: %w", err)
	}
	return nil
}

func writeEndMarker(w io.Writer) error {
	if err := binary.Write(w, binary.BigEndian, uint16(0)); err != nil {
		return fmt.Errorf("usecase: write end marker: %w", err)
	}
	return nil
}

// readFileHeader returns (entry, end, err). end=true (err=nil) signals a
// clean end-of-transfer marker; no entry follows.
func readFileHeader(r io.Reader) (entry port.FileEntry, end bool, err error) {
	var pathLen uint16
	if err := binary.Read(r, binary.BigEndian, &pathLen); err != nil {
		return port.FileEntry{}, false, fmt.Errorf("usecase: read path length: %w", err)
	}
	if pathLen == 0 {
		return port.FileEntry{}, true, nil
	}

	pathBytes := make([]byte, pathLen)
	if _, err := io.ReadFull(r, pathBytes); err != nil {
		return port.FileEntry{}, false, fmt.Errorf("usecase: read path: %w", err)
	}

	var size uint64
	if err := binary.Read(r, binary.BigEndian, &size); err != nil {
		return port.FileEntry{}, false, fmt.Errorf("usecase: read size: %w", err)
	}
	if size > math.MaxInt64 {
		// An unchecked uint64->int64 cast here would silently produce a
		// negative Size. io.CopyN then treats "already have -N of N
		// bytes" as trivially satisfied (0 < negative is false) and
		// returns success having consumed zero bytes of what the sender
		// actually wrote as file content -- desyncing the rest of the
		// stream's framing into garbage instead of failing cleanly here.
		return port.FileEntry{}, false, fmt.Errorf("usecase: declared file size %d overflows int64", size)
	}

	return port.FileEntry{RelPath: string(pathBytes), Size: int64(size)}, false, nil
}

func writeResumeOffset(w io.Writer, offset int64) error {
	if err := binary.Write(w, binary.BigEndian, uint64(offset)); err != nil {
		return fmt.Errorf("usecase: write resume offset: %w", err)
	}
	return nil
}

func readResumeOffset(r io.Reader) (int64, error) {
	var offset uint64
	if err := binary.Read(r, binary.BigEndian, &offset); err != nil {
		return 0, fmt.Errorf("usecase: read resume offset: %w", err)
	}
	if offset > math.MaxInt64 {
		// Same reasoning as readFileHeader's size overflow check: an
		// unchecked cast could silently produce a negative offset and
		// desync the rest of the stream instead of failing cleanly here.
		return 0, fmt.Errorf("usecase: resume offset %d overflows int64", offset)
	}
	return int64(offset), nil
}
