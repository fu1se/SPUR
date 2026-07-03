package main

import (
	"context"
	"fmt"
	"os"

	"github.com/fu1se/spur/internal/adapter/cli"
	"github.com/fu1se/spur/internal/adapter/localfs"
	"github.com/fu1se/spur/internal/usecase"
)

// send is "spur send": stream path (a file, or a directory walked
// recursively) to counterpart, who must be running "spur receive" against
// the same peer ID (or, with counterpartID empty, register a pairing
// code and wait for "spur receive <code>" instead — see
// counterpartResolverFor). roomName, if non-empty, resolves the
// counterpart via a persistent room instead. onProgress is forwarded
// straight into usecase.SendFiles.OnProgress — see cli.ProgressFunc's
// doc comment for why the rendering itself lives in the cli package, not
// here.
func send(ctx context.Context, serverAddr, stunAddr, counterpartID, roomName, identityPath, path string, onSelfID func(string), onProgress cli.ProgressFunc, onCode cli.OnCodeFunc, onVersionMismatch cli.VersionMismatchFunc) error {
	// Checked up front, before rendezvous: usecase.SendFiles only
	// discovers a bad local path (typo, doesn't exist, no permission)
	// after the full P2P handshake completes, which can take up to a
	// minute (NAT punching, possible relay fallback) -- and leaves the
	// receiving peer sitting in AcceptStream with no idea why it never
	// gets data. Failing fast here means a simple typo costs nothing.
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("app: %w", err)
	}

	tun, _, _, err := rendezvous(ctx, serverAddr, stunAddr, identityPath, counterpartResolverFor(counterpartID, roomName, onCode), onSelfID, onVersionMismatch)
	if err != nil {
		return err
	}
	defer tun.Close()

	return usecase.SendFiles{Source: localfs.Source{Path: path}, Tunnel: tun.conn, OnProgress: usecase.TransferProgress(onProgress)}.Run(ctx)
}

// receive is "spur receive": accept whatever counterpart streams via
// "spur send" and write it under destDir, recreating the sender's relative
// directory structure. counterpartID, roomName, onCode: see send.
// onProgress: see send. onResumeOffer is forwarded into
// usecase.ReceiveFiles.OnResumeOffer — see cli.ResumeOfferFunc's doc
// comment.
func receive(ctx context.Context, serverAddr, stunAddr, counterpartID, roomName, identityPath, destDir string, onSelfID func(string), onProgress cli.ProgressFunc, onCode cli.OnCodeFunc, onResumeOffer cli.ResumeOfferFunc, onVersionMismatch cli.VersionMismatchFunc) error {
	tun, _, _, err := rendezvous(ctx, serverAddr, stunAddr, identityPath, counterpartResolverFor(counterpartID, roomName, onCode), onSelfID, onVersionMismatch)
	if err != nil {
		return err
	}
	defer tun.Close()

	return usecase.ReceiveFiles{
		Sink:          localfs.Sink{DestDir: destDir},
		Tunnel:        tun.conn,
		OnProgress:    usecase.TransferProgress(onProgress),
		OnResumeOffer: usecase.ResumeOffer(onResumeOffer),
	}.Run(ctx)
}
