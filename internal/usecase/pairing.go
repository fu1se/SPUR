package usecase

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/fu1se/spur/internal/domain"
	"github.com/fu1se/spur/internal/usecase/port"
)

// pairingCodeAlphabet is Crockford's base32 minus the digits/letters it
// deliberately excludes for being visually ambiguous (0/O, 1/I/L, U) —
// this alphabet is meant to be read off a screen and typed by a human
// without transcription errors, unlike a peer ID (a raw hex hash, never
// meant to be memorized or spoken).
const pairingCodeAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

const pairingCodeLength = 6

// PairingCodeTTL bounds how long a registered code stays valid — long
// enough for a human to read it out, communicate it, and have the
// counterpart type it in, short enough that a brute-force guess against
// the ~2^30 (32^6) code space isn't practically feasible before it
// expires. Exported: cmd/spur's host-side flow also uses it to bound how
// long AwaitPairingCodeUse blocks.
const PairingCodeTTL = 10 * time.Minute

// RegisterPairingCode mints a short, human-typeable code for self and
// stores it server-side for PairingCodeTTL. Server-side use case, backed
// by port.PairingCodeStore.
type RegisterPairingCode struct {
	Store port.PairingCodeStore
}

func (uc RegisterPairingCode) Execute(ctx context.Context, self domain.PeerID) (string, error) {
	code, err := generatePairingCode()
	if err != nil {
		return "", err
	}
	if err := uc.Store.Register(ctx, code, self, PairingCodeTTL); err != nil {
		return "", err
	}
	return code, nil
}

// generatePairingCode draws pairingCodeLength bytes from crypto/rand and
// maps each to one alphabet symbol via modulo. This is unbiased, not just
// convenient: len(pairingCodeAlphabet) is 32, a divisor of 256, so every
// symbol gets exactly 8 of the 256 possible byte values — no rejection
// sampling needed, unlike alphabets whose length doesn't evenly divide
// 256.
func generatePairingCode() (string, error) {
	raw := make([]byte, pairingCodeLength)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("usecase: generate pairing code: %w", err)
	}
	code := make([]byte, pairingCodeLength)
	for i, b := range raw {
		code[i] = pairingCodeAlphabet[int(b)%len(pairingCodeAlphabet)]
	}
	return string(code), nil
}

// ResolvePairingCode looks up which peer ID a pairing code refers to, on
// behalf of guest (who is thereby recorded as the one connecting — see
// port.PairingCodeStore.Resolve). Server-side use case.
type ResolvePairingCode struct {
	Store port.PairingCodeStore
}

func (uc ResolvePairingCode) Execute(ctx context.Context, code string, guest domain.PeerID) (domain.PeerID, error) {
	return uc.Store.Resolve(ctx, code, guest)
}

// AwaitPairingCodeUse blocks until code has been resolved by a
// counterpart, returning that counterpart's peer ID. Server-side use
// case.
type AwaitPairingCodeUse struct {
	Store port.PairingCodeStore
}

func (uc AwaitPairingCodeUse) Execute(ctx context.Context, code string) (domain.PeerID, error) {
	return uc.Store.AwaitUse(ctx, code)
}
