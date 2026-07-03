package port

import "io"

// FileEntry describes one file to transfer, identified by a path relative
// to whatever root is being sent — preserved on the receiving side to
// reconstruct the directory structure the sender walked.
type FileEntry struct {
	RelPath string
	Size    int64
}

// FileSource enumerates and opens files rooted at a local path (a single
// file, or a directory walked recursively) — the sending side ("spur
// send").
type FileSource interface {
	List() ([]FileEntry, error)
	// Open opens relPath for reading, positioned skip bytes into the
	// file — used to resume a partially-sent file without re-reading (and
	// re-sending) bytes the receiver already has. skip 0 opens normally
	// from the start.
	Open(relPath string, skip int64) (io.ReadCloser, error)
}

// FileSink writes received files under a destination directory, creating
// parent directories as needed — the receiving side ("spur receive").
// Implementations must reject a RelPath that would escape the
// destination directory (e.g. via "../"): it comes from whatever the
// counterpart peer sent over the wire, not from a trusted local source.
type FileSink interface {
	// ExistingSize reports how many bytes are already present at
	// entry.RelPath under the sink's destination, 0 if the file doesn't
	// exist yet — used to detect and offer resuming an interrupted
	// transfer instead of starting over.
	ExistingSize(relPath string) (int64, error)
	// Create opens relPath for writing starting at offset: offset 0
	// truncates and creates fresh; offset > 0 appends starting exactly
	// there. The caller (usecase.ReceiveFiles) is responsible for offset
	// matching a size ExistingSize actually reported — Create itself
	// doesn't re-verify that.
	Create(entry FileEntry, offset int64) (io.WriteCloser, error)
}
