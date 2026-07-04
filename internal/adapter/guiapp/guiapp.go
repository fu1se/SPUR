// Package guiapp is the desktop GUI's equivalent of android/spurmobile: a
// facade over the same shared client-side packages (rendezvous, usecase,
// meshclient, wgmesh, localfs) that cmd/spur's CLI commands call directly,
// reshaped for a UI that needs non-blocking Start/Stop/Await handles
// instead of a function that blocks the calling goroutine until Ctrl+C.
//
// Unlike spurmobile, this package has no gomobile constraint forcing it
// out of internal/ (see CLAUDE.md's "Android-клиент" section for why
// spurmobile is special) — cmd/spur-gui is a normal Go binary, so this is
// a normal internal/adapter package. It still mirrors spurmobile's shape
// closely (Client, PortForward, Transfer, MeshSession) because the
// underlying problem is identical: a GUI event loop, like a mobile
// runtime, can't block on rendezvous.Establish or a mesh join loop on its
// main thread.
//
// Where spurmobile has to invent its own mirror types (MobileReader,
// FileSource/FileSink over SAF, single-method callback interfaces) to
// cross the gomobile/JNI boundary, this package doesn't: it runs in the
// same process and address space as cmd/spur-gui, so it uses real
// os/filepath I/O via internal/adapter/localfs and real TUN creation via
// wgmesh.NewDevice, exactly like cmd/spur does — and it freely reuses
// internal/adapter/cli's plain func callback types (OnCodeFunc,
// ProgressFunc, VersionMismatchFunc, ResumeOfferFunc) and result structs
// (RegisterResult, RoomResult, JoinNetworkResult) instead of redefining
// mirrors of them: cli.Explain and these types carry no cobra dependency,
// and spurmobile already sets the precedent of importing cli for exactly
// this reason (see spurmobile's explain.go).
package guiapp

import (
	"github.com/fu1se/spur/internal/adapter/rendezvous"
	"github.com/fu1se/spur/internal/domain"
	"github.com/fu1se/spur/internal/infra"
)

// Client holds the persistent state (identity path) every operation
// needs. identityPath == "" resolves to the same default the desktop CLI
// uses (see rendezvous.ResolveIdentityPath, infra.DefaultIdentityPath) —
// a GUI running as the same user as the CLI shares its identity/known
// servers automatically, which is desirable: "spur whoami" on the
// command line and the GUI's identity panel are the same peer.
type Client struct {
	identityPath string
	selfID       string
}

// NewClient loads (or creates, on first run) the identity persisted at
// identityPath ("" for the default path). Pure local file I/O, no network
// access — same separation as the desktop CLI's "spur whoami".
func NewClient(identityPath string) (*Client, error) {
	resolvedPath, err := rendezvous.ResolveIdentityPath(identityPath)
	if err != nil {
		return nil, err
	}
	id, err := infra.LoadOrCreateIdentity(resolvedPath)
	if err != nil {
		return nil, err
	}
	return &Client{
		identityPath: identityPath,
		selfID:       string(domain.DerivePeerID(id.PublicKey)),
	}, nil
}

// SelfID is this client's persistent peer ID — unchanged across calls,
// since it's derived purely from the identity NewClient already loaded.
func (c *Client) SelfID() string { return c.selfID }
