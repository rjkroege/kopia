//go:build darwin || linux

package restore

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/kopia/kopia/fs"
	"github.com/kopia/kopia/fs/localfs"
	"github.com/kopia/kopia/internal/mockfs"
	"github.com/kopia/kopia/internal/testlogging"
)

func requireXattrSupport(t *testing.T, dir string) {
	t.Helper()

	p := filepath.Join(dir, ".probe")
	require.NoError(t, os.WriteFile(p, nil, 0o600))

	defer os.Remove(p)

	err := unix.Lsetxattr(p, "user.probe", []byte("x"), 0)
	if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP) {
		t.Skipf("filesystem does not support xattrs: %v", err)
	}

	require.NoError(t, err)
}

func restoreMock(t *testing.T, root *mockfs.Directory, configure func(o *FilesystemOutput)) (string, *FilesystemOutput) {
	t.Helper()

	ctx := testlogging.Context(t)
	target := t.TempDir()
	requireXattrSupport(t, target)

	o := &FilesystemOutput{TargetPath: target, OverwriteDirectories: true, IgnorePermissionErrors: true}
	if configure != nil {
		configure(o)
	}

	require.NoError(t, o.Init(ctx))

	_, err := Entry(ctx, nil, o, root, Options{Parallel: 1, RestoreDirEntryAtDepth: math.MaxInt32})
	require.NoError(t, err)

	return target, o
}

func localAttrs(t *testing.T, path string) *fs.ExtendedAttributes {
	t.Helper()

	e, err := localfs.NewEntry(path)
	require.NoError(t, err)

	a, err := fs.ReadExtendedAttributes(t.Context(), e, fs.AttributeOptions{Xattrs: fs.XattrsAll, BSDFlags: true, BirthTime: true})
	require.NoError(t, err)

	if a == nil {
		return &fs.ExtendedAttributes{}
	}

	return a
}

func TestRestore_XattrsRoundTrip(t *testing.T) {
	root := mockfs.NewDirectory()
	d := root.AddDir("Foo.app", 0o755)
	d.SetExtendedAttributes(&fs.ExtendedAttributes{Xattrs: map[string][]byte{"user.dir": []byte("d")}})

	f := root.AddFile("Foo.app/readonly", []byte("contents"), 0o444)
	f.SetExtendedAttributes(&fs.ExtendedAttributes{Xattrs: map[string][]byte{
		"user.a":     []byte("1"),
		"user.empty": {},
	}})

	target, o := restoreMock(t, root, nil)

	require.Equal(t, []byte("d"), localAttrs(t, filepath.Join(target, "Foo.app")).Xattrs["user.dir"])

	fa := localAttrs(t, filepath.Join(target, "Foo.app", "readonly"))
	require.Equal(t, []byte("1"), fa.Xattrs["user.a"], "xattrs are set before the file becomes read-only")
	require.Contains(t, fa.Xattrs, "user.empty")

	st, err := os.Stat(filepath.Join(target, "Foo.app", "readonly"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o444), st.Mode().Perm())
	require.Zero(t, o.warnings.count)
}

func TestRestore_SkipXattrs(t *testing.T) {
	root := mockfs.NewDirectory()
	root.AddFile("f", []byte("x"), 0o644).SetExtendedAttributes(&fs.ExtendedAttributes{Xattrs: map[string][]byte{"user.a": []byte("1")}})

	target, _ := restoreMock(t, root, func(o *FilesystemOutput) { o.SkipXattrs = true })

	require.NotContains(t, localAttrs(t, filepath.Join(target, "f")).Xattrs, "user.a")
}

func TestRestore_RestoresMtimeAfterXattrs(t *testing.T) {
	root := mockfs.NewDirectory()
	f := root.AddFile("f", []byte("x"), 0o644)
	f.SetExtendedAttributes(&fs.ExtendedAttributes{Xattrs: map[string][]byte{"user.a": []byte("1")}})

	target, _ := restoreMock(t, root, nil)

	st, err := os.Lstat(filepath.Join(target, "f"))
	require.NoError(t, err)
	require.True(t, st.ModTime().Equal(f.ModTime()), "got %v want %v", st.ModTime(), f.ModTime())
}
