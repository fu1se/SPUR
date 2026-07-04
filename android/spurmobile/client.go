package spurmobile

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/fu1se/spur/internal/adapter/controlclient"
	"github.com/fu1se/spur/internal/adapter/rendezvous"
	"github.com/fu1se/spur/internal/domain"
	"github.com/fu1se/spur/internal/infra"
)

// registerTimeout bounds how long Client.Register blocks before giving
// up — a UI caller needs a definite answer to show the user, unlike the
// desktop CLI which can afford to let context.Background() run until the
// process itself is interrupted.
const registerTimeout = 15 * time.Second

// Client holds the persistent state (identity, TOFU trust store) every
// control-plane operation needs, rooted at a single app-private
// directory the Android side supplies (typically Context.filesDir) —
// Go's os.UserConfigDir() doesn't resolve to anything writable inside an
// Android app's sandbox the way it does on desktop, so every path here
// comes from the caller instead of a guessed default. Mirrors the
// desktop CLI's ~/.config/spur layout (identity.key, known_servers.json)
// one level down, under configDir instead of os.UserConfigDir().
type Client struct {
	identityPath   string
	trustStorePath string
	selfID         string
}

// NewClient loads (or creates, on first run) the identity persisted
// under configDir. Pure local file I/O, no network access — same
// separation as the desktop CLI's "spur whoami" vs "spur register".
func NewClient(configDir string) (*Client, error) {
	identityPath := filepath.Join(configDir, "identity.key")
	id, err := infra.LoadOrCreateIdentity(identityPath)
	if err != nil {
		return nil, fmt.Errorf("spurmobile: load identity: %w", err)
	}
	return &Client{
		identityPath:   identityPath,
		trustStorePath: filepath.Join(configDir, "known_servers.json"),
		selfID:         string(domain.DerivePeerID(id.PublicKey)),
	}, nil
}

// SelfID is this client's persistent peer ID (see domain.DerivePeerID) —
// unchanged across calls to Register, since it's derived purely from the
// identity NewClient already loaded.
func (c *Client) SelfID() string { return c.selfID }

// Register dials serverAddr over a TOFU-pinned control-plane connection
// and registers this client's identity — a one-shot connectivity check,
// not a persistent connection (mirrors the desktop CLI's "spur
// register": dial, register, close). The server's cert fingerprint gets
// pinned into this Client's trust store on first success (see
// infra.TOFUClientTLSConfig's doc comment), the same protection the
// desktop CLI gets.
func (c *Client) Register(serverAddr string) error {
	client, _, err := dialAndRegister(serverAddr, c)
	if err != nil {
		return explain(err)
	}
	client.Close()
	return nil
}

// dialAndRegister is the shared one-shot dial+register step behind
// Register, CreateRoom and JoinRoom — all three need nothing more than a
// registered connection to issue one further RPC on.
func dialAndRegister(serverAddr string, c *Client) (*controlclient.Client, infra.Identity, error) {
	ctx, cancel := context.WithTimeout(context.Background(), registerTimeout)
	defer cancel()
	return rendezvous.DialAndRegister(ctx, serverAddr, c.identityPath, c.trustStorePath, Version(), nil)
}
