package endtoend_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/kopia/kopia/internal/testutil"
)

func runTool(ctx context.Context, t *testing.T, dir, name string, args ...string) (string, error) {
	t.Helper()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()

	return string(out), err
}

func mustRunTool(ctx context.Context, t *testing.T, dir, name string, args ...string) string {
	t.Helper()

	out, err := runTool(ctx, t, dir, name, args...)
	require.NoError(t, err, "%v %v: %s", name, args, out)

	return out
}

// xattrListing returns "xattr -lr ." for root without attributes the system adds by itself.
func xattrListing(ctx context.Context, t *testing.T, root string) string {
	t.Helper()

	var lines []string

	for line := range strings.SplitSeq(mustRunTool(ctx, t, root, "xattr", "-lr", "."), "\n") {
		ignored := false

		for name := range xattrsIgnoredInComparison {
			if strings.Contains(line, ": "+name+":") {
				ignored = true
			}
		}

		if !ignored && line != "" {
			lines = append(lines, line)
		}
	}

	return strings.Join(lines, "\n")
}

// statListing returns birth time, flags and mode of every entry under root.
func statListing(ctx context.Context, t *testing.T, root string) string {
	t.Helper()

	return mustRunTool(ctx, t, root, "find", ".", "-exec", "stat", "-f", "%N %B %Sf %Sp", "{}", "+")
}

// newMacOSMetadataSource creates a tree with Finder metadata, tags, flags and birth times.
func newMacOSMetadataSource(ctx context.Context, t *testing.T) string {
	t.Helper()

	src := testutil.TempDirectory(t)
	bundle := filepath.Join(src, "Drawing.pxd")
	tagged := filepath.Join(src, "tagged.txt")
	hidden := filepath.Join(src, "hidden.txt")

	require.NoError(t, os.MkdirAll(filepath.Join(bundle, "data"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bundle, "data", "layer"), []byte("pixels"), 0o644))
	require.NoError(t, os.WriteFile(tagged, []byte("tagged"), 0o644))
	require.NoError(t, os.WriteFile(hidden, []byte("hidden"), 0o644))

	finderInfo := make([]byte, 32)
	finderInfo[8] = 0x20 // kHasBundle: shown by Finder as a single item without a known extension
	setXattrOrSkip(t, bundle, "com.apple.FinderInfo", finderInfo)

	labelInfo := make([]byte, 32)
	labelInfo[9] = 0x0c // red label
	setXattrOrSkip(t, tagged, "com.apple.FinderInfo", labelInfo)

	tags := mustRunTool(ctx, t, src, "sh", "-c", `printf '<plist version="1.0"><array><string>Red
6</string></array></plist>' | plutil -convert binary1 -o - -`)
	setXattrOrSkip(t, tagged, "com.apple.metadata:_kMDItemUserTags", []byte(tags))

	require.NoError(t, unix.Chflags(hidden, unix.UF_HIDDEN))

	// an mtime earlier than the birth time moves the birth time back; then set a newer mtime
	mustRunTool(ctx, t, src, "touch", "-t", "200103241200", tagged)
	mustRunTool(ctx, t, src, "touch", "-t", "202001011200", tagged)

	return src
}

func TestSnapshotRestore_MacOSMetadataFidelity(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	src := newMacOSMetadataSource(ctx, t)
	e := newExtendedMetadataEnv(t)

	defer e.RunAndExpectSuccess(t, "repo", "disconnect")

	e.RunAndExpectSuccess(t, "policy", "set", "--global", "--xattrs=all", "--file-flags=true", "--birth-time=true")
	e.RunAndExpectSuccess(t, "snapshot", "create", src)

	snapID := latestSnapshotID(t, e, src)
	wantXattrs := xattrListing(ctx, t, src)
	wantStat := statListing(ctx, t, src)

	require.Contains(t, wantXattrs, "com.apple.FinderInfo")
	require.Contains(t, wantStat, "hidden")

	t.Run("local", func(t *testing.T) {
		dst := testutil.TempDirectory(t)
		e.RunAndExpectSuccess(t, "snapshot", "restore", snapID, dst)

		require.Equal(t, wantXattrs, xattrListing(ctx, t, dst))
		require.Equal(t, wantStat, statListing(ctx, t, dst))
	})

	t.Run("tar", func(t *testing.T) {
		archive := filepath.Join(testutil.TempDirectory(t), "out.tar")
		e.RunAndExpectSuccess(t, "snapshot", "restore", snapID, archive)

		dst := testutil.TempDirectory(t)
		mustRunTool(ctx, t, dst, "bsdtar", "-x", "--xattrs", "--fflags", "-f", archive)

		require.Equal(t, wantXattrs, xattrListing(ctx, t, dst))
	})
}

