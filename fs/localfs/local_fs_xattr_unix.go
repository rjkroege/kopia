//go:build darwin || linux

package localfs

import (
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"

	"github.com/kopia/kopia/fs"
)

// maxXattrReadAttempts bounds retries when a value grows between sizing and reading it.
const maxXattrReadAttempts = 3

func readXattrs(path string, opts fs.AttributeOptions, a *fs.ExtendedAttributes) error {
	names, err := listXattrs(path)
	if err != nil {
		return err
	}

	limit := opts.MaxXattrValueSize()

	for _, name := range names {
		if !includeXattr(name, opts) {
			continue
		}

		v, tooLarge, err := getXattr(path, name, limit)

		switch {
		case isNoAttrError(err):
			// removed since it was listed
			continue
		case err != nil:
			return errors.Wrapf(err, "getxattr %q", name)
		case tooLarge:
			a.SkippedXattrs = append(a.SkippedXattrs, name)
			continue
		}

		if a.Xattrs == nil {
			a.Xattrs = map[string][]byte{}
		}

		a.Xattrs[name] = v
	}

	return nil
}

func listXattrs(path string) ([]string, error) {
	for range maxXattrReadAttempts {
		sz, err := unix.Llistxattr(path, nil)
		if err != nil {
			if isNotSupportedError(err) {
				return nil, nil
			}

			return nil, errors.Wrap(err, "listxattr")
		}

		if sz == 0 {
			return nil, nil
		}

		buf := make([]byte, sz)

		sz, err = unix.Llistxattr(path, buf)
		if errors.Is(err, unix.ERANGE) {
			continue
		}

		if err != nil {
			return nil, errors.Wrap(err, "listxattr")
		}

		var names []string

		for n := range strings.SplitSeq(string(buf[:sz]), "\x00") {
			if n != "" {
				names = append(names, n)
			}
		}

		return names, nil
	}

	return nil, errors.Wrap(unix.ERANGE, "listxattr")
}

// getXattr returns the value of the named attribute, or tooLarge if it exceeds limit bytes.
func getXattr(path, name string, limit int) (value []byte, tooLarge bool, err error) {
	for range maxXattrReadAttempts {
		sz, err := unix.Lgetxattr(path, name, nil)
		if err != nil {
			return nil, false, err //nolint:wrapcheck
		}

		if sz > limit {
			return nil, true, nil
		}

		buf := make([]byte, sz)
		if sz == 0 {
			return buf, false, nil
		}

		sz, err = unix.Lgetxattr(path, name, buf)
		if errors.Is(err, unix.ERANGE) {
			continue
		}

		if err != nil {
			return nil, false, err //nolint:wrapcheck
		}

		return buf[:sz], false, nil
	}

	return nil, false, unix.ERANGE
}

func isNotSupportedError(err error) bool {
	return errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP)
}

// includeXattr returns true if the named attribute should be captured under opts.
func includeXattr(name string, opts fs.AttributeOptions) bool {
	if opts.IsXattrExcluded(name) {
		return false
	}

	return opts.Xattrs == fs.XattrsAll || isUserXattr(name)
}
