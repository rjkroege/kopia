package localfs

import (
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"

	"github.com/kopia/kopia/fs"
)

// restorableBSDFlags are the st_flags bits worth capturing. Others are excluded because they
// describe storage state rather than user intent (UF_COMPRESSED, UF_TRACKED, SF_DATALESS for
// iCloud placeholders) or cannot be set by restore (SF_RESTRICTED, SF_FIRMLINK).
const restorableBSDFlags = unix.UF_NODUMP | unix.UF_IMMUTABLE | unix.UF_APPEND | unix.UF_OPAQUE |
	unix.UF_HIDDEN | unix.SF_ARCHIVED | unix.SF_IMMUTABLE | unix.SF_APPEND | unix.SF_NOUNLINK

// isUserXattr returns true for all names: macOS has no xattr namespaces.
func isUserXattr(string) bool {
	return true
}

func isNoAttrError(err error) bool {
	return errors.Is(err, unix.ENOATTR)
}

func flagsAndBirthTime(path string) (uint32, fs.UTCTimestamp, error) {
	var st unix.Stat_t
	if err := unix.Lstat(path, &st); err != nil {
		return 0, 0, errors.Wrap(err, "lstat")
	}

	return st.Flags & restorableBSDFlags, fs.UTCTimestamp(st.Btim.Nano()), nil
}
