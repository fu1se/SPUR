// Package localfs implements port.FileSource and port.FileSink over the
// local filesystem: the sending side ("spur send") walks a path (a single
// file, or a directory, recursively), the receiving side ("spur receive")
// writes files under a destination directory, recreating the relative
// directory structure the sender walked.
package localfs

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/fu1se/spur/internal/usecase/port"
)

// Source implements port.FileSource rooted at Path — a single file, or a
// directory walked recursively.
type Source struct {
	Path string
}

func (s Source) List() ([]port.FileEntry, error) {
	// Lstat, not Stat: if s.Path itself is a symlink, following it here
	// would mean sending whatever it points to (possibly well outside the
	// path the user actually named) under an innocuous top-level name.
	info, err := os.Lstat(s.Path)
	if err != nil {
		return nil, fmt.Errorf("localfs: stat %s: %w", s.Path, err)
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return nil, fmt.Errorf("localfs: refusing to send %s: it is a symlink", s.Path)
	}

	if !info.IsDir() {
		return []port.FileEntry{{RelPath: filepath.Base(s.Path), Size: info.Size()}}, nil
	}

	var entries []port.FileEntry
	err = filepath.WalkDir(s.Path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			// Not followed: WalkDir's DirEntry is Lstat-based, so naively
			// following a symlink here would report the wrong size (the
			// length of the link target string, not the real file) and
			// then, on Open, read the *real* target's content — silently
			// leaking bytes from an arbitrary file elsewhere on disk
			// under the symlink's own innocuous relative name. There's no
			// way to represent a symlink on the wire anyway (only regular
			// file bytes are ever transferred), so it's simply excluded.
			return nil
		}
		rel, err := filepath.Rel(s.Path, path)
		if err != nil {
			return fmt.Errorf("localfs: relative path for %s: %w", path, err)
		}
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("localfs: stat %s: %w", path, err)
		}
		// Wire format uses forward slashes regardless of host OS, so a
		// Windows sender and a Linux receiver (or vice versa) agree on
		// path separators.
		entries = append(entries, port.FileEntry{RelPath: filepath.ToSlash(rel), Size: info.Size()})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("localfs: walk %s: %w", s.Path, err)
	}
	return entries, nil
}

func (s Source) Open(relPath string, skip int64) (io.ReadCloser, error) {
	info, err := os.Lstat(s.Path)
	if err != nil {
		return nil, fmt.Errorf("localfs: stat %s: %w", s.Path, err)
	}

	target := s.Path
	if info.IsDir() {
		target = filepath.Join(s.Path, filepath.FromSlash(relPath))
	}

	// Defense in depth alongside List's filtering above: refuse to open a
	// symlink even if somehow asked to (relPath is meant to only ever be
	// one List already vetted, but Open doesn't otherwise know that).
	if targetInfo, err := os.Lstat(target); err == nil && targetInfo.Mode()&fs.ModeSymlink != 0 {
		return nil, fmt.Errorf("localfs: refusing to open %s: it is a symlink", target)
	}

	f, err := os.Open(target)
	if err != nil {
		return nil, fmt.Errorf("localfs: open %s: %w", target, err)
	}
	if skip > 0 {
		if _, err := f.Seek(skip, io.SeekStart); err != nil {
			f.Close()
			return nil, fmt.Errorf("localfs: seek %s to %d: %w", target, skip, err)
		}
	}
	return f, nil
}

// Sink implements port.FileSink under DestDir.
type Sink struct {
	DestDir string
}

// ExistingSize reports how many bytes are already present at relPath
// under DestDir, 0 if the file doesn't exist yet — used to detect and
// offer resuming an interrupted transfer instead of starting over.
func (s Sink) ExistingSize(relPath string) (int64, error) {
	safeRel, err := sanitizeRelPath(relPath)
	if err != nil {
		return 0, err
	}

	info, err := os.Stat(filepath.Join(s.DestDir, safeRel))
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("localfs: stat %s: %w", relPath, err)
	}
	return info.Size(), nil
}

func (s Sink) Create(entry port.FileEntry, offset int64) (io.WriteCloser, error) {
	safeRel, err := sanitizeRelPath(entry.RelPath)
	if err != nil {
		return nil, err
	}

	full := filepath.Join(s.DestDir, safeRel)
	parent := filepath.Dir(full)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, fmt.Errorf("localfs: create parent dir for %s: %w", full, err)
	}

	if err := s.ensureWithinDestDir(parent); err != nil {
		return nil, err
	}

	if offset == 0 {
		f, err := os.Create(full)
		if err != nil {
			return nil, fmt.Errorf("localfs: create %s: %w", full, err)
		}
		return f, nil
	}

	// Resuming: the file must already exist with exactly offset bytes —
	// see port.FileSink.Create's doc comment on whose job it is to have
	// checked that (usecase.ReceiveFiles, via ExistingSize). Verified
	// again here rather than trusted blindly: a mismatch would otherwise
	// either silently skip real content (opening past the true end,
	// leaving a gap) or overwrite bytes that were supposed to be kept
	// (opening before it).
	if info, err := os.Stat(full); err != nil {
		return nil, fmt.Errorf("localfs: stat %s before resuming: %w", full, err)
	} else if info.Size() != offset {
		return nil, fmt.Errorf("localfs: resume offset %d for %s doesn't match actual size %d on disk", offset, full, info.Size())
	}

	f, err := os.OpenFile(full, os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("localfs: open %s for resume: %w", full, err)
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		f.Close()
		return nil, fmt.Errorf("localfs: seek %s to %d: %w", full, offset, err)
	}
	return f, nil
}

// ensureWithinDestDir defends against a symlink that already exists
// somewhere under DestDir before this transfer even started: sanitizeRelPath
// only rejects RelPath strings that syntactically escape DestDir (e.g.
// "../etc/passwd"), but a syntactically safe RelPath can still resolve
// outside DestDir on disk if one of its path components is a pre-existing
// symlink (e.g. DestDir/sub -> /etc, then RelPath "sub/evil.txt"). Called
// after MkdirAll so every component of parent actually exists and
// EvalSymlinks can resolve it.
func (s Sink) ensureWithinDestDir(parent string) error {
	realDestDir, err := filepath.EvalSymlinks(s.DestDir)
	if err != nil {
		return fmt.Errorf("localfs: resolve %s: %w", s.DestDir, err)
	}
	realParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return fmt.Errorf("localfs: resolve %s: %w", parent, err)
	}

	rel, err := filepath.Rel(realDestDir, realParent)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("localfs: refusing to write outside %s (symlink escape via %s)", s.DestDir, parent)
	}
	return nil
}

// sanitizeRelPath rejects a relative path that would escape DestDir once
// joined — entry.RelPath comes from whatever the sending peer put on the
// wire, not from a trusted local source, so a malicious or buggy sender
// sending "../../etc/passwd"-style paths must not be able to write
// outside the destination directory.
func sanitizeRelPath(relPath string) (string, error) {
	if relPath == "" {
		return "", fmt.Errorf("localfs: empty relative path")
	}

	cleaned := filepath.Clean(filepath.FromSlash(relPath))
	if filepath.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("localfs: unsafe relative path %q", relPath)
	}
	return cleaned, nil
}