// newSignedCodeSource creates ad-hoc signed code: a Mach-O tool, a script (whose signature is
// stored in com.apple.cs.* xattrs) and an application bundle.
func newSignedCodeSource(ctx context.Context, t *testing.T) string {
	t.Helper()

	if _, err := exec.LookPath("codesign"); err != nil {
		t.Skip("codesign not found")
	}

	src := testutil.TempDirectory(t)
	app := filepath.Join(src, "Hello.app", "Contents")

	require.NoError(t, os.MkdirAll(filepath.Join(app, "MacOS"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(app, "Resources"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(app, "Info.plist"), []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleExecutable</key><string>Hello</string>
<key>CFBundleIdentifier</key><string>org.kopia.test.hello</string>
<key>CFBundleName</key><string>Hello</string>
<key>CFBundlePackageType</key><string>APPL</string>
</dict></plist>
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(app, "Resources", "data.txt"), []byte("data"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(src, "script.sh"), []byte("#!/bin/sh\necho signed\n"), 0o755))

	mustRunTool(ctx, t, src, "cp", "/usr/bin/true", "tool")
	mustRunTool(ctx, t, src, "cp", "/usr/bin/true", filepath.Join(app, "MacOS", "Hello"))

	mustRunTool(ctx, t, src, "codesign", "-f", "-s", "-", "tool")
	mustRunTool(ctx, t, src, "codesign", "-f", "-s", "-", "script.sh")
	mustRunTool(ctx, t, src, "codesign", "--deep", "-f", "-s", "-", "Hello.app")

	verifySignedCode(ctx, t, src)

	return src
}

func verifySignedCode(ctx context.Context, t *testing.T, root string) {
	t.Helper()

	mustRunTool(ctx, t, root, "codesign", "--verify", "--strict", "tool")
	mustRunTool(ctx, t, root, "codesign", "--verify", "--strict", "script.sh")
	mustRunTool(ctx, t, root, "codesign", "--verify", "--deep", "--strict", "Hello.app")

	// the restored code still runs
	mustRunTool(ctx, t, root, "./tool")
	mustRunTool(ctx, t, root, "./Hello.app/Contents/MacOS/Hello")
	require.Equal(t, "signed\n", mustRunTool(ctx, t, root, "./script.sh"))
}

func TestSnapshotRestore_SignedCode(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	src := newSignedCodeSource(ctx, t)
	e := newExtendedMetadataEnv(t)

	defer e.RunAndExpectSuccess(t, "repo", "disconnect")

	e.RunAndExpectSuccess(t, "policy", "set", "--global", "--xattrs=all", "--file-flags=true", "--birth-time=true")
	e.RunAndExpectSuccess(t, "snapshot", "create", src)

	snapID := latestSnapshotID(t, e, src)

	t.Run("with xattrs", func(t *testing.T) {
		dst := testutil.TempDirectory(t)
		e.RunAndExpectSuccess(t, "snapshot", "restore", snapID, dst)

		verifySignedCode(ctx, t, dst)
	})

	t.Run("skip xattrs breaks script signature", func(t *testing.T) {
		dst := testutil.TempDirectory(t)
		e.RunAndExpectSuccess(t, "snapshot", "restore", "--skip-xattrs", snapID, dst)

		// signatures embedded in files survive; the script's lives in xattrs and does not
		mustRunTool(ctx, t, dst, "codesign", "--verify", "--strict", "tool")
		mustRunTool(ctx, t, dst, "codesign", "--verify", "--deep", "--strict", "Hello.app")

		out, err := runTool(ctx, t, dst, "codesign", "--verify", "--strict", "script.sh")
		require.Error(t, err, "script signature must not verify without xattrs: %s", out)
	})
}

// TestSnapshotRestore_InstalledAppSignature restores a copy of an installed application and
// verifies its signature. KOPIA_TEST_SIGNED_APP selects the application (default Calculator).
// It is skipped when the copy is not permitted (app management protection).
func TestSnapshotRestore_InstalledAppSignature(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	appPath := os.Getenv("KOPIA_TEST_SIGNED_APP")
	if appPath == "" {
		appPath = "/System/Applications/Calculator.app"
	}

	ctx := t.Context()
	src := testutil.TempDirectory(t)
	app := filepath.Base(appPath)

	if out, err := runTool(ctx, t, src, "ditto", appPath, app); err != nil {
		t.Skipf("unable to copy %v: %v %s", appPath, err, out)
	}

	mustRunTool(ctx, t, src, "codesign", "--verify", "--deep", "--strict", app)

	e := newExtendedMetadataEnv(t)

	defer e.RunAndExpectSuccess(t, "repo", "disconnect")

	e.RunAndExpectSuccess(t, "policy", "set", "--global", "--xattrs=all", "--file-flags=true", "--birth-time=true")
	e.RunAndExpectSuccess(t, "snapshot", "create", src)

	dst := testutil.TempDirectory(t)
	e.RunAndExpectSuccess(t, "snapshot", "restore", latestSnapshotID(t, e, src), dst)

	mustRunTool(ctx, t, dst, "codesign", "--verify", "--deep", "--strict", app)
	require.Equal(t, xattrListing(ctx, t, src), xattrListing(ctx, t, dst))
}
