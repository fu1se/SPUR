package controlproto

import (
	"encoding/binary"
	"fmt"
	"io"

	"google.golang.org/protobuf/proto"
)

// maxFrameSize bounds a single control-message frame, guarding against a
// misbehaving peer sending an unbounded length prefix.
const maxFrameSize = 64 * 1024

// WriteFrame writes a length-prefixed protobuf message: a 4-byte
// big-endian length followed by the marshaled message.
func WriteFrame(w io.Writer, msg proto.Message) error {
	body, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("controlproto: marshal: %w", err)
	}

	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(body)))

	if _, err := w.Write(header[:]); err != nil {
		return fmt.Errorf("controlproto: write header: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("controlproto: write body: %w", err)
	}
	return nil
}

// ReadFrame reads one length-prefixed protobuf message written by WriteFrame.
func ReadFrame(r io.Reader, msg proto.Message) error {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return fmt.Errorf("controlproto: read header: %w", err)
	}

	size := binary.BigEndian.Uint32(header[:])
	if size > maxFrameSize {
		return fmt.Errorf("controlproto: frame too large: %d bytes", size)
	}

	body := make([]byte, size)
	if _, err := io.ReadFull(r, body); err != nil {
		return fmt.Errorf("controlproto: read body: %w", err)
	}

	if err := proto.Unmarshal(body, msg); err != nil {
		return fmt.Errorf("controlproto: unmarshal: %w", err)
	}
	return nil
}
