package usecase

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/spur/internal/usecase/port"
)

func TestReadFileHeader_RoundTrip(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeFileHeader(&buf, port.FileEntry{RelPath: "a.txt", Size: 5}))

	entry, end, err := readFileHeader(&buf)
	require.NoError(t, err)
	require.False(t, end)
	require.Equal(t, "a.txt", entry.RelPath)
	require.Equal(t, int64(5), entry.Size)
}

// TestReadFileHeader_RejectsSizeOverflowingInt64 guards against the wire
// parser desync found in a security audit: an unchecked uint64->int64
// cast on the declared size let a value above math.MaxInt64 become
// negative, and the receiver's io.CopyN treated a negative byte count as
// "already done" instead of erroring -- consuming zero bytes of what the
// sender actually wrote as file content and desyncing every header read
// after it into garbage. readFileHeader must reject it outright instead.
func TestReadFileHeader_RejectsSizeOverflowingInt64(t *testing.T) {
	var buf bytes.Buffer
	pathBytes := []byte("evil.txt")
	require.NoError(t, binary.Write(&buf, binary.BigEndian, uint16(len(pathBytes))))
	_, err := buf.Write(pathBytes)
	require.NoError(t, err)
	require.NoError(t, binary.Write(&buf, binary.BigEndian, uint64(math.MaxInt64)+1))

	_, _, err = readFileHeader(&buf)
	require.Error(t, err)
}
