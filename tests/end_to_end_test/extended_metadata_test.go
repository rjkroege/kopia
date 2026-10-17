//go:build darwin || linux

package endtoend_test

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/kopia/kopia/internal/testutil"
	"github.com/kopia/kopia/tests/clitestutil"
	"github.com/kopia/kopia/tests/testenv"
)

// xattrsIgnoredInComparison are added by the system (sandbox provenance, Gatekeeper) to files
// the test creates or restores, or are excluded by default, so they are not compared.
var xattrsIgnoredInComparison = map[string]bool{
	"com.apple.lastuseddate#PS": true,
	"com.apple.macl":            true,
	"com.apple.provenance":      true,
	"com.apple.quarantine":      true,
}

func setXattrOrSkip(t *testing.T, path, name string, value []byte) {
	t.Helper()

	err := unix.Lsetxattr(path, name, value, 0)
	if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP) {
		t.Skipf("filesystem does not support xattrs: %v", err)
	}

	require.NoError(t, err)
}

// listXattrs returns the comparable xattrs of path, without following symlinks.
func listXattrs(t *testing.T, path string) map[string][]byte {
	t.Helper()

	sz, err := unix.Llistxattr(path, nil)
	require.NoError(t, err)

	result := map[string][]byte{}
	if sz == 0 {
		return result
	}

	buf := make([]byte, sz)
	sz, err = unix.Llistxattr(path, buf)
	require.NoError(t, err)

	for name := range strings.SplitSeq(string(buf[:sz]), "\x00") {
		if name == "" || xattrsIgnoredInComparison[name] {
			continue
		}

		vsz, err := unix.Lgetxattr(path, name, nil)
		require.NoError(t, err)

		v := make([]byte, vsz)
		if vsz > 0 {
			vsz, err = unix.Lgetxattr(path, name, v)
			require.NoError(t, err)
		}

		result[name] = v[:vsz]
	}

	return result
}

// treeXattrs returns comparable xattrs of every entry under root, keyed by relative path.
func treeXattrs(t *testing.T, root string) map[string]map[string][]byte {
	t.Helper()

	result := map[string]map[string][]byte{}

	require.NoError(t, filepath.WalkDir(root, func(path string, _ fs.DirEntry, err error) error {
		require.NoError(t, err)

		rel, err := filepath.Rel(root, path)
		require.NoError(t, err)

		if rel == "." {
			return nil
		}

		if x := listXattrs(t, path); len(x) > 0 {
			result[rel] = x
		}

		return nil
	}))

	return result
}

// newExtendedMetadataSource creates a tree with user xattrs on files and directories.
func newExtendedMetadataSource(t *testing.T) string {
	t.Helper()

	src := testutil.TempDirectory(t)
	sub := filepath.Join(src, "Notes.rtfd")
	file := filepath.Join(sub, "TXT.rtf")
	readonly := filepath.Join(src, "readonly.txt")

	require.NoError(t, os.Mkdir(sub, 0o755))
	require.NoError(t, os.WriteFile(file, []byte("{\\rtf1 hello}"), 0o644))
	require.NoError(t, os.WriteFile(readonly, []byte("ro"), 0o644))

	setXattrOrSkip(t, sub, "user.dir", []byte("directory"))
	setXattrOrSkip(t, file, "user.text", []byte("value"))
	setXattrOrSkip(t, file, "user.binary", []byte{0, 1, 2, 0xff})
	setXattrOrSkip(t, file, "user.empty", nil)
	setXattrOrSkip(t, readonly, "user.ro", []byte("x"))
	require.NoError(t, os.Chmod(readonly, 0o444))

	return src
}

func newExtendedMetadataEnv(t *testing.T) *testenv.CLITest {
	t.Helper()

	e := testenv.NewCLITest(t, testenv.RepoFormatNotImportant, testenv.NewInProcRunner(t))
	e.RunAndExpectSuccess(t, "repo", "create", "filesystem", "--path", e.RepoDir)

	return e
}

