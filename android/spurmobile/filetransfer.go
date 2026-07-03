package spurmobile

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/fu1se/spur/internal/adapter/rendezvous"
	"github.com/fu1se/spur/internal/usecase"
	"github.com/fu1se/spur/internal/usecase/port"
)

// MobileReader/MobileWriter mirror io.ReadCloser/io.WriteCloser: gomobile
// can't bind stdlib io interfaces directly (only interfaces it generates
// bindings for from this package), so these are this package's own
// equivalents — implemented on the Kotlin side over Storage Access
// Framework streams (content:// URIs), not plain os.File, since Android
// doesn't give apps arbitrary filesystem path access the way desktop
// does (see FileSource/FileSink below).
//
// MobileReader.Read deliberately does NOT follow io.Reader's "fill the
// caller-supplied buffer" shape (confirmed live, the hard way: a first
// version shaped like Go's io.Reader — Read(buf []byte) (int, error),
// with Kotlin filling buf in place — silently produced all-zero data on
// the wire every time, despite reporting the "correct" byte count and no
// error. gomobile's []byte marshaling for a parameter passed from Go
// into a Java-implemented interface method copies Go's slice into a new
// Java array for the call, but never copies that Java array's contents
// back into the original Go slice afterward — Kotlin was filling a copy
// nobody on the Go side ever looked at again. Passing data the other way
// (Write below, or a []byte return value like here) is the direction
// gomobile actually supports, so Read instead asks for up to n bytes and
// returns a freshly allocated slice — mirrored in reverse from
// mobileReadCloser.Read.
type MobileReader interface {
	// Read returns up to n freshly read bytes, or a zero-length slice at
	// EOF (mirrors java.io.InputStream.read() returning -1, translated
	// to "no bytes" here since there's no separate error channel for it —
	// see mobileReadCloser.Read).
	Read(n int) ([]byte, error)
	Close() error
}

type MobileWriter interface {
	Write(buf []byte) (int, error)
	Close() error
}

// FileSource is implemented on the Kotlin side (backed by SAF) and
// called from Go — the mobile-boundary equivalent of
// usecase/port.FileSource. ListJSON returns a JSON array of
// {"relPath":str,"size":int64} objects rather than []port.FileEntry
// directly: gomobile can't bind a slice of structs from a package it
// isn't told to bind, and port.FileEntry lives in internal/usecase/port,
// not here.
type FileSource interface {
	ListJSON() (string, error)
	Open(relPath string, skip int64) (MobileReader, error)
}

// FileSink is implemented on the Kotlin side (backed by SAF) — the
// mobile-boundary equivalent of usecase/port.FileSink. Create takes
// size/offset as separate primitives instead of a port.FileEntry for the
// same reason ListJSON returns JSON instead of a struct slice.
type FileSink interface {
	ExistingSize(relPath string) (int64, error)
	Create(relPath string, size int64, offset int64) (MobileWriter, error)
}

// ProgressCallback mirrors usecase.TransferProgress as a single-method
// interface — gomobile can't bind bare Go func types as callbacks.
type ProgressCallback interface {
	OnProgress(relPath string, fileDone, fileTotal, overallDone, overallTotal int64)
}

// ResumeCallback mirrors usecase.ResumeOffer. Asked once per transfer,
// not per file — see usecase.ReceiveFiles.OnResumeOffer's doc comment.
type ResumeCallback interface {
	OfferResume(filesWithData int, alreadyHave, total int64) bool
}

// mobileFileSource/mobileFileSink adapt the Kotlin-backed FileSource/
// FileSink into usecase/port.FileSource/FileSink, so SendFiles/
// ReceiveFiles never need to know their data actually lives behind JNI
// calls into SAF rather than plain os/filepath.
type mobileFileSource struct{ src FileSource }

func (s mobileFileSource) List() ([]port.FileEntry, error) {
	js, err := s.src.ListJSON()
	if err != nil {
		return nil, err
	}
	var entries []port.FileEntry
	if err := json.Unmarshal([]byte(js), &entries); err != nil {
		return nil, fmt.Errorf("spurmobile: decode file list: %w", err)
	}
	return entries, nil
}

func (s mobileFileSource) Open(relPath string, skip int64) (io.ReadCloser, error) {
	r, err := s.src.Open(relPath, skip)
	if err != nil {
		return nil, err
	}
	return mobileReadCloser{r}, nil
}

type mobileReadCloser struct{ r MobileReader }

