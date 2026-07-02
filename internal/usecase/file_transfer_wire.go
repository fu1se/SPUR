package usecase

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"github.com/fu1se/localizator/internal/usecase/port"
)

// The file transfer wire format is a sequence of headers, each followed by
// exactly that many bytes of file content, terminated by a zero-length
// path header (no size field after it) and then a single ack byte from
// the receiver back to the sender (see SendFiles.Run's doc comment for
// why the ack exists — it is not part of this framing, just the next
// thing written on the same stream).
//
//	header := pathLen(uint16) path(pathLen bytes) [size(uint64)]
//	pathLen == 0 means "end of transfer", no size field follows.

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

	return port.FileEntry{RelPath: string(pathBytes), Size: int64(size)}, false, nil
}
