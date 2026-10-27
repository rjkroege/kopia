package restore

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/kopia/kopia/fs"
	"github.com/kopia/kopia/fs/localfs"
	"github.com/kopia/kopia/internal/mockfs"
)

func TestRestore_FinderInfoFlagsAndBirthTime(t *testing.T) {
	btime := time.Date(2001, 3, 24, 12, 0, 0, 0, time.UTC)
	finderInfo := make([]byte, 32)
	finderInfo[8] = 0x20 // kHasBundle
	finderInfo[9] = 0x0c // color label

	root := mockfs.NewDirectory()
	root.AddDir("Bundle", 0o755).SetExtendedAttributes(fs.ExtendedAttributes{
		Xattrs:    map[string][]byte{"com.apple.FinderInfo": finderInfo},
		BirthTime: fs.UTCTimestampFromTime(btime),
	})
	root.AddFile("hidden", []byte("x"), 0o644).SetExtendedAttributes(fs.ExtendedAttributes{
		BSDFlags:  unix.UF_HIDDEN,
		BirthTime: fs.UTCTimestampFromTime(btime),
	})
	root.AddSymlink("link", "hidden", 0o755).SetExtendedAttributes(fs.ExtendedAttributes{
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

	require.Zero(t, o.warnings.warningCount())
}

func TestRestore_SymlinkAttributesAreIndependentOfTarget(t *testing.T) {
	targetBtime := time.Date(2001, 3, 24, 12, 0, 0, 0, time.UTC)
	linkBtime := time.Date(2011, 7, 20, 12, 0, 0, 0, time.UTC)

	root := mockfs.NewDirectory()
	root.AddFile("target", []byte("x"), 0o644).SetExtendedAttributes(fs.ExtendedAttributes{
		Xattrs:    map[string][]byte{"user.target": []byte("t")},
		BirthTime: fs.UTCTimestampFromTime(targetBtime),
	})
	root.AddSymlink("link", "target", 0o755).SetExtendedAttributes(fs.ExtendedAttributes{
		Xattrs:    map[string][]byte{"user.link": []byte("l")},
		BSDFlags:  unix.UF_HIDDEN,
		BirthTime: fs.UTCTimestampFromTime(linkBtime),
	})

	target, o := restoreMock(t, root, nil)

	ta := localAttrs(t, filepath.Join(target, "target"))
	require.Equal(t, []byte("t"), ta.Xattrs["user.target"])
	require.NotContains(t, ta.Xattrs, "user.link")
	require.Zero(t, ta.BSDFlags)
	require.Equal(t, targetBtime, ta.BirthTime.ToTime().UTC())

	la := localAttrs(t, filepath.Join(target, "link"))
	require.Equal(t, []byte("l"), la.Xattrs["user.link"])
	require.NotContains(t, la.Xattrs, "user.target")
	require.Equal(t, uint32(unix.UF_HIDDEN), la.BSDFlags)
	require.Equal(t, linkBtime, la.BirthTime.ToTime().UTC())

	require.Zero(t, o.warnings.warningCount())
}

func TestRestore_SkipFileFlagsAndTimes(t *testing.T) {
	root := mockfs.NewDirectory()
	root.AddFile("f", []byte("x"), 0o644).SetExtendedAttributes(fs.ExtendedAttributes{
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
	root.AddFile("f", []byte("x"), 0o644).SetExtendedAttributes(fs.ExtendedAttributes{Xattrs: map[string][]byte{
		"com.apple.macl": []byte("m"),
		"user.a":         []byte("1"),
	}})

	target, o := restoreMock(t, root, nil)

	a := localAttrs(t, filepath.Join(target, "f"))
	require.NotContains(t, a.Xattrs, "com.apple.macl")
	require.Equal(t, []byte("1"), a.Xattrs["user.a"])
	require.Zero(t, o.warnings.warningCount())
}

func TestRestore_UnsettableFlagIsAWarning(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can set system flags")
	}

	root := mockfs.NewDirectory()
	root.AddFile("f", []byte("x"), 0o644).SetExtendedAttributes(fs.ExtendedAttributes{BSDFlags: unix.SF_ARCHIVED})

	_, o := restoreMock(t, root, nil)

	require.Equal(t, 1, o.warnings.warningCount())
}

func aclListing(t *testing.T, path string) string {
	t.Helper()

	out, err := exec.CommandContext(t.Context(), "ls", "-led", path).CombinedOutput()
	require.NoError(t, err, string(out))

	lines := strings.SplitN(string(out), "\n", 2)
	require.Len(t, lines, 2)

	return lines[1]
}

func TestRestore_ACL(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src")
	require.NoError(t, os.WriteFile(src, []byte("x"), 0o600))

	out, err := exec.CommandContext(t.Context(), "chmod", "+a", "everyone allow readattr", src).CombinedOutput()
	require.NoError(t, err, string(out))

	le, err := localfs.NewEntry(src)
	require.NoError(t, err)

	attrs, err := fs.ReadExtendedAttributes(t.Context(), le, fs.AttributeOptions{ACLs: true})
	require.NoError(t, err)
	require.NotNil(t, attrs)

	for _, skip := range []bool{false, true} {
		root := mockfs.NewDirectory()
		root.AddFile("f", []byte("x"), 0o644).SetExtendedAttributes(*attrs)

		target, o := restoreMock(t, root, func(o *FilesystemOutput) { o.SkipPermissions = skip })

		if skip {
			require.NotContains(t, aclListing(t, filepath.Join(target, "f")), "allow readattr")
		} else {
			require.Equal(t, aclListing(t, src), aclListing(t, filepath.Join(target, "f")))
		}

		require.Zero(t, o.warnings.warningCount())
	}
}
