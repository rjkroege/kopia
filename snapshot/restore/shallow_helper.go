package restore

import (
	"strings"

	"github.com/kopia/kopia/fs/localfs"
)

// PathIfPlaceholder returns the placeholder suffix trimmed from path and
// true if path is a placeholder directory or file path. Otherwise,
// returns path unchanged and false.
func PathIfPlaceholder(path string) string {
	if strings.HasSuffix(path, localfs.SHALLOWENTRYSUFFIX) {
		return localfs.TrimShallowSuffix(path)
	}

	return ""
}
