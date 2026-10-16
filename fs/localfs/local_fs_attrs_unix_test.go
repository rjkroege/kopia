//go:build darwin || linux

package localfs

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/kopia/kopia/fs"
)

func setXattrOrSkip(t *testing.T, path, name string, value []byte) {
	t.Helper()

	err := unix.Lsetxattr(path, name, value, 0)
	if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP) {
		t.Skipf("filesystem does not support xattrs: %v", err)
	}

	require.NoError(t, err)
}

func readAttrs(t *testing.T, path string, opts fs.AttributeOptions) *fs.ExtendedAttributes {
	t.Helper()

	e, err := NewEntry(path)
	require.NoError(t, err)

	a, err := fs.ReadExtendedAttributes(t.Context(), e, opts)
	require.NoError(t, err)

	return a
}

func TestExtendedAttributes_CapturesXattrs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))

	setXattrOrSkip(t, path, "user.keep", []byte("value"))
	setXattrOrSkip(t, path, "user.empty", nil)
	setXattrOrSkip(t, path, "user.excluded", []byte("e"))

	a := readAttrs(t, path, fs.AttributeOptions{
		Xattrs:        fs.XattrsUser,
		ExcludeXattrs: []string{"user.excluded"},
	})

	require.NotNil(t, a)
	require.Equal(t, []byte("value"), a.Xattrs["user.keep"])
	require.Contains(t, a.Xattrs, "user.empty")
	require.Empty(t, a.Xattrs["user.empty"])
	require.NotContains(t, a.Xattrs, "user.excluded")
}

func TestExtendedAttributes_SkipsValuesOverSizeLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))

	setXattrOrSkip(t, path, "user.keep", []byte("value"))

	// Many Linux filesystems limit all xattrs of an inode to one block, so this may not fit.
	err := unix.Lsetxattr(path, "user.big", bytes.Repeat([]byte("b"), fs.MaxXattrSize+1), 0)
	if errors.Is(err, unix.E2BIG) || errors.Is(err, unix.ENOSPC) {
		t.Skipf("filesystem does not support values over %v bytes: %v", fs.MaxXattrSize, err)
	}

	require.NoError(t, err)

	a := readAttrs(t, path, fs.AttributeOptions{Xattrs: fs.XattrsUser})
	require.NotNil(t, a)
	require.Contains(t, a.Xattrs, "user.keep")
	require.NotContains(t, a.Xattrs, "user.big")
}

func TestExtendedAttributes_ModeNoneReadsNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))
	setXattrOrSkip(t, path, "user.keep", []byte("value"))

	require.Nil(t, readAttrs(t, path, fs.AttributeOptions{Xattrs: fs.XattrsNone}))
	require.Nil(t, readAttrs(t, path, fs.AttributeOptions{}))
}

func TestExtendedAttributes_DoesNotFollowSymlinks(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")

	require.NoError(t, os.WriteFile(target, []byte("x"), 0o600))
	require.NoError(t, os.Symlink(target, link))
	setXattrOrSkip(t, target, "user.target", []byte("t"))

	a := readAttrs(t, link, fs.AttributeOptions{Xattrs: fs.XattrsAll})
	if a != nil {
		require.NotContains(t, a.Xattrs, "user.target")
	}
}

func TestExtendedAttributes_Directory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Foo.app")
	require.NoError(t, os.Mkdir(dir, 0o700))
	setXattrOrSkip(t, dir, "user.dir", []byte("d"))

	a := readAttrs(t, dir, fs.AttributeOptions{Xattrs: fs.XattrsUser})
	require.NotNil(t, a)
	require.Equal(t, []byte("d"), a.Xattrs["user.dir"])
}

func TestExtendedAttributes_ViaDirectoryIteration(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "f")

	require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))
	setXattrOrSkip(t, path, "user.keep", []byte("value"))

	d, err := Directory(root)
	require.NoError(t, err)

	e, err := d.Child(t.Context(), "f")
	require.NoError(t, err)

	a, err := fs.ReadExtendedAttributes(t.Context(), e, fs.AttributeOptions{Xattrs: fs.XattrsUser})
	require.NoError(t, err)
	require.NotNil(t, a)
	require.Equal(t, []byte("value"), a.Xattrs["user.keep"])
}
