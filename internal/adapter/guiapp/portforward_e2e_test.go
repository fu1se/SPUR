package guiapp_test

import (
	"context"
	"io"
	"net"
	"net/netip"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/spur/internal/adapter/controlproto"
	"github.com/fu1se/spur/internal/adapter/controlserver"
	"github.com/fu1se/spur/internal/adapter/guiapp"
	"github.com/fu1se/spur/internal/adapter/repository/memory"
	"github.com/fu1se/spur/internal/adapter/stunserver"
	"github.com/fu1se/spur/internal/infra"
	"github.com/fu1se/spur/internal/usecase"
)

// TestGuiapp_ConnectExposeEndToEnd drives guiapp's async Client.StartConnect/
// StartExpose facade — the same one cmd/spur-gui calls from a background
// goroutine — the same way the desktop CLI's own end-to-end test drives
// cmd/spur's blocking connect/expose functions (see
// internal/adapter/tunnel/portforward_e2e_test.go). What's actually new
// here isn't the rendezvous/tunnel machinery itself (already covered
// there) but guiapp's own layer on top of it: that StartConnect/
// StartExpose return a working PortForward handle whose Stop/Wait
// actually control the background goroutine, not just that a raw
// usecase.ForwardPort call succeeds.
func TestGuiapp_ConnectExposeEndToEnd(t *testing.T) {
	// os.UserConfigDir() (used for the TOFU trust store, since guiapp
	// always passes "" for trustStorePath — see mesh.go/portforward.go's
	// doc comments) resolves from $HOME; isolate it so this test never
	// touches the real ~/.config/spur/known_servers.json, mirroring how
	// two real "spur connect"/"spur expose" OS processes already share
	// one trust store file for the same server (see CLAUDE.md's TOFU
	// section on the concurrent-writer race that was fixed there).
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	controlAddr := startTestControlServer(t, ctx)
	stunAddr := startTestSTUNServer(t, ctx)
	echoAddr := startTestEchoServer(t)

	dir := t.TempDir()
	connector, err := guiapp.NewClient(filepath.Join(dir, "connector.key"))
	require.NoError(t, err)
	exposer, err := guiapp.NewClient(filepath.Join(dir, "exposer.key"))
	require.NoError(t, err)

	// Host side: exposer registers a pairing code and waits for it — this
	// blocks until the connector resolves the code below, so (just like a
	// real GUI, which must call this from a background goroutine rather
	// than its event-dispatch thread) it has to run concurrently with the
	// rest of this test, not before it.
	codeCh := make(chan string, 1)
	type exposeResult struct {
		pf  *guiapp.PortForward
		err error
	}
	exposeResultCh := make(chan exposeResult, 1)
	go func() {
		pf, err := exposer.StartExpose(ctx, controlAddr, stunAddr.String(), "", "", echoPortOf(t, echoAddr), func(string) {}, func(code string) { codeCh <- code }, nil)
		exposeResultCh <- exposeResult{pf, err}
	}()

	code := <-codeCh
	require.NotEmpty(t, code)

	connectPF, err := connector.StartConnect(ctx, controlAddr, stunAddr.String(), code, "", 0, func(string) {}, nil, nil)
	require.NoError(t, err)
	defer connectPF.Stop()

	exposed := <-exposeResultCh
	require.NoError(t, exposed.err)
	defer exposed.pf.Stop()

	conn, err := net.Dial("tcp", connectPF.LocalAddr)
	require.NoError(t, err)
	defer conn.Close()

	const msg = "hello through guiapp"
	_, err = conn.Write([]byte(msg))
	require.NoError(t, err)
	buf := make([]byte, len(msg))
	_, err = io.ReadFull(conn, buf)
	require.NoError(t, err)
	require.Equal(t, msg, string(buf))

	// Stop causes the run loop to observe its context cancelled, so Wait
	// reports context.Canceled here, not nil — see PortForward.Wait's doc
	// comment.
	connectPF.Stop()
	require.ErrorIs(t, connectPF.Wait(), context.Canceled)
	exposed.pf.Stop()
	_ = exposed.pf.Wait()
}

func echoPortOf(t *testing.T, addr string) int {
	t.Helper()
	_, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)
	return port
}

func startTestControlServer(t *testing.T, ctx context.Context) string {
	t.Helper()

	tlsConf, err := infra.SelfSignedServerTLSConfig(controlproto.ALPN)
	require.NoError(t, err)

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)

	peers := memory.NewPeerRepository()
	candidateBroker := memory.NewCandidateBroker()
	relayBroker := memory.NewRelayBroker()
	pairingBroker := memory.NewPairingCodeBroker()

	srv := &controlserver.Server{
		RegisterPeer:        usecase.RegisterPeer{Peers: peers},
		PublishCandidates:   usecase.PublishCandidates{Store: candidateBroker},
		AwaitCandidates:     usecase.AwaitCandidates{Store: candidateBroker},
		RelayFallback:       usecase.RelayFallback{Broker: relayBroker},
		RegisterPairingCode: usecase.RegisterPairingCode{Store: pairingBroker},
		ResolvePairingCode:  usecase.ResolvePairingCode{Store: pairingBroker},
		AwaitPairingCodeUse: usecase.AwaitPairingCodeUse{Store: pairingBroker},
	}

	go func() { _ = srv.Serve(ctx, conn, tlsConf, infra.DefaultQUICConfig()) }()

	return conn.LocalAddr().String()
}

func startTestSTUNServer(t *testing.T, ctx context.Context) netip.AddrPort {
	t.Helper()

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	require.NoError(t, err)

	addr := conn.LocalAddr().(*net.UDPAddr).AddrPort() //nolint:forcetypeassert // net.ListenUDP always returns *net.UDPAddr

	go func() { _ = stunserver.Serve(ctx, conn) }()

	return addr
}

func startTestEchoServer(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}()
		}
	}()

	return ln.Addr().String()
}
