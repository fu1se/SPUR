package guiapp

import (
	"context"
	"crypto/rand"
	"fmt"

	"github.com/fu1se/spur/internal/adapter/cli"
	"github.com/fu1se/spur/internal/adapter/controlclient"
	"github.com/fu1se/spur/internal/adapter/rendezvous"
	"github.com/fu1se/spur/internal/domain"
	"github.com/fu1se/spur/internal/infra"
)

// Register is a one-shot control-plane reachability check — mirrors
// cmd/spur's "spur register": dial serverAddr, register a throwaway
// ephemeral key (not the persisted identity NewClient loaded — same as
// the CLI command of the same name, kept distinct from the identity used
// by Connect/Expose/Send/Receive/JoinMesh, which always goes through
// rendezvous.Establish's own persisted-identity load), and report what
// the server saw. Useful in a GUI purely as a "is this server address
// reachable at all" button before attempting a real rendezvous.
//
// Returns a plain error, same as cmd/spur's functions — unlike
// spurmobile, this package crosses no process/JNI boundary, so there's
// no need to pre-format the message with cli.Explain here: cmd/spur-gui
// calls cli.Explain(err) itself at display time, exactly like cmd/spur's
// main.go does after root.ExecuteContext.
func (c *Client) Register(ctx context.Context, serverAddr string, onVersionMismatch cli.VersionMismatchFunc) (cli.RegisterResult, error) {
	var pub domain.PublicKey
	if _, err := rand.Read(pub[:]); err != nil {
		return cli.RegisterResult{}, fmt.Errorf("guiapp: generate ephemeral key: %w", err)
	}

	tlsConf, err := rendezvous.ControlClientTLS(serverAddr, "")
	if err != nil {
		return cli.RegisterResult{}, err
	}

	client, err := controlclient.Dial(ctx, serverAddr, tlsConf, infra.DefaultQUICConfig())
	if err != nil {
		return cli.RegisterResult{}, err
	}
	defer client.Close()

	result, err := client.Register(ctx, pub, cli.Version())
	if err != nil {
		return cli.RegisterResult{}, err
	}
	rendezvous.WarnIfVersionMismatch(cli.Version(), result.ServerVersion, rendezvous.VersionMismatchFunc(onVersionMismatch))

	return cli.RegisterResult{
		PeerID:          string(result.PeerID),
		ObservedAddress: result.ObservedAddress,
	}, nil
}
