package restore

import (
	"archive/tar"
	"bytes"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/kopia/kopia/fs"
	"github.com/kopia/kopia/internal/mockfs"
	"github.com/kopia/kopia/internal/testlogging"
)

// TestTarOutput_BSDTarExtractsExtendedMetadata verifies that the system tar (bsdtar) restores
// xattrs and flags from the PAX records kopia writes.
func TestTarOutput_BSDTarExtractsExtendedMetadata(t *testing.T) {
	tarExe, err := exec.LookPath("bsdtar")
	if err != nil {
		t.Skip("bsdtar not found")
	}

	ctx := testlogging.Context(t)
	finderInfo := make([]byte, 32)
	finderInfo[8] = 0x20 // kHasBundle

	root := mockfs.NewDirectory()
	root.AddDir("Bundle", 0o755).SetExtendedAttributes(&fs.ExtendedAttributes{
		Xattrs: map[string][]byte{"com.apple.FinderInfo": finderInfo},
	})
	root.AddFile("Bundle/f", []byte("x"), 0o644).SetExtendedAttributes(&fs.ExtendedAttributes{
		Xattrs:   map[string][]byte{"user.a": []byte("1")},
		BSDFlags: unix.UF_HIDDEN,
	})

	dir := t.TempDir()
	archive := filepath.Join(dir, "out.tar")

	var buf bytes.Buffer

	_, err = Entry(ctx, nil, NewTarOutput(nopWriteCloser{&buf}), root, Options{Parallel: 1, RestoreDirEntryAtDepth: math.MaxInt32})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(archive, buf.Bytes(), 0o600))

	// sanity check that the archive is PAX
	h, err := tar.NewReader(bytes.NewReader(buf.Bytes())).Next()
	require.NoError(t, err)
	require.Equal(t, tar.FormatPAX, h.Format)

	out := filepath.Join(dir, "x")
	require.NoError(t, os.Mkdir(out, 0o700))

	cmd := exec.CommandContext(ctx, tarExe, "-x", "--xattrs", "--fflags", "-f", archive, "-C", out)
	b, err := cmd.CombinedOutput()
	require.NoError(t, err, string(b))

	v := make([]byte, 64)
	n, err := unix.Lgetxattr(filepath.Join(out, "Bundle"), "com.apple.FinderInfo", v)
	require.NoError(t, err)
	require.Equal(t, finderInfo, v[:n])

	n, err = unix.Lgetxattr(filepath.Join(out, "Bundle", "f"), "user.a", v)
	require.NoError(t, err)
	require.Equal(t, []byte("1"), v[:n])

	var st unix.Stat_t
	require.NoError(t, unix.Lstat(filepath.Join(out, "Bundle", "f"), &st))
	require.NotZero(t, st.Flags&unix.UF_HIDDEN)
}
