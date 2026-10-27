package localfs

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/kopia/kopia/fs"
)

func TestExtendedAttributes_BSDFlagsAndBirthTime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))
	require.NoError(t, unix.Chflags(path, unix.UF_HIDDEN|unix.UF_NODUMP))

	a := readAttrs(t, path, fs.AttributeOptions{BSDFlags: true, BirthTime: true})
	require.NotNil(t, a)
	require.Equal(t, uint32(unix.UF_HIDDEN|unix.UF_NODUMP), a.BSDFlags)
	require.NotZero(t, a.BirthTime)
	require.Empty(t, a.Xattrs, "xattrs are not read unless selected")

	a = readAttrs(t, path, fs.AttributeOptions{BirthTime: true})
	require.Zero(t, a.BSDFlags, "flags are not read unless selected")
}

func TestExtendedAttributes_MasksStorageFlags(t *testing.T) {
	require.Zero(t, restorableBSDFlags&(unix.UF_COMPRESSED|unix.UF_TRACKED|unix.SF_DATALESS|unix.SF_RESTRICTED))
}

func TestExtendedAttributes_FinderInfo(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Bundle")
	require.NoError(t, os.Mkdir(dir, 0o700))

	finderInfo := make([]byte, 32)
	finderInfo[8] = 0x20 // kHasBundle

	setXattrOrSkip(t, dir, "com.apple.FinderInfo", finderInfo)

	a := readAttrs(t, dir, fs.AttributeOptions{Xattrs: fs.XattrsAll})
	require.NotNil(t, a)
	require.Equal(t, finderInfo, a.Xattrs["com.apple.FinderInfo"])
}

func TestExtendedAttributes_ACL(t *testing.T) {
	dir := t.TempDir()
	withACL := filepath.Join(dir, "acl")
	withoutACL := filepath.Join(dir, "plain")

	require.NoError(t, os.WriteFile(withACL, []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(withoutACL, []byte("x"), 0o600))

	out, err := exec.CommandContext(t.Context(), "chmod", "+a", "everyone allow readattr", withACL).CombinedOutput()
	require.NoError(t, err, string(out))

	a := readAttrs(t, withACL, fs.AttributeOptions{ACLs: true})
	require.NotNil(t, a)
	require.NotEmpty(t, a.ACL)
	require.Empty(t, a.Xattrs)

	require.Nil(t, readAttrs(t, withoutACL, fs.AttributeOptions{ACLs: true}))
	require.Nil(t, readAttrs(t, withACL, fs.AttributeOptions{BirthTime: false}))
}
