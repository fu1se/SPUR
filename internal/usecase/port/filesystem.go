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
	Open(relPath string) (io.ReadCloser, error)
}

// FileSink writes received files under a destination directory, creating
// parent directories as needed — the receiving side ("spur receive").
// Implementations must reject a RelPath that would escape the
// destination directory (e.g. via "../"): it comes from whatever the
// counterpart peer sent over the wire, not from a trusted local source.
type FileSink interface {
	Create(entry FileEntry) (io.WriteCloser, error)
}
