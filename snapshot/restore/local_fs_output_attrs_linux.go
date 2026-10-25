package restore

import (
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"

	"github.com/kopia/kopia/fs"
)

var errBSDFlagsNotSupported = errors.New("file flags are not supported on this platform")

func isProtectedXattr(string) bool {
	return false
}

// setXattr sets an extended attribute of path itself, not of a symlink's target. Linux
// only allows user.* attributes on regular files and directories, so setting one on a
// symlink fails with EPERM and is reported as a warning.
func setXattr(path, name string, value []byte) error {
	return errors.Wrapf(unix.Lsetxattr(path, name, value, 0), "lsetxattr %v", name)
}

// setBirthTime does nothing: Linux has no system call that sets the birth time (statx
// only reads it), so restored files get the time of the restore.
func setBirthTime(string, fs.UTCTimestamp) error {
	return nil
}

func setBSDFlags(string, uint32, bool) error {
	return errBSDFlagsNotSupported
}
