// Package controlclient is the client-side control-plane adapter: it opens
// a QUIC connection to the rendezvous server and speaks the control
// protocol over it.
package controlclient

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/quic-go/quic-go"

	"github.com/fu1se/spur/internal/adapter/controlproto"
	"github.com/fu1se/spur/internal/domain"
)

// Client is a control-plane connection to a rendezvous server.
type Client struct {
	conn *quic.Conn
}

// Dial establishes the QUIC control connection.
func Dial(ctx context.Context, addr string, tlsConf *tls.Config, quicConf *quic.Config) (*Client, error) {
	conn, err := quic.DialAddr(ctx, addr, tlsConf, quicConf)
	if err != nil {
		return nil, fmt.Errorf("controlclient: dial: %w", err)
	}
	return &Client{conn: conn}, nil
}

// Close tears down the control connection.
func (c *Client) Close() error {
	return c.conn.CloseWithError(0, "")
}

// RegisterResult is what the server tells the client about itself.
type RegisterResult struct {
	PeerID          domain.PeerID
	ObservedAddress string
	// ServerVersion is the server's own build version — empty if the
	// server didn't set one (an old build predating this field, or a
	// composition root that left controlserver.Server.Version unset).
	ServerVersion string
}

// Register announces pub (and this client's own build version, purely
// informational — see RegisterRequest.client_version) to the server and
// returns the peer ID the server assigned, the address the server
// observed the client at (the server-reflexive candidate), and the
// server's own build version for the caller to compare against its own.
func (c *Client) Register(ctx context.Context, pub domain.PublicKey, clientVersion string) (RegisterResult, error) {
	stream, err := c.conn.OpenStreamSync(ctx)
	if err != nil {
		return RegisterResult{}, fmt.Errorf("controlclient: open stream: %w", err)
	}
	defer stream.Close()

	if err := controlproto.WriteMethod(stream, controlproto.MethodRegister); err != nil {
		return RegisterResult{}, err
	}
	if err := controlproto.WriteFrame(stream, &controlproto.RegisterRequest{
		PublicKey:     pub[:],
		ClientVersion: clientVersion,
	}); err != nil {
		return RegisterResult{}, err
	}

	var resp controlproto.RegisterResponse
	if err := controlproto.ReadFrame(stream, &resp); err != nil {
		return RegisterResult{}, err
	}

	return RegisterResult{
		PeerID:          domain.PeerID(resp.PeerId),
		ObservedAddress: resp.ObservedAddress,
		ServerVersion:   resp.ServerVersion,
	}, nil
}
