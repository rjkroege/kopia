package localfs

import (
	"context"

	"github.com/pkg/errors"

	"github.com/kopia/kopia/fs"
)

// ExtendedAttributes implements fs.Entry. Symbolic links are not followed. Attributes larger
// than fs.MaxXattrSize are not captured and are logged.
func (e *filesystemEntry) ExtendedAttributes(ctx context.Context, opts fs.AttributeOptions) (*fs.ExtendedAttributes, error) {
	if opts.IsZero() {
		return nil, nil
	}

	path := e.fullPath()
	a := &fs.ExtendedAttributes{}

	if opts.Xattrs != fs.XattrsNone {
		tooLarge, err := readXattrs(path, opts, a)
		if err != nil {
			return nil, errors.Wrap(err, "unable to read extended attributes")
		}

		if len(tooLarge) > 0 {
			log(ctx).Warnw("extended attributes not captured because they exceed the size limit",
				"path", path, "names", tooLarge, "limit", fs.MaxXattrSize)
		}
	}

	if opts.BSDFlags || opts.BirthTime {
		flags, btime, err := flagsAndBirthTime(path)
		if err != nil {
			return nil, errors.Wrap(err, "unable to read file flags and birth time")
		}

		if opts.BSDFlags {
			a.BSDFlags = flags
		}

		if opts.BirthTime {
			a.BirthTime = btime
		}
	}

	if a.IsEmpty() {
		return nil, nil
	}

	return a, nil
}
