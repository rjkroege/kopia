package endtoend_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kopia/kopia/internal/testutil"
	"github.com/kopia/kopia/tests/clitestutil"
	"github.com/kopia/kopia/tests/testenv"
)

// listJSON runs "kopia ls -j" with args and decodes its output.
func listJSON(t *testing.T, e *testenv.CLITest, args ...string) []map[string]any {
	t.Helper()

	out := strings.Join(e.RunAndExpectSuccess(t, append([]string{"ls", "-j"}, args...)...), "\n")

	var entries []map[string]any

	require.NoError(t, json.Unmarshal([]byte(out), &entries), "output: %v", out)

	return entries
}

// jsonEntryNamed returns the entry whose DisplayName is the relative path name, with or
// without the root prefix that ls adds without --long.
func jsonEntryNamed(t *testing.T, entries []map[string]any, name string) map[string]any {
	t.Helper()

	for _, e := range entries {
		if dn, _ := e["DisplayName"].(string); dn == name || strings.HasSuffix(dn, "/"+name) {
			return e
		}
	}

	require.Failf(t, "entry not found", "%q not in %v", name, entries)

	return nil
}

func TestList_JSON(t *testing.T) {
	t.Parallel()

	src := testutil.TempDirectory(t)
	require.NoError(t, os.Mkdir(filepath.Join(src, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "sub", "file.txt"), []byte("hello"), 0o644))

	e := testenv.NewCLITest(t, testenv.RepoFormatNotImportant, testenv.NewInProcRunner(t))
	e.RunAndExpectSuccess(t, "repo", "create", "filesystem", "--path", e.RepoDir)

	defer e.RunAndExpectSuccess(t, "repo", "disconnect")

	e.RunAndExpectSuccess(t, "snapshot", "create", src)

	si := clitestutil.ListSnapshotsAndExpectSuccess(t, e, src)
	require.Len(t, si, 1)
	require.Len(t, si[0].Snapshots, 1)

	root := si[0].Snapshots[0].SnapshotID

	entries := listJSON(t, e, root)
	require.Len(t, entries, 1)
	require.Equal(t, map[string]any{"DisplayName": "sub", "ErrorSummary": ""}, entries[0])

	entries = listJSON(t, e, "-o", root)
	require.Len(t, entries, 1)
	require.ElementsMatch(t, []string{"Oid", "DisplayName", "ErrorSummary"}, keysOf(entries[0]))
	require.NotEmpty(t, entries[0]["Oid"])

	entries = listJSON(t, e, "-l", "-r", root)
	require.Len(t, entries, 2)

	file := jsonEntryNamed(t, entries, "sub/file.txt")
	require.ElementsMatch(t, []string{"Mode", "Size", "Time", "Oid", "DisplayName", "ErrorSummary"}, keysOf(file))
	require.InDelta(t, 5, file["Size"], 0)
}

func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	return keys
}
