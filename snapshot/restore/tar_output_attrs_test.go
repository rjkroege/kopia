package restore

import (
	"archive/tar"
	"bytes"
	"io"
	"math"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kopia/kopia/fs"
	"github.com/kopia/kopia/internal/mockfs"
	"github.com/kopia/kopia/internal/testlogging"
)

type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error { return nil }

func tarHeaders(t *testing.T, root *mockfs.Directory) map[string]*tar.Header {
	t.Helper()

	ctx := testlogging.Context(t)

	var buf bytes.Buffer

	o := NewTarOutput(nopWriteCloser{&buf})

	_, err := Entry(ctx, nil, o, root, Options{Parallel: 1, RestoreDirEntryAtDepth: math.MaxInt32})
	require.NoError(t, err)

	headers := map[string]*tar.Header{}
	r := tar.NewReader(&buf)

	for {
		h, err := r.Next()
		if err == io.EOF {
			break
		}

		require.NoError(t, err)

		headers[h.Name] = h
	}

	return headers
}

func TestTarOutput_WritesPAXExtendedMetadata(t *testing.T) {
	btime := time.Date(2001, 3, 24, 12, 0, 0, 5, time.UTC)
	binary := []byte{0, 1, 2, 0xff, 0}

	root := mockfs.NewDirectory()
	root.AddDir("Bundle", 0o755).SetExtendedAttributes(fs.ExtendedAttributes{
		Xattrs: map[string][]byte{"com.apple.FinderInfo": binary},
	})
	root.AddFile("Bundle/f", []byte("x"), 0o644).SetExtendedAttributes(fs.ExtendedAttributes{
		Xattrs:    map[string][]byte{"user.a": []byte("1")},
		BSDFlags:  0x8000 | 0x1,
		BirthTime: fs.UTCTimestampFromTime(btime),
	})
	root.AddSymlink("link", "Bundle/f", 0o777).SetExtendedAttributes(fs.ExtendedAttributes{
		Xattrs: map[string][]byte{"user.l": []byte("l")},
	})
	root.AddFile("plain", []byte("x"), 0o644)

	h := tarHeaders(t, root)

	require.Equal(t, string(binary), h["Bundle/"].PAXRecords["SCHILY.xattr.com.apple.FinderInfo"], "binary values survive")
	require.Equal(t, "1", h["Bundle/f"].PAXRecords["SCHILY.xattr.user.a"])
	require.Equal(t, "nodump,hidden", h["Bundle/f"].PAXRecords["SCHILY.fflags"])
	require.Equal(t, "985435200.000000005", h["Bundle/f"].PAXRecords["LIBARCHIVE.creationtime"])
	require.Equal(t, "l", h["link"].PAXRecords["SCHILY.xattr.user.l"])
	require.Empty(t, h["plain"].PAXRecords)
}

func TestBSDFlagsText(t *testing.T) {
	require.Empty(t, bsdFlagsText(0))
	require.Equal(t, "uchg,schg", bsdFlagsText(0x2|0x20000))
	require.Empty(t, bsdFlagsText(0x20), "flags without a name are dropped")
}

func TestTarMode(t *testing.T) {
	require.Equal(t, int64(0o755), tarMode(os.ModeDir|0o755))
	require.Equal(t, int64(0o4755), tarMode(os.ModeSetuid|0o755))
	require.Equal(t, int64(0o3775), tarMode(os.ModeDir|os.ModeSetgid|os.ModeSticky|0o775))
}
