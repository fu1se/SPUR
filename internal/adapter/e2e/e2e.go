// Package e2e wraps a port.TunnelConn with authenticated encryption keyed
// by the two peers' X25519 identities, independent of whatever transport
// carries the bytes underneath (see adapter/tunnel).
//
// Why this exists: P2P sessions get real encryption from QUIC's TLS 1.3,
// but the client currently trusts the server's certificate with
// InsecureSkipVerify (see CLAUDE.md's "Требования окружения для сборки" —
// full verification is still open work), so that hop isn't authenticated
// against the peer's actual identity. Relay sessions are worse: the
// rendezvous server splices the two sides' streams together in plaintext
// (see memory.RelayBroker) — it is the transport for that hop, so it can
// read and tamper with anything not encrypted above it. Wrapping every
// Stream here, regardless of which path was used, means the server (or a
// P2P man-in-the-middle) sees only ciphertext authenticated against the
// counterpart's real public key, closing both gaps with one mechanism.
package e2e

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"sync/atomic"

	"github.com/fu1se/spur/internal/domain"
	"github.com/fu1se/spur/internal/usecase/port"
)

// CombineSalt merges the two peers' independently generated per-session
// salts into the single value fed to deriveKeys as the HKDF salt. XOR is
// used because it's commutative — both sides compute the identical result
// regardless of which one they call "own" versus "peer" — without needing
// a canonical ordering the way domain.SessionIDFor does for peer IDs.
func CombineSalt(own, peer [32]byte) [32]byte {
	var combined [32]byte
	for i := range combined {
		combined[i] = own[i] ^ peer[i]
	}
	return combined
}

// deriveKeys computes the two directional AEAD keys for a link between
// self and counterpart from an X25519 shared secret. HKDF with distinct
// "info" labels per direction guarantees dialer's send key equals
// listener's recv key (and vice versa) without any extra coordination:
// both sides run the same derivation, they just pick opposite roles.
//
// salt (see CombineSalt) is mixed in specifically so that the derived keys
// are unique per session: priv/peerPub are the peers' persistent
// identities, so an ECDH of those two alone is identical across every
// session two given peers ever establish, which — combined with each
// Stream's nonce starting from a shared-but-still-small counter — risked
// AES-GCM nonce reuse across process runs, not just across streams within
// one run.
func deriveKeys(priv *ecdh.PrivateKey, peerPub domain.PublicKey, salt [32]byte, isDialer bool) (sendKey, recvKey [32]byte, err error) {
	remote, err := ecdh.X25519().NewPublicKey(peerPub[:])
	if err != nil {
		return sendKey, recvKey, fmt.Errorf("e2e: peer public key: %w", err)
	}

	shared, err := priv.ECDH(remote)
	if err != nil {
		return sendKey, recvKey, fmt.Errorf("e2e: ecdh: %w", err)
	}

	keyToDialer, err := hkdf.Key(sha256.New, shared, salt[:], "spur-e2e-to-dialer", 32)
	if err != nil {
		return sendKey, recvKey, fmt.Errorf("e2e: hkdf: %w", err)
	}
	keyToListener, err := hkdf.Key(sha256.New, shared, salt[:], "spur-e2e-to-listener", 32)
	if err != nil {
		return sendKey, recvKey, fmt.Errorf("e2e: hkdf: %w", err)
	}

	if isDialer {
		// The dialer sends on "to-listener" and receives on "to-dialer";
		// the listener does the opposite.
		copy(sendKey[:], keyToListener)
		copy(recvKey[:], keyToDialer)
	} else {
		copy(sendKey[:], keyToDialer)
		copy(recvKey[:], keyToListener)
	}
	return sendKey, recvKey, nil
}

// WrapConn wraps every Stream OpenStream/AcceptStream produces with
// authenticated encryption keyed by priv and peerPub. isDialer must match
// whatever value picked the transport role for conn (domain.IsDialer) —
// it decides which of the two derived keys is "mine to send with" versus
// "mine to receive with". salt must be CombineSalt(ownSalt, peerSalt) —
// both sides compute the same value from their own and the exchanged
// counterpart's domain.CandidateSet.Salt.
func WrapConn(conn port.TunnelConn, priv *ecdh.PrivateKey, peerPub domain.PublicKey, salt [32]byte, isDialer bool) (port.TunnelConn, error) {
	sendKey, recvKey, err := deriveKeys(priv, peerPub, salt, isDialer)
	if err != nil {
		return nil, err
	}
	return &wrappedConn{
		inner:     conn,
		sendKey:   sendKey,
		recvKey:   recvKey,
		sendNonce: new(atomic.Uint64),
		recvNonce: new(atomic.Uint64),
	}, nil
}

