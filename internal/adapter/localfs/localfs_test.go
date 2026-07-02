package localfs_test

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/localizator/internal/adapter/localfs"
	"github.com/fu1se/localizator/internal/usecase/port"
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
	r, err := src.Open("a.txt")
	require.NoError(t, err)
	defer r.Close()

	content, err := io.ReadAll(r)
	require.NoError(t, err)
	require.Equal(t, "hello", string(content))
}

func TestSink_CreateRecreatesDirectoryStructure(t *testing.T) {
	dir := t.TempDir()
	sink := localfs.Sink{DestDir: dir}

	w, err := sink.Create(port.FileEntry{RelPath: "sub/nested/a.txt", Size: 5})
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
		_, err := sink.Create(port.FileEntry{RelPath: relPath})
		require.Error(t, err, "expected %q to be rejected", relPath)
	}

	entries, err := os.ReadDir(filepath.Dir(dir))
	require.NoError(t, err)
	for _, e := range entries {
		require.NotEqual(t, "escape.txt", e.Name())
	}
}
