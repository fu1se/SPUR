package localfs_test

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/spur/internal/adapter/localfs"
	"github.com/fu1se/spur/internal/usecase/port"
)

func TestSource_ListSingleFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	require.NoError(t, os.WriteFile(path, []byte("hi"), 0o644))

	src := localfs.Source{Path: path}
	entries, err := src.List()
	require.NoError(t, err)
	require.Equal(t, []port.FileEntry{{RelPath: "hello.txt", Size: 2}}, entries)
}

func TestSource_ListDirectoryWalksRecursively(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("bb"), 0o644))

	src := localfs.Source{Path: dir}
	entries, err := src.List()
	require.NoError(t, err)

	sort.Slice(entries, func(i, j int) bool { return entries[i].RelPath < entries[j].RelPath })
	require.Equal(t, []port.FileEntry{
		{RelPath: "a.txt", Size: 1},
		{RelPath: filepath.ToSlash(filepath.Join("sub", "b.txt")), Size: 2},
	}, entries)
}

func TestSource_OpenReadsContent(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644))

	src := localfs.Source{Path: dir}
	r, err := src.Open("a.txt", 0)
	require.NoError(t, err)
	defer r.Close()

	content, err := io.ReadAll(r)
	require.NoError(t, err)
	require.Equal(t, "hello", string(content))
}

func TestSink_CreateRecreatesDirectoryStructure(t *testing.T) {
	dir := t.TempDir()
	sink := localfs.Sink{DestDir: dir}

	w, err := sink.Create(port.FileEntry{RelPath: "sub/nested/a.txt", Size: 5}, 0)
	require.NoError(t, err)
	_, err = w.Write([]byte("hello"))
	require.NoError(t, err)
	require.NoError(t, w.Close())

	content, err := os.ReadFile(filepath.Join(dir, "sub", "nested", "a.txt"))
	require.NoError(t, err)
	require.Equal(t, "hello", string(content))
}

// TestSink_RejectsPathTraversal is a security regression test: RelPath
// comes from whatever the sending peer put on the wire, not a trusted
// local source, so a malicious or buggy sender must not be able to write
// outside DestDir.
func TestSink_RejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	sink := localfs.Sink{DestDir: dir}

	for _, relPath := range []string{
		"../escape.txt",
		"../../etc/passwd",
		"sub/../../escape.txt",
		"/absolute/path.txt",
		"",
	} {
		_, err := sink.Create(port.FileEntry{RelPath: relPath}, 0)
		require.Error(t, err, "expected %q to be rejected", relPath)
	}

	entries, err := os.ReadDir(filepath.Dir(dir))
	require.NoError(t, err)
	for _, e := range entries {
		require.NotEqual(t, "escape.txt", e.Name())
	}
}

// TestSource_ListSkipsSymlinks is a security regression test: a symlink
// inside the tree being sent used to be followed (WalkDir's DirEntry is
// Lstat-based, so d.IsDir() is false for it and it fell through to being
// listed), reporting the wrong size and, on Open, reading the *real*
// target's content -- silently leaking bytes from an arbitrary file
// elsewhere on disk under the symlink's own innocuous relative name.
func TestSource_ListSkipsSymlinks(t *testing.T) {
	dir := t.TempDir()
	secretDir := t.TempDir()
	secret := filepath.Join(secretDir, "secret.txt")
	require.NoError(t, os.WriteFile(secret, []byte("SECRET-CONTENT-OUTSIDE-TREE"), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644))
	require.NoError(t, os.Symlink(secret, filepath.Join(dir, "link.txt")))

	src := localfs.Source{Path: dir}
	entries, err := src.List()
	require.NoError(t, err)

	require.Equal(t, []port.FileEntry{{RelPath: "a.txt", Size: 1}}, entries)
}

// TestSource_ListRejectsSymlinkTopLevelPath checks the same protection
// when the symlink is the path the user directly named, not something
// found while walking a directory.
func TestSource_ListRejectsSymlinkTopLevelPath(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, "secret.txt")
	require.NoError(t, os.WriteFile(secret, []byte("secret"), 0o644))

	link := filepath.Join(t.TempDir(), "link.txt")
	require.NoError(t, os.Symlink(secret, link))

	src := localfs.Source{Path: link}
	_, err := src.List()
	require.Error(t, err)
}

// TestSink_RejectsSymlinkEscape is a security regression test:
// sanitizeRelPath only rejects RelPath strings that syntactically escape
// DestDir; it can't see a pre-existing symlink under DestDir that
// redirects a syntactically safe RelPath outside DestDir on disk. A
// receiving user's --out directory containing (or being tricked into
// containing) such a symlink must not let a sender's file land outside it.
func TestSink_RejectsSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	outsideDir := t.TempDir()

	require.NoError(t, os.Symlink(outsideDir, filepath.Join(dir, "sub")))

	sink := localfs.Sink{DestDir: dir}
	_, err := sink.Create(port.FileEntry{RelPath: "sub/evil.txt", Size: 4}, 0)
	require.Error(t, err)

	_, statErr := os.Stat(filepath.Join(outsideDir, "evil.txt"))
	require.True(t, os.IsNotExist(statErr), "file must not have been written outside DestDir")
}

func TestSource_OpenWithSkipSeeksPastThatManyBytes(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("0123456789"), 0o644))

	src := localfs.Source{Path: dir}
	r, err := src.Open("a.txt", 4)
	require.NoError(t, err)
	defer r.Close()

	content, err := io.ReadAll(r)
	require.NoError(t, err)
	require.Equal(t, "456789", string(content))
}

func TestSink_ExistingSizeReportsZeroForMissingFile(t *testing.T) {
	dir := t.TempDir()
	sink := localfs.Sink{DestDir: dir}

	size, err := sink.ExistingSize("nope.txt")
	require.NoError(t, err)
	require.Zero(t, size)
}

func TestSink_ExistingSizeReportsRealSize(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644))
	sink := localfs.Sink{DestDir: dir}

	size, err := sink.ExistingSize("a.txt")
	require.NoError(t, err)
	require.EqualValues(t, 5, size)
}

// TestSink_CreateWithOffsetAppends is the core resume behavior: writing
// through a non-zero offset must extend the existing content, not
// truncate and start over.
func TestSink_CreateWithOffsetAppends(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello "), 0o644))
	sink := localfs.Sink{DestDir: dir}

	w, err := sink.Create(port.FileEntry{RelPath: "a.txt", Size: 11}, 6)
	require.NoError(t, err)
	_, err = w.Write([]byte("world"))
	require.NoError(t, err)
	require.NoError(t, w.Close())

	content, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	require.NoError(t, err)
	require.Equal(t, "hello world", string(content))
}

// TestSink_CreateWithOffsetMismatchFails guards against the caller (or a
// concurrently modified destination file) claiming a resume offset that
// doesn't actually match what's on disk -- writing at the wrong offset
// would either skip real content or silently overwrite bytes meant to be
// kept.
func TestSink_CreateWithOffsetMismatchFails(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644))
	sink := localfs.Sink{DestDir: dir}

	_, err := sink.Create(port.FileEntry{RelPath: "a.txt", Size: 100}, 999)
	require.Error(t, err)
}