type wrappedConn struct {
	inner            port.TunnelConn
	sendKey, recvKey [32]byte

	// sendNonce/recvNonce are shared by every Stream this connection
	// produces, not reset per stream: a wrappedConn's send/recv key pair is
	// fixed for its whole lifetime, and AES-GCM requires every (key, nonce)
	// pair to be used at most once. A per-stream counter starting at 0
	// would let two concurrently open streams (e.g. two forwarded TCP
	// connections through one `spur connect`) encrypt their first frame
	// under the identical (key, nonce=0) pair — a two-time-pad break of
	// both confidentiality and, per GCM's "forbidden attack", forgeable
	// authentication for the rest of that key's lifetime.
	sendNonce, recvNonce *atomic.Uint64
}

func (c *wrappedConn) OpenStream(ctx context.Context) (port.Stream, error) {
	s, err := c.inner.OpenStream(ctx)
	if err != nil {
		return nil, err
	}
	return newStream(s, c.sendKey, c.recvKey, c.sendNonce, c.recvNonce)
}

func (c *wrappedConn) AcceptStream(ctx context.Context) (port.Stream, error) {
	s, err := c.inner.AcceptStream(ctx)
	if err != nil {
		return nil, err
	}
	return newStream(s, c.sendKey, c.recvKey, c.sendNonce, c.recvNonce)
}

func (c *wrappedConn) Close() error { return c.inner.Close() }

// stream implements port.Stream, encrypting each Write call as one
// length-prefixed AEAD-sealed frame and decrypting frames on Read,
// buffering any plaintext the caller's buffer couldn't hold yet.
type stream struct {
	inner            port.Stream
	sendAEAD         cipher.AEAD
	recvAEAD         cipher.AEAD
	sendNonce        *atomic.Uint64
	recvNonce        *atomic.Uint64
	pendingPlaintext []byte
}

const maxFrameSize = 65535

func newStream(inner port.Stream, sendKey, recvKey [32]byte, sendNonce, recvNonce *atomic.Uint64) (*stream, error) {
	sendAEAD, err := newAEAD(sendKey)
	if err != nil {
		return nil, err
	}
	recvAEAD, err := newAEAD(recvKey)
	if err != nil {
		return nil, err
	}
	return &stream{inner: inner, sendAEAD: sendAEAD, recvAEAD: recvAEAD, sendNonce: sendNonce, recvNonce: recvNonce}, nil
}

func newAEAD(key [32]byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("e2e: aes cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("e2e: gcm: %w", err)
	}
	return aead, nil
}

func nonceFromCounter(aead cipher.AEAD, counter uint64) []byte {
	nonce := make([]byte, aead.NonceSize())
	binary.BigEndian.PutUint64(nonce[len(nonce)-8:], counter)
	return nonce
}

func (s *stream) Write(p []byte) (int, error) {
	if len(p) > maxFrameSize {
		return 0, fmt.Errorf("e2e: write too large: %d bytes", len(p))
	}

	counter := s.sendNonce.Add(1) - 1
	nonce := nonceFromCounter(s.sendAEAD, counter)
	ciphertext := s.sendAEAD.Seal(nil, nonce, p, nil)

	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(ciphertext)))
	if _, err := s.inner.Write(lenBuf[:]); err != nil {
		return 0, err
	}
	if _, err := s.inner.Write(ciphertext); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (s *stream) Read(p []byte) (int, error) {
	if len(s.pendingPlaintext) > 0 {
		n := copy(p, s.pendingPlaintext)
		s.pendingPlaintext = s.pendingPlaintext[n:]
		return n, nil
	}

	var lenBuf [4]byte
	if _, err := io.ReadFull(s.inner, lenBuf[:]); err != nil {
		return 0, err
	}
	size := binary.BigEndian.Uint32(lenBuf[:])
	if size > maxFrameSize+16 { // +16 for the GCM tag
		return 0, fmt.Errorf("e2e: frame too large: %d", size)
	}

	ciphertext := make([]byte, size)
	if _, err := io.ReadFull(s.inner, ciphertext); err != nil {
		return 0, err
	}

	counter := s.recvNonce.Add(1) - 1
	nonce := nonceFromCounter(s.recvAEAD, counter)
	plaintext, err := s.recvAEAD.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return 0, fmt.Errorf("e2e: decrypt: %w", err)
	}

	n := copy(p, plaintext)
	if n < len(plaintext) {
		s.pendingPlaintext = plaintext[n:]
	}
	return n, nil
}

func (s *stream) Close() error {
	return s.inner.Close()
}
