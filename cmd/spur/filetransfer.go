package main

import (
	"context"
	"fmt"
	"os"

	"github.com/fu1se/spur/internal/adapter/cli"
	"github.com/fu1se/spur/internal/adapter/localfs"
	"github.com/fu1se/spur/internal/adapter/rendezvous"
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
// here. A network drop mid-transfer reconnects and retries (see
// rendezvous.RunPersistent); the receiving side's resume support means a
// retried transfer picks up from what already arrived, not from zero.
func send(ctx context.Context, serverAddr, stunAddr, counterpartID, roomName, identityPath, path string, onSelfID func(string), onProgress cli.ProgressFunc, onCode cli.OnCodeFunc, onVersionMismatch cli.VersionMismatchFunc, onReconnect cli.OnReconnectFunc) error {
	// Checked up front, before rendezvous: usecase.SendFiles only
	// discovers a bad local path (typo, doesn't exist, no permission)
	// after the full P2P handshake completes, which can take up to a
	// minute (NAT punching, possible relay fallback) -- and leaves the
	// receiving peer sitting in AcceptStream with no idea why it never
	// gets data. Failing fast here means a simple typo costs nothing.
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("app: %w", err)
	}

	resolve := rendezvous.CounterpartResolverFor(counterpartID, roomName, rendezvous.OnCodeFunc(onCode))
	return rendezvous.RunPersistent(ctx, serverAddr, stunAddr, identityPath, "", cli.Version(), resolve, onSelfID, rendezvous.VersionMismatchFunc(onVersionMismatch), rendezvous.OnReconnectFunc(onReconnect),
		func(ctx context.Context, tun *rendezvous.Tunnel) error {
			return usecase.SendFiles{Source: localfs.Source{Path: path}, Tunnel: tun.Conn, OnProgress: usecase.TransferProgress(onProgress)}.Run(ctx)
		})
}

// receive is "spur receive": accept whatever counterpart streams via
// "spur send" and write it under destDir, recreating the sender's relative
// directory structure. counterpartID, roomName, onCode: see send.
// onProgress: see send. onResumeOffer is forwarded into
// usecase.ReceiveFiles.OnResumeOffer — but only for the first attempt:
// when an automatic reconnect retries the transfer, the partial data on
// disk came from this very transfer moments ago, so re-asking the user
// "resume?" would be noise (and, worse, block the retry on a prompt
// nobody may be around to answer) — retries always resume.
func receive(ctx context.Context, serverAddr, stunAddr, counterpartID, roomName, identityPath, destDir string, onSelfID func(string), onProgress cli.ProgressFunc, onCode cli.OnCodeFunc, onResumeOffer cli.ResumeOfferFunc, onVersionMismatch cli.VersionMismatchFunc, onReconnect cli.OnReconnectFunc) error {
	resolve := rendezvous.CounterpartResolverFor(counterpartID, roomName, rendezvous.OnCodeFunc(onCode))

	attempt := 0
	return rendezvous.RunPersistent(ctx, serverAddr, stunAddr, identityPath, "", cli.Version(), resolve, onSelfID, rendezvous.VersionMismatchFunc(onVersionMismatch), rendezvous.OnReconnectFunc(onReconnect),
		func(ctx context.Context, tun *rendezvous.Tunnel) error {
			attempt++
			resume := usecase.ResumeOffer(onResumeOffer)
			if attempt > 1 {
				resume = func(int, int64, int64) bool { return true }
			}
			return usecase.ReceiveFiles{
				Sink:          localfs.Sink{DestDir: destDir},
				Tunnel:        tun.Conn,
				OnProgress:    usecase.TransferProgress(onProgress),
				OnResumeOffer: resume,
			}.Run(ctx)
		})
}
