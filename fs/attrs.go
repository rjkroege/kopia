package fs

import (
	"bytes"
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/pkg/errors"
)

// DefaultMaxXattrSize is the default limit on the size of a single extended attribute value
// that is captured. Larger values are skipped and recorded in ExtendedAttributes.SkippedXattrs.
const DefaultMaxXattrSize = 8 << 10

// ExtendedAttributes holds metadata of a filesystem entry beyond what os.FileInfo,
// OwnerInfo and DeviceInfo describe. The zero value means no extended metadata.
type ExtendedAttributes struct {
	// Xattrs maps extended attribute names to their values.
	Xattrs map[string][]byte `json:"x,omitempty"`

	// SkippedXattrs lists names of extended attributes that were present but not captured,
	// for example because their values exceeded the size limit.
	SkippedXattrs []string `json:"xskip,omitempty"`

	// BSDFlags holds BSD file flags (st_flags, as set by chflags(1)).
	BSDFlags uint32 `json:"flags,omitempty"`

	// BirthTime is the creation time of the entry, zero if unknown.
	BirthTime UTCTimestamp `json:"btime,omitempty"`
}

// IsEmpty returns true if a carries no metadata. A nil receiver is empty.
func (a *ExtendedAttributes) IsEmpty() bool {
	return a == nil || (len(a.Xattrs) == 0 && len(a.SkippedXattrs) == 0 && a.BSDFlags == 0 && a.BirthTime == 0)
}

// Clone returns a deep copy of a, or nil if a is nil.
func (a *ExtendedAttributes) Clone() *ExtendedAttributes {
	if a == nil {
		return nil
	}

	c := *a
	c.SkippedXattrs = slices.Clone(a.SkippedXattrs)

	if a.Xattrs != nil {
		c.Xattrs = make(map[string][]byte, len(a.Xattrs))
		for k, v := range a.Xattrs {
			c.Xattrs[k] = bytes.Clone(v)
		}
	}

	return &c
}

// Equal returns true if a and b carry the same metadata. Nil and empty values are equal.
func (a *ExtendedAttributes) Equal(b *ExtendedAttributes) bool {
	if a.IsEmpty() || b.IsEmpty() {
		return a.IsEmpty() == b.IsEmpty()
	}

	return a.BSDFlags == b.BSDFlags &&
		a.BirthTime == b.BirthTime &&
		slices.Equal(a.SkippedXattrs, b.SkippedXattrs) &&
		maps.EqualFunc(a.Xattrs, b.Xattrs, bytes.Equal)
}

// XattrMode selects which extended attributes are captured.
type XattrMode string

// Supported extended attribute capture modes.
const (
	// XattrsNone captures no extended attributes.
	XattrsNone XattrMode = "none"

	// XattrsUser captures attributes that unprivileged users can set: the "user." namespace
	// on Linux and all attributes on macOS, which has no namespaces.
	XattrsUser XattrMode = "user"

	// XattrsAll captures attributes in all namespaces that the process can read.
	XattrsAll XattrMode = "all"
)

// AttributeOptions controls which extended metadata is read from an entry.
// The zero value reads nothing.
type AttributeOptions struct {
	// Xattrs selects which extended attributes to read. Empty means XattrsNone.
	Xattrs XattrMode

	// ExcludeXattrs lists attribute names to skip. A name ending in "*" matches as a prefix.
	ExcludeXattrs []string

	// MaxXattrSize limits the size of a single value; 0 means DefaultMaxXattrSize.
	MaxXattrSize int

	// BSDFlags enables reading BSD file flags.
	BSDFlags bool

	// BirthTime enables reading the creation time.
	BirthTime bool
}

// IsZero returns true if o requests no extended metadata.
func (o AttributeOptions) IsZero() bool {
	return (o.Xattrs == "" || o.Xattrs == XattrsNone) && !o.BSDFlags && !o.BirthTime
}

// MaxXattrValueSize returns the effective maximum size of a single attribute value.
func (o AttributeOptions) MaxXattrValueSize() int {
	if o.MaxXattrSize <= 0 {
		return DefaultMaxXattrSize
	}

	return o.MaxXattrSize
}

// IsXattrExcluded returns true if name matches an entry in ExcludeXattrs.
func (o AttributeOptions) IsXattrExcluded(name string) bool {
	for _, pat := range o.ExcludeXattrs {
		if prefix, ok := strings.CutSuffix(pat, "*"); ok {
			if strings.HasPrefix(name, prefix) {
				return true
			}

			continue
		}

		if name == pat {
			return true
		}
	}

	return false
}

// ExtendedAttributesReader is implemented by entries that can report extended metadata.
type ExtendedAttributesReader interface {
	// ExtendedAttributes returns the metadata selected by opts, or nil if there is none.
	ExtendedAttributes(ctx context.Context, opts AttributeOptions) (*ExtendedAttributes, error)
}

// ReadExtendedAttributes returns the extended metadata of e selected by opts. It returns nil
// without error when opts selects nothing, e does not support extended metadata, or e has none.
func ReadExtendedAttributes(ctx context.Context, e Entry, opts AttributeOptions) (*ExtendedAttributes, error) {
	if opts.IsZero() {
		return nil, nil
	}

	r, ok := e.(ExtendedAttributesReader)
	if !ok {
		return nil, nil
	}

	a, err := r.ExtendedAttributes(ctx, opts)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to read extended attributes of %v", e.Name())
	}

	if a.IsEmpty() {
		return nil, nil
	}

	return a, nil
}

// Filtered returns a copy of a with only the metadata selected by opts: xattrs are dropped
// unless opts selects some, excluded names are removed, and values larger than the size limit
// are moved to SkippedXattrs. Namespace selection (XattrsUser) is platform-specific and is
// not applied. Returns nil if nothing remains.
func (a *ExtendedAttributes) Filtered(opts AttributeOptions) *ExtendedAttributes {
	if a.IsEmpty() || opts.IsZero() {
		return nil
	}

	c := &ExtendedAttributes{}

	if opts.BSDFlags {
		c.BSDFlags = a.BSDFlags
	}

	if opts.BirthTime {
		c.BirthTime = a.BirthTime
	}

	if opts.Xattrs != "" && opts.Xattrs != XattrsNone {
		for _, name := range a.SkippedXattrs {
			if !opts.IsXattrExcluded(name) {
				c.SkippedXattrs = append(c.SkippedXattrs, name)
			}
		}

		for name, v := range a.Xattrs {
			switch {
			case opts.IsXattrExcluded(name):
			case len(v) > opts.MaxXattrValueSize():
				c.SkippedXattrs = append(c.SkippedXattrs, name)
			default:
				if c.Xattrs == nil {
					c.Xattrs = map[string][]byte{}
				}

				c.Xattrs[name] = bytes.Clone(v)
			}
		}

		slices.Sort(c.SkippedXattrs)
	}

	if c.IsEmpty() {
		return nil
	}

	return c
}
