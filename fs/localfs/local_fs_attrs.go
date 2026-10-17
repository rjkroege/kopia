package localfs

import (
	"context"
	"slices"

	"github.com/pkg/errors"

	"github.com/kopia/kopia/fs"
)

// ExtendedAttributes implements fs.ExtendedAttributesReader. Symbolic links are not followed.
func (e *filesystemEntry) ExtendedAttributes(_ context.Context, opts fs.AttributeOptions) (*fs.ExtendedAttributes, error) {
	if opts.IsZero() {
		return nil, nil
	}

	path := e.fullPath()
	a := &fs.ExtendedAttributes{}

	if opts.XattrsEnabled() || opts.ACLs {
		if err := readXattrs(path, opts, a); err != nil {
			return nil, errors.Wrap(err, "unable to read extended attributes")
		}
	}

	if opts.ACLs {
		acl, err := readACL(path)
		if err != nil {
			return nil, errors.Wrap(err, "unable to read ACL")
		}

		a.ACL = acl
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

	slices.Sort(a.SkippedXattrs)

	if a.IsEmpty() {
		return nil, nil
	}

	return a, nil
}
