package controlproto

import (
	"fmt"
	"io"
)

// Method tags which control-protocol RPC a freshly opened QUIC stream
// carries. A stream is single-purpose: the caller writes the Method byte,
// then one request frame, then reads one response frame, then closes it.
type Method byte

const (
	MethodRegister Method = iota + 1
	MethodPublishCandidates
	MethodAwaitCandidates
	// MethodRelay: after the RelayOpenRequest frame, the rest of the
	// stream is a raw, unframed byte pipe — see RelayOpenRequest's doc.
	MethodRelay
	MethodJoinNetwork
	MethodRegisterPairingCode
	MethodResolvePairingCode
	MethodAwaitPairingCodeUse
	MethodCreateRoom
	MethodJoinRoom
	MethodResolveRoom
)

func WriteMethod(w io.Writer, m Method) error {
	if _, err := w.Write([]byte{byte(m)}); err != nil {
		return fmt.Errorf("controlproto: write method: %w", err)
	}
	return nil
}

func ReadMethod(r io.Reader) (Method, error) {
	var b [1]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return 0, fmt.Errorf("controlproto: read method: %w", err)
	}
	return Method(b[0]), nil
}
