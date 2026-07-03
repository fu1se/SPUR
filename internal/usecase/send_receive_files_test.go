package usecase_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/spur/internal/usecase"
	"github.com/fu1se/spur/internal/usecase/port"
)

// fakeTunnelConn wraps a single pre-established net.Conn as a
// port.TunnelConn with exactly one stream — enough for SendFiles/
// ReceiveFiles, which only ever open/accept one stream per transfer.
type fakeTunnelConn struct {
	stream port.Stream
}

func (f fakeTunnelConn) OpenStream(context.Context) (port.Stream, error)   { return f.stream, nil }
func (f fakeTunnelConn) AcceptStream(context.Context) (port.Stream, error) { return f.stream, nil }
func (f fakeTunnelConn) Close() error                                      { return nil }

// memFileSource is an in-memory port.FileSource fake.
type memFileSource struct {
	files map[string][]byte // relPath -> content
	order []string
}

func newMemFileSource(files map[string][]byte, order []string) *memFileSource {
	return &memFileSource{files: files, order: order}
}

func (s *memFileSource) List() ([]port.FileEntry, error) {
	entries := make([]port.FileEntry, 0, len(s.order))
	for _, relPath := range s.order {
		entries = append(entries, port.FileEntry{RelPath: relPath, Size: int64(len(s.files[relPath]))})
	}
	return entries, nil
}

func (s *memFileSource) Open(relPath string) (io.ReadCloser, error) {
	content, ok := s.files[relPath]
	if !ok {
		return nil, errors.New("memFileSource: no such file")
	}
	return io.NopCloser(bytes.NewReader(content)), nil
}

// memFileSink is an in-memory port.FileSink fake.
type memFileSink struct {
	files map[string][]byte
}

func newMemFileSink() *memFileSink {
	return &memFileSink{files: make(map[string][]byte)}
}

type memWriteCloser struct {
	sink    *memFileSink
	relPath string
	buf     bytes.Buffer
}

func (w *memWriteCloser) Write(p []byte) (int, error) { return w.buf.Write(p) }
func (w *memWriteCloser) Close() error {
	w.sink.files[w.relPath] = w.buf.Bytes()
	return nil
}

func (s *memFileSink) Create(entry port.FileEntry) (io.WriteCloser, error) {
	return &memWriteCloser{sink: s, relPath: entry.RelPath}, nil
}

func TestSendReceiveFiles_SingleFileRoundTrip(t *testing.T) {
	senderConn, receiverConn := net.Pipe()
	defer senderConn.Close()
	defer receiverConn.Close()

	source := newMemFileSource(map[string][]byte{
		"hello.txt": []byte("hello, world"),
	}, []string{"hello.txt"})
	sink := newMemFileSink()

	errCh := make(chan error, 2)
	go func() {
		errCh <- usecase.SendFiles{Source: source, Tunnel: fakeTunnelConn{stream: senderConn}}.Run(context.Background())
	}()
	go func() {
		errCh <- usecase.ReceiveFiles{Sink: sink, Tunnel: fakeTunnelConn{stream: receiverConn}}.Run(context.Background())
	}()

	require.NoError(t, <-errCh)
	require.NoError(t, <-errCh)

	require.Equal(t, []byte("hello, world"), sink.files["hello.txt"])
}

func TestSendReceiveFiles_MultipleFilesPreserveRelativePaths(t *testing.T) {
	senderConn, receiverConn := net.Pipe()
	defer senderConn.Close()
	defer receiverConn.Close()

	files := map[string][]byte{
		"a.txt":        []byte("a"),
		"dir/b.txt":    []byte("bbb"),
		"dir/sub/c.md": []byte(""),
	}
	order := []string{"a.txt", "dir/b.txt", "dir/sub/c.md"}
	source := newMemFileSource(files, order)
	sink := newMemFileSink()

	errCh := make(chan error, 2)
	go func() {
		errCh <- usecase.SendFiles{Source: source, Tunnel: fakeTunnelConn{stream: senderConn}}.Run(context.Background())
	}()
	go func() {
		errCh <- usecase.ReceiveFiles{Sink: sink, Tunnel: fakeTunnelConn{stream: receiverConn}}.Run(context.Background())
	}()

	require.NoError(t, <-errCh)
	require.NoError(t, <-errCh)

	require.Len(t, sink.files, len(files))
	for relPath, content := range files {
		require.True(t, bytes.Equal(content, sink.files[relPath]), "mismatch for %s", relPath)
	}
}

