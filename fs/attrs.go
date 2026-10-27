package fs

import (
	"bytes"
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/pkg/errors"
)

// MaxXattrSize is the limit on the size of a single extended attribute value that is captured.
// Larger values are not captured.
const MaxXattrSize = 8 << 10

// ExtendedAttributes holds metadata of a filesystem entry beyond what os.FileInfo,
// OwnerInfo and DeviceInfo describe. The zero value means no extended metadata.
type ExtendedAttributes struct {
	// Xattrs maps extended attribute names to their values.
	Xattrs map[string][]byte `json:"x,omitempty"`

	// BSDFlags holds BSD file flags (st_flags, as set by chflags(1)).
	BSDFlags uint32 `json:"flags,omitempty"`

	// BirthTime is the creation time of the entry, zero if unknown.
	BirthTime UTCTimestamp `json:"btime,omitempty"`

	// ACL holds the macOS extended security information (kauth_filesec) of the entry.
	// POSIX ACLs on Linux are stored as system.posix_acl_* entries in Xattrs instead.
	ACL []byte `json:"acl,omitempty"`
}

// IsEmpty returns true if a carries no metadata. A nil receiver is empty.
func (a *ExtendedAttributes) IsEmpty() bool {
	return a == nil || (len(a.Xattrs) == 0 && a.BSDFlags == 0 && a.BirthTime == 0 && len(a.ACL) == 0)
}

// Clone returns a deep copy of a, or nil if a is nil.
func (a *ExtendedAttributes) Clone() *ExtendedAttributes {
	if a == nil {
		return nil
	}

	c := *a
	c.ACL = bytes.Clone(a.ACL)

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
		bytes.Equal(a.ACL, b.ACL) &&
		maps.EqualFunc(a.Xattrs, b.Xattrs, bytes.Equal)
}

// XattrMode selects which extended attributes are captured.
type XattrMode byte

// Supported extended attribute capture modes.
const (
	// XattrsNone captures no extended attributes.
	XattrsNone XattrMode = iota

	// XattrsUser captures attributes that unprivileged users can set: the "user." namespace
	// on Linux and all attributes on macOS, which has no namespaces.
	XattrsUser

	// XattrsAll captures attributes in all namespaces that the process can read.
	XattrsAll
)

// Extended attribute capture mode strings, as used in policies and on the command line.
const (
	XattrsNoneString = "none"
	XattrsUserString = "user"
	XattrsAllString  = "all"
)

func (m XattrMode) String() string {
	switch m {
	case XattrsUser:
		return XattrsUserString
	case XattrsAll:
		return XattrsAllString
	default:
		return XattrsNoneString
	}
}

// MarshalText implements encoding.TextMarshaler.
func (m XattrMode) MarshalText() ([]byte, error) {
	return []byte(m.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (m *XattrMode) UnmarshalText(b []byte) error {
	switch string(b) {
	case XattrsNoneString:
		*m = XattrsNone
	case XattrsUserString:
		*m = XattrsUser
	case XattrsAllString:
		*m = XattrsAll
	default:
		return errors.Errorf("invalid extended attribute mode %q, must be one of %v, %v or %v",
			b, XattrsNoneString, XattrsUserString, XattrsAllString)
	}

	return nil
}

// AttributeOptions controls which extended metadata is read from an entry.
// The zero value reads nothing.
type AttributeOptions struct {
	// Xattrs selects which extended attributes to read.
	Xattrs XattrMode

	// ExcludeXattrs lists attribute names to skip.
	ExcludeXattrs []string

	// BSDFlags enables reading BSD file flags.
	BSDFlags bool

	// BirthTime enables reading the creation time.
	BirthTime bool

	// ACLs enables reading access control lists: the macOS extended security information and,
	// on Linux, the system.posix_acl_* attributes. They are governed by this option alone,
	// regardless of Xattrs.
	ACLs bool
}

// IsZero returns true if o requests no extended metadata.
func (o AttributeOptions) IsZero() bool {
	return o.Xattrs == XattrsNone && !o.BSDFlags && !o.BirthTime && !o.ACLs
}

// IncludesXattr returns true if the named attribute is selected by o, ignoring the size limit
// and platform-specific namespaces.
func (o AttributeOptions) IncludesXattr(name string) bool {
	if IsPOSIXACLXattr(name) {
		return o.ACLs
	}

	return o.Xattrs != XattrsNone && !o.IsXattrExcluded(name)
}

// IsPOSIXACLXattr returns true for the extended attributes Linux uses to store POSIX ACLs.
func IsPOSIXACLXattr(name string) bool {
	return strings.HasPrefix(name, "system.posix_acl_")
}

// IsXattrExcluded returns true if name is listed in ExcludeXattrs. The list is short, so a
// linear scan is cheaper than building a set.
func (o AttributeOptions) IsXattrExcluded(name string) bool {
	return slices.Contains(o.ExcludeXattrs, name)
}

// ReadExtendedAttributes returns the extended metadata of e selected by opts. It returns nil
// without error when opts selects nothing or e has no extended metadata.
func ReadExtendedAttributes(ctx context.Context, e Entry, opts AttributeOptions) (*ExtendedAttributes, error) {
	if opts.IsZero() {
		return nil, nil
	}

	a, err := e.ExtendedAttributes(ctx, opts)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to read extended attributes of %v", e.Name())
	}

	if a.IsEmpty() {
		return nil, nil
	}

	return a, nil
}

// Filtered returns the metadata of a selected by opts: xattrs are dropped unless opts selects
// them (see IncludesXattr). Namespace selection (XattrsUser) is platform-specific and is not
// applied. Returns nil if nothing remains.
//
// Filtered is for metadata that was already captured, such as metadata stored in a snapshot.
// When opts drops nothing it returns a itself, and otherwise the result shares xattr values
// and the ACL with a, so the result must not be modified.
func (a *ExtendedAttributes) Filtered(opts AttributeOptions) *ExtendedAttributes {
	if a.IsEmpty() || opts.IsZero() {
		return nil
	}

	if a.selectedBy(opts) {
		return a
	}

	c := &ExtendedAttributes{}

	if opts.BSDFlags {
		c.BSDFlags = a.BSDFlags
	}

	if opts.BirthTime {
		c.BirthTime = a.BirthTime
	}

	if opts.ACLs {
		c.ACL = a.ACL
	}

	for name, v := range a.Xattrs {
		if !opts.IncludesXattr(name) {
			continue
		}

		if c.Xattrs == nil {
			c.Xattrs = map[string][]byte{}
		}

		c.Xattrs[name] = v
	}

	if c.IsEmpty() {
		return nil
	}

	return c
}

// selectedBy returns true if opts selects all metadata in a, so that filtering drops nothing.
func (a *ExtendedAttributes) selectedBy(opts AttributeOptions) bool {
	if (a.BSDFlags != 0 && !opts.BSDFlags) || (a.BirthTime != 0 && !opts.BirthTime) ||
		(len(a.ACL) != 0 && !opts.ACLs) {
		return false
	}

	for name := range a.Xattrs {
		if !opts.IncludesXattr(name) {
			return false
		}
	}

	return true
}
