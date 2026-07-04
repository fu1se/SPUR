package guiapp

import (
	"context"

	"github.com/fu1se/spur/internal/adapter/cli"
	"github.com/fu1se/spur/internal/adapter/localfs"
	"github.com/fu1se/spur/internal/adapter/rendezvous"
	"github.com/fu1se/spur/internal/usecase"
)

// Transfer is a running "send"/"receive" session (see
// Client.StartSend/StartReceive) — same shape as PortForward: already
// established by the time a caller gets one back, runs to completion (or
// Stop) in a background goroutine, Wait reports how it ended.
type Transfer struct {
	cancel context.CancelFunc
	done   chan error
}

// Stop aborts the transfer. Safe to call more than once.
func (t *Transfer) Stop() { t.cancel() }

// Wait blocks until the transfer ends, returning the reason: nil when
// SendFiles/ReceiveFiles.Run finishes the transfer on its own, or
// context.Canceled when Stop cut it short — see PortForward.Wait's doc
// comment for the same distinction.
func (t *Transfer) Wait() error { return <-t.done }

// StartSend is "spur send": streams path (a file, or a directory walked
// recursively) to whichever counterpart to/room resolves to — see
// StartConnect for the to/room/onCode contract, identical here.
// onProgress is forwarded straight into usecase.SendFiles.OnProgress.
// Blocks until the tunnel is established before returning; call from a
// background goroutine.
func (c *Client) StartSend(ctx context.Context, serverAddr, stunAddr, to, room, path string, onSelfID func(string), onProgress cli.ProgressFunc, onCode cli.OnCodeFunc, onVersionMismatch cli.VersionMismatchFunc) (*Transfer, error) {
	resolve := rendezvous.CounterpartResolverFor(to, room, rendezvous.OnCodeFunc(onCode))
	tun, _, _, err := rendezvous.Establish(ctx, serverAddr, stunAddr, c.identityPath, "", cli.Version(), resolve, onSelfID, rendezvous.VersionMismatchFunc(onVersionMismatch))
	if err != nil {
		return nil, err
	}

	runCtx, cancel := context.WithCancel(context.Background())
	tr := &Transfer{cancel: cancel, done: make(chan error, 1)}
	go func() {
		runErr := usecase.SendFiles{
			Source:     localfs.Source{Path: path},
			Tunnel:     tun.Conn,
			OnProgress: usecase.TransferProgress(onProgress),
		}.Run(runCtx)
		tun.Close()
		tr.done <- runErr
	}()
	return tr, nil
}

// StartReceive is "spur receive": accepts whatever the resolved
// counterpart streams via StartSend and writes it under destDir,
// recreating the sender's relative directory structure. See StartSend
// for to/room/onCode/onProgress; onResumeOffer is asked once, before any
// data arrives, whether to resume files destDir already has partial data
// for (nil behaves like always starting fresh).
func (c *Client) StartReceive(ctx context.Context, serverAddr, stunAddr, to, room, destDir string, onSelfID func(string), onProgress cli.ProgressFunc, onCode cli.OnCodeFunc, onResumeOffer cli.ResumeOfferFunc, onVersionMismatch cli.VersionMismatchFunc) (*Transfer, error) {
	resolve := rendezvous.CounterpartResolverFor(to, room, rendezvous.OnCodeFunc(onCode))
	tun, _, _, err := rendezvous.Establish(ctx, serverAddr, stunAddr, c.identityPath, "", cli.Version(), resolve, onSelfID, rendezvous.VersionMismatchFunc(onVersionMismatch))
	if err != nil {
		return nil, err
	}

	runCtx, cancel := context.WithCancel(context.Background())
	tr := &Transfer{cancel: cancel, done: make(chan error, 1)}
	go func() {
		runErr := usecase.ReceiveFiles{
			Sink:          localfs.Sink{DestDir: destDir},
			Tunnel:        tun.Conn,
			OnProgress:    usecase.TransferProgress(onProgress),
			OnResumeOffer: usecase.ResumeOffer(onResumeOffer),
		}.Run(runCtx)
		tun.Close()
		tr.done <- runErr
	}()
	return tr, nil
}
