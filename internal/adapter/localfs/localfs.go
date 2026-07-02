// Package localfs implements port.FileSource and port.FileSink over the
// local filesystem: the sending side ("app send") walks a path (a single
// file, or a directory, recursively), the receiving side ("app receive")
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

	"github.com/fu1se/localizator/internal/usecase/port"
)

// Source implements port.FileSource rooted at Path — a single file, or a
// directory walked recursively.
type Source struct {
	Path string
}

func (s Source) List() ([]port.FileEntry, error) {
	info, err := os.Stat(s.Path)
	if err != nil {
		return nil, fmt.Errorf("localfs: stat %s: %w", s.Path, err)
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

func (s Source) Open(relPath string) (io.ReadCloser, error) {
	info, err := os.Stat(s.Path)
	if err != nil {
		return nil, fmt.Errorf("localfs: stat %s: %w", s.Path, err)
	}

	target := s.Path
	if info.IsDir() {
		target = filepath.Join(s.Path, filepath.FromSlash(relPath))
	}

	f, err := os.Open(target)
	if err != nil {
		return nil, fmt.Errorf("localfs: open %s: %w", target, err)
	}
	return f, nil
}

// Sink implements port.FileSink under DestDir.
type Sink struct {
	DestDir string
}

func (s Sink) Create(entry port.FileEntry) (io.WriteCloser, error) {
	safeRel, err := sanitizeRelPath(entry.RelPath)
	if err != nil {
		return nil, err
	}

	full := filepath.Join(s.DestDir, safeRel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return nil, fmt.Errorf("localfs: create parent dir for %s: %w", full, err)
	}

	f, err := os.Create(full)
	if err != nil {
		return nil, fmt.Errorf("localfs: create %s: %w", full, err)
	}
	return f, nil
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
