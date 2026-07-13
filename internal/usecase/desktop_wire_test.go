package usecase_test

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/spur/internal/usecase"
	"github.com/fu1se/spur/internal/usecase/port"
)

// offerTunnel adapts one net.Pipe end as a single-stream TunnelConn,
// like send_receive_files_test.go's fakeTunnelConn.
type offerTunnel struct{ stream net.Conn }

func (f offerTunnel) OpenStream(context.Context) (port.Stream, error)   { return f.stream, nil }
func (f offerTunnel) AcceptStream(context.Context) (port.Stream, error) { return f.stream, nil }
func (f offerTunnel) Close() error                                      { return nil }

func TestDesktopOffer_RoundTrip(t *testing.T) {
	shareEnd, viewEnd := net.Pipe()

	sent := usecase.DesktopOffer{Protocol: "vnc", Password: "s3cr3t42", ViewOnly: true}
	sendErr := make(chan error, 1)
	go func() {
		sendErr <- usecase.SendDesktopOffer(context.Background(), offerTunnel{stream: shareEnd}, sent)
	}()

	got, err := usecase.ReceiveDesktopOffer(context.Background(), offerTunnel{stream: viewEnd})
	require.NoError(t, err)
	require.NoError(t, <-sendErr)
	require.Equal(t, sent, got)
}

func TestReceiveDesktopOffer_RejectsGarbage(t *testing.T) {
	shareEnd, viewEnd := net.Pipe()

	go func() {
		_, _ = shareEnd.Write([]byte("definitely not json"))
		shareEnd.Close()
	}()

	_, err := usecase.ReceiveDesktopOffer(context.Background(), offerTunnel{stream: viewEnd})
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "decode desktop offer"))
}
