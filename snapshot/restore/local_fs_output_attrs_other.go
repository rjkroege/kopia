//go:build !darwin && !linux

package restore

import (
	"github.com/pkg/errors"

	"github.com/kopia/kopia/fs"
)

var (
	errXattrsNotSupported   = errors.New("extended attributes are not supported on this platform")
	errBSDFlagsNotSupported = errors.New("file flags are not supported on this platform")
)

func isProtectedXattr(string) bool {
	return false
}

func setXattr(string, string, []byte) error {
	return errXattrsNotSupported
}

func setBirthTime(string, fs.UTCTimestamp) error {
	return nil
}

func setBSDFlags(string, uint32, bool) error {
	return errBSDFlagsNotSupported
}
