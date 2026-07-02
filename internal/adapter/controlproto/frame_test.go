package controlproto_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/spur/internal/adapter/controlproto"
)

func TestWriteReadFrame_RoundTrip(t *testing.T) {
	var buf bytes.Buffer
	req := &controlproto.RegisterRequest{PublicKey: []byte("hello")}
	require.NoError(t, controlproto.WriteFrame(&buf, req))

	var got controlproto.RegisterRequest
	require.NoError(t, controlproto.ReadFrame(&buf, &got))
	require.Equal(t, req.GetPublicKey(), got.GetPublicKey())
}

// TestWriteFrame_RejectsOversizedMessage guards against the asymmetry
// found in a security audit: ReadFrame already rejected any frame over
// maxFrameSize, but WriteFrame didn't check the same bound before writing
// -- so a message that grew past the limit (e.g. a JoinNetworkResponse
// listing enough mesh members) would be written successfully and only
// fail confusingly on the reading side.
func TestWriteFrame_RejectsOversizedMessage(t *testing.T) {
	var buf bytes.Buffer
	req := &controlproto.JoinNetworkRequest{NetworkName: strings.Repeat("x", 70*1024)}

	err := controlproto.WriteFrame(&buf, req)
	require.Error(t, err)
	require.Zero(t, buf.Len(), "nothing should be written to the wire once the size check fails")
}