// Read asks the Kotlin side for up to len(p) bytes (see MobileReader's
// doc comment for why it can't just fill p directly) and copies the
// freshly returned slice into p. A zero-length result with no error
// means EOF — the mobile-boundary equivalent of
// java.io.InputStream.read() returning -1.
func (m mobileReadCloser) Read(p []byte) (int, error) {
	chunk, err := m.r.Read(len(p))
	if err != nil {
		return 0, err
	}
	if len(chunk) == 0 {
		return 0, io.EOF
	}
	return copy(p, chunk), nil
}
func (m mobileReadCloser) Close() error { return m.r.Close() }

type mobileFileSink struct{ sink FileSink }

func (s mobileFileSink) ExistingSize(relPath string) (int64, error) {
	return s.sink.ExistingSize(relPath)
}

func (s mobileFileSink) Create(entry port.FileEntry, offset int64) (io.WriteCloser, error) {
	w, err := s.sink.Create(entry.RelPath, entry.Size, offset)
	if err != nil {
		return nil, err
	}
	return mobileWriteCloser{w}, nil
}

type mobileWriteCloser struct{ w MobileWriter }

func (m mobileWriteCloser) Write(p []byte) (int, error) { return m.w.Write(p) }
func (m mobileWriteCloser) Close() error                { return m.w.Close() }

func progressFunc(cb ProgressCallback) usecase.TransferProgress {
	if cb == nil {
		return nil
	}
	return func(relPath string, fileDone, fileTotal, overallDone, overallTotal int64) {
		cb.OnProgress(relPath, fileDone, fileTotal, overallDone, overallTotal)
	}
}

func resumeFunc(cb ResumeCallback) usecase.ResumeOffer {
	if cb == nil {
		return nil
	}
	return func(filesWithData int, alreadyHave, total int64) bool {
		return cb.OfferResume(filesWithData, alreadyHave, total)
	}
}

// Transfer is a running "send"/"receive" session (see
// Client.StartSend/StartReceive) — same shape as PortForward: already
// established by the time a caller gets one back, runs to completion (or
// Stop) in a background goroutine, Await reports how it ended.
type Transfer struct {
	cancel context.CancelFunc
	done   chan error
}

func (t *Transfer) Stop() { t.cancel() }

func (t *Transfer) Await() error { return <-t.done }

// StartSend is "spur send": streams every file source enumerates to
// whichever counterpart to/room resolves to (see StartConnect for the
// to/room/onCode contract — identical here). Blocks until the tunnel is
// established before returning; call from a background thread.
func (c *Client) StartSend(serverAddr, stunAddr, to, room string, source FileSource, onCode CodeCallback, onProgress ProgressCallback) (*Transfer, error) {
	resolve := rendezvous.CounterpartResolverFor(to, room, codeFunc(onCode))
	tun, _, _, err := rendezvous.Establish(context.Background(), serverAddr, stunAddr, c.identityPath, c.trustStorePath, Version(), resolve, func(string) {}, nil)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	tr := &Transfer{cancel: cancel, done: make(chan error, 1)}
	go func() {
		runErr := usecase.SendFiles{
			Source:     mobileFileSource{source},
			Tunnel:     tun.Conn,
			OnProgress: progressFunc(onProgress),
		}.Run(ctx)
		tun.Close()
		tr.done <- runErr
	}()
	return tr, nil
}

// StartReceive is "spur receive": accepts whatever the resolved
// counterpart streams and writes it via sink. See StartSend for
// to/room/onCode/onProgress; onResume is asked once, before any data
// arrives, whether to resume files sink already has data for (nil
// behaves like always starting fresh — see usecase.ReceiveFiles.OnResumeOffer).
func (c *Client) StartReceive(serverAddr, stunAddr, to, room string, sink FileSink, onCode CodeCallback, onProgress ProgressCallback, onResume ResumeCallback) (*Transfer, error) {
	resolve := rendezvous.CounterpartResolverFor(to, room, codeFunc(onCode))
	tun, _, _, err := rendezvous.Establish(context.Background(), serverAddr, stunAddr, c.identityPath, c.trustStorePath, Version(), resolve, func(string) {}, nil)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	tr := &Transfer{cancel: cancel, done: make(chan error, 1)}
	go func() {
		runErr := usecase.ReceiveFiles{
			Sink:          mobileFileSink{sink},
			Tunnel:        tun.Conn,
			OnProgress:    progressFunc(onProgress),
			OnResumeOffer: resumeFunc(onResume),
		}.Run(ctx)
		tun.Close()
		tr.done <- runErr
	}()
	return tr, nil
}
