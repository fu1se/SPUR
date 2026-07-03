package controlclient

import (
	"context"
	"errors"
	"fmt"

	"github.com/fu1se/spur/internal/adapter/controlproto"
	"github.com/fu1se/spur/internal/domain"
)

// RegisterPairingCode asks the server to mint a short code that resolves
// to self's own peer ID (derived server-side from pub) for a limited
// time.
func (c *Client) RegisterPairingCode(ctx context.Context, pub domain.PublicKey) (string, error) {
	stream, err := c.conn.OpenStreamSync(ctx)
	if err != nil {
		return "", fmt.Errorf("controlclient: open stream: %w", err)
	}
	defer stream.Close()

	if err := controlproto.WriteMethod(stream, controlproto.MethodRegisterPairingCode); err != nil {
		return "", err
	}
	if err := controlproto.WriteFrame(stream, &controlproto.RegisterPairingCodeRequest{
		PublicKey: pub[:],
	}); err != nil {
		return "", err
	}

	var resp controlproto.RegisterPairingCodeResponse
	if err := controlproto.ReadFrame(stream, &resp); err != nil {
		return "", err
	}
	return resp.GetCode(), nil
}

// ResolvePairingCode looks up which peer ID code refers to, identifying
// the caller (via pub) as the one connecting — wakes up the code's
// registrant, who is blocked in AwaitPairingCodeUse.
func (c *Client) ResolvePairingCode(ctx context.Context, code string, pub domain.PublicKey) (domain.PeerID, error) {
	stream, err := c.conn.OpenStreamSync(ctx)
	if err != nil {
		return "", fmt.Errorf("controlclient: open stream: %w", err)
	}
	defer stream.Close()

	if err := controlproto.WriteMethod(stream, controlproto.MethodResolvePairingCode); err != nil {
		return "", err
	}
	if err := controlproto.WriteFrame(stream, &controlproto.ResolvePairingCodeRequest{
		Code:      code,
		PublicKey: pub[:],
	}); err != nil {
		return "", err
	}

	var resp controlproto.ResolvePairingCodeResponse
	if err := controlproto.ReadFrame(stream, &resp); err != nil {
		return "", err
	}
	if resp.GetError() != "" {
		return "", errors.New(resp.GetError())
	}
	return domain.PeerID(resp.GetPeerId()), nil
}

// AwaitPairingCodeUse blocks until code has been resolved by a
// counterpart, returning that counterpart's peer ID, or ctx is done.
func (c *Client) AwaitPairingCodeUse(ctx context.Context, code string) (domain.PeerID, error) {
	stream, err := c.conn.OpenStreamSync(ctx)
	if err != nil {
		return "", fmt.Errorf("controlclient: open stream: %w", err)
	}
	defer stream.Close()

	if err := controlproto.WriteMethod(stream, controlproto.MethodAwaitPairingCodeUse); err != nil {
		return "", err
	}
	if err := controlproto.WriteFrame(stream, &controlproto.AwaitPairingCodeUseRequest{
		Code: code,
	}); err != nil {
		return "", err
	}

	var resp controlproto.AwaitPairingCodeUseResponse
	if err := controlproto.ReadFrame(stream, &resp); err != nil {
		return "", err
	}
	return domain.PeerID(resp.GetPeerId()), nil
}
