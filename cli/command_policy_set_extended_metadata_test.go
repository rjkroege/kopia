package cli_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kopia/kopia/internal/testutil"
	"github.com/kopia/kopia/tests/testenv"
)

func TestSetExtendedMetadataPolicy(t *testing.T) {
	e := testenv.NewCLITest(t, testenv.RepoFormatNotImportant, testenv.NewInProcRunner(t))
	defer e.RunAndExpectSuccess(t, "repo", "disconnect")

	e.RunAndExpectSuccess(t, "repo", "create", "filesystem", "--path", e.RepoDir)

	lines := compressSpaces(e.RunAndExpectSuccess(t, "policy", "show", "--global"))
	require.Contains(t, lines, " Extended attributes: none (defined for this target)")
	require.Contains(t, lines, " com.apple.provenance")
	require.Contains(t, lines, " File flags: false (defined for this target)")
	require.Contains(t, lines, " Birth time: false (defined for this target)")
	require.Contains(t, lines, " ACLs: false (defined for this target)")

	td := testutil.TempDirectory(t)

	e.RunAndExpectSuccess(t, "policy", "set", td, "--xattrs=all", "--file-flags=true", "--birth-time=true",
		"--acls=true", "--add-xattr-exclude=user.skip*")

	lines = compressSpaces(e.RunAndExpectSuccess(t, "policy", "show", td))
	require.Contains(t, lines, " Extended attributes: all (defined for this target)")
	require.Contains(t, lines, " user.skip*")
	require.NotContains(t, lines, " com.apple.provenance", "a non-empty exclude list replaces the inherited one")
	require.Contains(t, lines, " File flags: true (defined for this target)")
	require.Contains(t, lines, " Birth time: true (defined for this target)")
	require.Contains(t, lines, " ACLs: true (defined for this target)")

	e.RunAndExpectSuccess(t, "policy", "set", td, "--xattrs=inherit", "--clear-xattr-exclude", "--file-flags=inherit")

	lines = compressSpaces(e.RunAndExpectSuccess(t, "policy", "show", td))
	require.Contains(t, lines, " Extended attributes: none inherited from (global)")
	require.Contains(t, lines, " com.apple.provenance")
	require.Contains(t, lines, " File flags: false inherited from (global)")

	e.RunAndExpectFailure(t, "policy", "set", td, "--xattrs=some")
}
