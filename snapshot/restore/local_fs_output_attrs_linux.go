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

func setXattr(path, name string, value []byte) error {
	//nolint:wrapcheck
	return unix.Lsetxattr(path, name, value, 0)
}

// setBirthTime does nothing: Linux does not allow setting the birth time.
func setBirthTime(string, fs.UTCTimestamp) error {
	return nil
}

func setBSDFlags(string, uint32, bool) error {
	return errBSDFlagsNotSupported
}
