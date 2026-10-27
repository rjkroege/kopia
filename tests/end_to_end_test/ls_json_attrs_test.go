//go:build darwin || linux

package endtoend_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestList_JSONShowsExtendedAttributes(t *testing.T) {
	t.Parallel()

	src := newExtendedMetadataSource(t)
	e := newExtendedMetadataEnv(t)

	defer e.RunAndExpectSuccess(t, "repo", "disconnect")

	e.RunAndExpectSuccess(t, "policy", "set", "--global", "--xattrs=all")
	e.RunAndExpectSuccess(t, "snapshot", "create", src)

	root := latestSnapshotID(t, e, src)

	for _, args := range [][]string{{"-r"}, {"-r", "-o"}, {"-r", "-l"}} {
		entries := listJSON(t, e, append(args, "-@", root)...)

		dir := jsonEntryNamed(t, entries, "Notes.rtfd/")
		require.Contains(t, dir, "ExtendedAttributes", args)
		dirAttrs, ok := dir["ExtendedAttributes"].(map[string]any)
		require.True(t, ok, args)
		require.Equal(t, map[string]any{"user.dir": float64(len("directory"))}, userXattrs(dirAttrs))

		file := jsonEntryNamed(t, entries, "Notes.rtfd/TXT.rtf")
		fileAttrs, ok := file["ExtendedAttributes"].(map[string]any)
		require.True(t, ok, args)
		require.Equal(t, map[string]any{
			"user.text":   float64(5),
			"user.binary": float64(4),
			"user.empty":  float64(0),
		}, userXattrs(fileAttrs))
	}

	for _, e := range listJSON(t, e, "-r", root) {
		require.NotContains(t, e, "ExtendedAttributes", "only with -@")
	}
}

// userXattrs returns the user.* entries of the Xattrs map of a JSON ExtendedAttributes object.
// The system may add others, such as com.apple.provenance on macOS.
func userXattrs(attrs map[string]any) map[string]any {
	all, _ := attrs["Xattrs"].(map[string]any)
	result := map[string]any{}

	for name, size := range all {
		if strings.HasPrefix(name, "user.") {
			result[name] = size
		}
	}

	return result
}
