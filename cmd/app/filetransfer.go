package main

import (
	"context"

	"github.com/fu1se/localizator/internal/adapter/localfs"
	"github.com/fu1se/localizator/internal/domain"
	"github.com/fu1se/localizator/internal/usecase"
)

// send is "app send": stream path (a file, or a directory walked
// recursively) to counterpart, who must be running "app receive" against
// the same peer ID.
func send(ctx context.Context, serverAddr, stunAddr, counterpartID, identityPath, path string, onSelfID func(string)) error {
	tun, _, err := rendezvous(ctx, serverAddr, stunAddr, identityPath, domain.PeerID(counterpartID), onSelfID)
	if err != nil {
		return err
	}
	defer tun.Close()

	return usecase.SendFiles{Source: localfs.Source{Path: path}, Tunnel: tun.conn}.Run(ctx)
}

// receive is "app receive": accept whatever counterpart streams via
// "app send" and write it under destDir, recreating the sender's relative
// directory structure.
func receive(ctx context.Context, serverAddr, stunAddr, counterpartID, identityPath, destDir string, onSelfID func(string)) error {
	tun, _, err := rendezvous(ctx, serverAddr, stunAddr, identityPath, domain.PeerID(counterpartID), onSelfID)
	if err != nil {
		return err
	}
	defer tun.Close()

	return usecase.ReceiveFiles{Sink: localfs.Sink{DestDir: destDir}, Tunnel: tun.conn}.Run(ctx)
}
