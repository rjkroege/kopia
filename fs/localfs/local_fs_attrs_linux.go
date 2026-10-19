package localfs

import (
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"

	"github.com/kopia/kopia/fs"
)

// isUserXattr returns true for names in the "user." namespace, the only one unprivileged
// processes can generally restore.
func isUserXattr(name string) bool {
	return strings.HasPrefix(name, "user.")
}

func isNoAttrError(err error) bool {
	return errors.Is(err, unix.ENODATA)
}

// flagsAndBirthTime returns no flags (Linux inode flags are not BSD flags) and the birth time
// when the filesystem reports one.
func flagsAndBirthTime(path string) (uint32, fs.UTCTimestamp, error) {
	var st unix.Statx_t

	err := unix.Statx(unix.AT_FDCWD, path, unix.AT_SYMLINK_NOFOLLOW, unix.STATX_BTIME, &st)
	if errors.Is(err, unix.ENOSYS) {
		return 0, 0, nil
	}

	if err != nil {
		return 0, 0, errors.Wrap(err, "statx")
	}

	if st.Mask&unix.STATX_BTIME == 0 {
		return 0, 0, nil
	}

	return 0, fs.UTCTimestamp(st.Btime.Sec*1e9 + int64(st.Btime.Nsec)), nil
}
