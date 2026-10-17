package restore

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/kopia/kopia/fs"
	"github.com/kopia/kopia/internal/mockfs"
)

func TestRestore_FinderInfoFlagsAndBirthTime(t *testing.T) {
	btime := time.Date(2001, 3, 24, 12, 0, 0, 0, time.UTC)
	finderInfo := make([]byte, 32)
	finderInfo[8] = 0x20 // kHasBundle
	finderInfo[9] = 0x0c // color label

	root := mockfs.NewDirectory()
	root.AddDir("Bundle", 0o755).SetExtendedAttributes(&fs.ExtendedAttributes{
		Xattrs:    map[string][]byte{"com.apple.FinderInfo": finderInfo},
		BirthTime: fs.UTCTimestampFromTime(btime),
	})
	root.AddFile("hidden", []byte("x"), 0o644).SetExtendedAttributes(&fs.ExtendedAttributes{
		BSDFlags:  unix.UF_HIDDEN,
		BirthTime: fs.UTCTimestampFromTime(btime),
	})
	root.AddSymlink("link", "hidden", 0o755).SetExtendedAttributes(&fs.ExtendedAttributes{
		Xattrs:   map[string][]byte{"user.link": []byte("l")},
		BSDFlags: unix.UF_HIDDEN,
	})

	target, o := restoreMock(t, root, nil)

	da := localAttrs(t, filepath.Join(target, "Bundle"))
	require.Equal(t, finderInfo, da.Xattrs["com.apple.FinderInfo"])
	require.Equal(t, btime, da.BirthTime.ToTime().UTC())

	fa := localAttrs(t, filepath.Join(target, "hidden"))
	require.Equal(t, uint32(unix.UF_HIDDEN), fa.BSDFlags)
	require.Equal(t, btime, fa.BirthTime.ToTime().UTC())

	la := localAttrs(t, filepath.Join(target, "link"))
	require.Equal(t, []byte("l"), la.Xattrs["user.link"])
	require.Equal(t, uint32(unix.UF_HIDDEN), la.BSDFlags)

	st, err := os.Lstat(filepath.Join(target, "link"))
	require.NoError(t, err)
	require.NotZero(t, st.Mode()&os.ModeSymlink, "flags are set on the link, not its target")

	require.Zero(t, o.warnings.count)
}

func TestRestore_SkipFileFlagsAndTimes(t *testing.T) {
	root := mockfs.NewDirectory()
	root.AddFile("f", []byte("x"), 0o644).SetExtendedAttributes(&fs.ExtendedAttributes{
		BSDFlags:  unix.UF_HIDDEN,
		BirthTime: fs.UTCTimestampFromTime(time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)),
	})

	target, _ := restoreMock(t, root, func(o *FilesystemOutput) {
		o.SkipFileFlags = true
		o.SkipTimes = true
	})

	a := localAttrs(t, filepath.Join(target, "f"))
	require.Zero(t, a.BSDFlags)
	require.NotEqual(t, 2001, a.BirthTime.ToTime().Year())
}

func TestRestore_ProtectedXattrsSkippedQuietly(t *testing.T) {
	root := mockfs.NewDirectory()
	root.AddFile("f", []byte("x"), 0o644).SetExtendedAttributes(&fs.ExtendedAttributes{Xattrs: map[string][]byte{
		"com.apple.macl": []byte("m"),
		"user.a":         []byte("1"),
	}})

	target, o := restoreMock(t, root, nil)

	a := localAttrs(t, filepath.Join(target, "f"))
	require.NotContains(t, a.Xattrs, "com.apple.macl")
	require.Equal(t, []byte("1"), a.Xattrs["user.a"])
	require.Zero(t, o.warnings.count)
}

func TestRestore_UnsettableFlagIsAWarning(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can set system flags")
	}

	root := mockfs.NewDirectory()
	root.AddFile("f", []byte("x"), 0o644).SetExtendedAttributes(&fs.ExtendedAttributes{BSDFlags: unix.SF_ARCHIVED})

	_, o := restoreMock(t, root, nil)

	require.Equal(t, 1, o.warnings.count)
}