func latestSnapshotID(t *testing.T, e *testenv.CLITest, source string) string {
	t.Helper()

	si := clitestutil.ListSnapshotsAndExpectSuccess(t, e, source)
	require.Len(t, si, 1)
	require.NotEmpty(t, si[0].Snapshots)

	return si[0].Snapshots[len(si[0].Snapshots)-1].SnapshotID
}

func TestSnapshotRestore_ExtendedAttributesRoundTrip(t *testing.T) {
	t.Parallel()

	src := newExtendedMetadataSource(t)
	e := newExtendedMetadataEnv(t)

	defer e.RunAndExpectSuccess(t, "repo", "disconnect")

	e.RunAndExpectSuccess(t, "policy", "set", "--global", "--xattrs=all", "--file-flags=true", "--birth-time=true")
	e.RunAndExpectSuccess(t, "snapshot", "create", src)

	dst := testutil.TempDirectory(t)
	e.RunAndExpectSuccess(t, "snapshot", "restore", latestSnapshotID(t, e, src), dst)

	want := treeXattrs(t, src)
	require.NotEmpty(t, want)
	require.Equal(t, want, treeXattrs(t, dst))

	st, err := os.Stat(filepath.Join(dst, "readonly.txt"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o444), st.Mode().Perm())
}

func TestSnapshotRestore_ExtendedAttributesDisabledByDefault(t *testing.T) {
	t.Parallel()

	src := newExtendedMetadataSource(t)
	e := newExtendedMetadataEnv(t)

	defer e.RunAndExpectSuccess(t, "repo", "disconnect")

	e.RunAndExpectSuccess(t, "snapshot", "create", src)

	dst := testutil.TempDirectory(t)
	e.RunAndExpectSuccess(t, "snapshot", "restore", latestSnapshotID(t, e, src), dst)

	require.Empty(t, treeXattrs(t, dst))
}

func TestSnapshotRestore_SkipXattrs(t *testing.T) {
	t.Parallel()

	src := newExtendedMetadataSource(t)
	e := newExtendedMetadataEnv(t)

	defer e.RunAndExpectSuccess(t, "repo", "disconnect")

	e.RunAndExpectSuccess(t, "policy", "set", "--global", "--xattrs=all")
	e.RunAndExpectSuccess(t, "snapshot", "create", src)

	dst := testutil.TempDirectory(t)
	e.RunAndExpectSuccess(t, "snapshot", "restore", "--skip-xattrs", latestSnapshotID(t, e, src), dst)

	require.Empty(t, treeXattrs(t, dst))
}

func TestSnapshotCreate_XattrOnlyChangeIsCaptured(t *testing.T) {
	t.Parallel()

	src := newExtendedMetadataSource(t)
	e := newExtendedMetadataEnv(t)

	defer e.RunAndExpectSuccess(t, "repo", "disconnect")

	file := filepath.Join(src, "Notes.rtfd", "TXT.rtf")

	e.RunAndExpectSuccess(t, "policy", "set", "--global", "--xattrs=all")
	e.RunAndExpectSuccess(t, "snapshot", "create", src)

	st, err := os.Lstat(file)
	require.NoError(t, err)

	setXattrOrSkip(t, file, "user.text", []byte("changed"))

	// keep mtime and size identical so only the xattr differs
	require.NoError(t, os.Chtimes(file, st.ModTime(), st.ModTime()))

	e.RunAndExpectSuccess(t, "snapshot", "create", src)

	si := clitestutil.ListSnapshotsAndExpectSuccess(t, e, src)
	require.Len(t, si[0].Snapshots, 2)
	require.NotEqual(t, si[0].Snapshots[0].ObjectID, si[0].Snapshots[1].ObjectID)

	dst := testutil.TempDirectory(t)
	e.RunAndExpectSuccess(t, "snapshot", "restore", si[0].Snapshots[1].SnapshotID, dst)

	got := listXattrs(t, filepath.Join(dst, "Notes.rtfd", "TXT.rtf"))
	require.True(t, bytes.Equal([]byte("changed"), got["user.text"]), "got %q", got["user.text"])
}
