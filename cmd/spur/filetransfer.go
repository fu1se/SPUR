package main

import (
	"context"
	"fmt"
	"os"

	"github.com/fu1se/spur/internal/adapter/localfs"
	"github.com/fu1se/spur/internal/domain"
	"github.com/fu1se/spur/internal/usecase"
)

// send is "spur send": stream path (a file, or a directory walked
// recursively) to counterpart, who must be running "spur receive" against
// the same peer ID.
func send(ctx context.Context, serverAddr, stunAddr, counterpartID, identityPath, path string, onSelfID func(string)) error {
	// Checked up front, before rendezvous: usecase.SendFiles only
	// discovers a bad local path (typo, doesn't exist, no permission)
	// after the full P2P handshake completes, which can take up to a
	// minute (NAT punching, possible relay fallback) -- and leaves the
	// receiving peer sitting in AcceptStream with no idea why it never
	// gets data. Failing fast here means a simple typo costs nothing.
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("app: %w", err)
	}

	tun, _, err := rendezvous(ctx, serverAddr, stunAddr, identityPath, domain.PeerID(counterpartID), onSelfID)
	if err != nil {
		return err
	}
	defer tun.Close()

	return usecase.SendFiles{Source: localfs.Source{Path: path}, Tunnel: tun.conn}.Run(ctx)
}

// receive is "spur receive": accept whatever counterpart streams via
// "spur send" and write it under destDir, recreating the sender's relative
// directory structure.
func receive(ctx context.Context, serverAddr, stunAddr, counterpartID, identityPath, destDir string, onSelfID func(string)) error {
	tun, _, err := rendezvous(ctx, serverAddr, stunAddr, identityPath, domain.PeerID(counterpartID), onSelfID)
	if err != nil {
		return err
	}
	defer tun.Close()

	return usecase.ReceiveFiles{Sink: localfs.Sink{DestDir: destDir}, Tunnel: tun.conn}.Run(ctx)
}