func TestSendFiles_SourceListErrorPropagates(t *testing.T) {
	senderConn, receiverConn := net.Pipe()
	defer senderConn.Close()
	defer receiverConn.Close()

	err := usecase.SendFiles{Source: failingSource{}, Tunnel: fakeTunnelConn{stream: senderConn}}.Run(context.Background())
	require.Error(t, err)
}

type failingSource struct{}

func (failingSource) List() ([]port.FileEntry, error)    { return nil, errors.New("boom") }
func (failingSource) Open(string) (io.ReadCloser, error) { return nil, errors.New("boom") }

// progressCall is one recorded invocation of usecase.TransferProgress.
type progressCall struct {
	relPath                   string
	fileDone, fileTotal       int64
	overallDone, overallTotal int64
}

// recordingProgress is safe to call from a different goroutine than the
// one reading Calls — SendFiles/ReceiveFiles run in their own goroutines
// in these tests, same as the real rendezvous()/send()/receive() split.
type recordingProgress struct {
	mu    sync.Mutex
	calls []progressCall
}

func (r *recordingProgress) record(relPath string, fileDone, fileTotal, overallDone, overallTotal int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, progressCall{relPath, fileDone, fileTotal, overallDone, overallTotal})
}

func (r *recordingProgress) snapshot() []progressCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]progressCall(nil), r.calls...)
}

// TestSendReceiveFiles_ReportsProgress drives a transfer big enough to
// span several chunks (see usecase.progressChunkSize) and checks both
// sides' OnProgress callbacks: the sender knows the real overall total up
// front (Source.List gave it the whole manifest), the receiver never
// does (see TransferProgress's doc comment for why) and always reports
// overallTotal 0.
func TestSendReceiveFiles_ReportsProgress(t *testing.T) {
	senderConn, receiverConn := net.Pipe()
	defer senderConn.Close()
	defer receiverConn.Close()

	big := bytes.Repeat([]byte{0xAB}, 100_000) // spans multiple 32KB chunks
	source := newMemFileSource(map[string][]byte{"big.bin": big}, []string{"big.bin"})
	sink := newMemFileSink()

	senderProgress := &recordingProgress{}
	receiverProgress := &recordingProgress{}

	errCh := make(chan error, 2)
	go func() {
		errCh <- usecase.SendFiles{
			Source:     source,
			Tunnel:     fakeTunnelConn{stream: senderConn},
			OnProgress: senderProgress.record,
		}.Run(context.Background())
	}()
	go func() {
		errCh <- usecase.ReceiveFiles{
			Sink:       sink,
			Tunnel:     fakeTunnelConn{stream: receiverConn},
			OnProgress: receiverProgress.record,
		}.Run(context.Background())
	}()

	require.NoError(t, <-errCh)
	require.NoError(t, <-errCh)

	senderCalls := senderProgress.snapshot()
	require.NotEmpty(t, senderCalls, "expected multiple progress callbacks for a 100KB file")
	require.Greater(t, len(senderCalls), 1, "a 100KB file should span more than one 32KB chunk")
	last := senderCalls[len(senderCalls)-1]
	require.Equal(t, "big.bin", last.relPath)
	require.Equal(t, int64(len(big)), last.fileDone)
	require.Equal(t, int64(len(big)), last.fileTotal)
	require.Equal(t, int64(len(big)), last.overallDone)
	require.Equal(t, int64(len(big)), last.overallTotal, "sender knows the real total up front")
	for i := 1; i < len(senderCalls); i++ {
		require.GreaterOrEqual(t, senderCalls[i].fileDone, senderCalls[i-1].fileDone, "progress must not go backwards")
	}

	receiverCalls := receiverProgress.snapshot()
	require.NotEmpty(t, receiverCalls)
	rLast := receiverCalls[len(receiverCalls)-1]
	require.Equal(t, int64(len(big)), rLast.fileDone)
	require.Equal(t, int64(0), rLast.overallTotal, "receiver never knows the overall total in advance")
}
