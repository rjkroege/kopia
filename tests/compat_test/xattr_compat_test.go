//go:build darwin || linux

package compat_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/kopia/kopia/internal/testutil"
	"github.com/kopia/kopia/tests/testenv"
)

// TestSnapshotWithExtendedAttributesReadableBy017 verifies that clients predating extended
// metadata can list, verify and restore snapshots that contain it.
func TestSnapshotWithExtendedAttributesReadableBy017(t *testing.T) {
	t.Parallel()

	if kopiaCurrentExe == "" || kopia017exe == "" {
		t.Skip()
	}

	src := testutil.TempDirectory(t)
	file := filepath.Join(src, "tagged.txt")
	require.NoError(t, os.WriteFile(file, []byte("hello"), 0o600))

	err := unix.Lsetxattr(file, "user.kopia-compat", []byte("value"), 0)
	if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP) {
		t.Skipf("filesystem does not support xattrs: %v", err)
	}

	require.NoError(t, err)

	runner017 := testenv.NewExeRunnerWithBinary(t, kopia017exe)
	runnerCurrent := testenv.NewExeRunnerWithBinary(t, kopiaCurrentExe)

	old := testenv.NewCLITest(t, testenv.RepoFormatNotImportant, runner017)
	old.RunAndExpectSuccess(t, "repo", "create", "filesystem", "--path", old.RepoDir)

	cur := testenv.NewCLITest(t, testenv.RepoFormatNotImportant, runnerCurrent)
	cur.RunAndExpectSuccess(t, "repo", "connect", "filesystem", "--path", old.RepoDir)
	cur.RunAndExpectSuccess(t, "policy", "set", "--global", "--xattrs=all", "--file-flags=true", "--birth-time=true")
	cur.RunAndExpectSuccess(t, "snapshot", "create", src)

	var snaps []struct {
		ID        string `json:"id"`
		RootEntry struct {
			ObjectID string `json:"obj"`
		} `json:"rootEntry"`
	}

	require.NoError(t, json.Unmarshal([]byte(strings.Join(cur.RunAndExpectSuccess(t, "snapshot", "list", src, "--json"), "\n")), &snaps))
	require.Len(t, snaps, 1)

	snapID := snaps[0].ID

	// the root directory manifest carries the new field
	rootDir := strings.Join(cur.RunAndExpectSuccess(t, "show", snaps[0].RootEntry.ObjectID), "\n")
	require.Contains(t, rootDir, `"attrs"`)
	require.Contains(t, rootDir, "user.kopia-compat")

	old.RunAndExpectSuccess(t, "snapshot", "list", src)
	old.RunAndExpectSuccess(t, "snapshot", "verify", "--verify-files-percent=100")
	old.RunAndExpectSuccess(t, "policy", "show", "--global")

	dst := testutil.TempDirectory(t)
	old.RunAndExpectSuccess(t, "snapshot", "restore", snapID, dst)

	got, err := os.ReadFile(filepath.Join(dst, "tagged.txt"))
	require.NoError(t, err)
	require.Equal(t, []byte("hello"), got)
}
