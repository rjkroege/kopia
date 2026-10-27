package policy

import (
	"github.com/pkg/errors"

	"github.com/kopia/kopia/fs"
	"github.com/kopia/kopia/snapshot"
)

// ExtendedMetadataPolicy describes which metadata beyond contents, permissions, ownership and
// modification time is captured in snapshots. Capturing it is disabled by default.
type ExtendedMetadataPolicy struct {
	// Xattrs selects which extended attributes are captured; nil means inherit.
	Xattrs *fs.XattrMode `json:"xattrs,omitempty"`

	// XattrExclude lists extended attribute names that are never captured. A non-empty list
	// replaces the inherited one.
	XattrExclude []string `json:"xattrExclude,omitempty"`

	// FileFlags enables capturing BSD file flags (chflags).
	FileFlags *OptionalBool `json:"fileFlags,omitempty"`

	// BirthTime enables capturing file creation times.
	BirthTime *OptionalBool `json:"birthTime,omitempty"`

	// ACLs enables capturing access control lists (macOS extended security, Linux POSIX ACLs).
	ACLs *OptionalBool `json:"acls,omitempty"`
}

// ExtendedMetadataPolicyDefinition specifies which policy definition provided the value of a particular field.
type ExtendedMetadataPolicyDefinition struct {
	Xattrs       snapshot.SourceInfo `json:"xattrs,omitempty"`
	XattrExclude snapshot.SourceInfo `json:"xattrExclude,omitempty"`
	FileFlags    snapshot.SourceInfo `json:"fileFlags,omitempty"`
	BirthTime    snapshot.SourceInfo `json:"birthTime,omitempty"`
	ACLs         snapshot.SourceInfo `json:"acls,omitempty"`
}

// Merge applies default values from the provided policy.
func (p *ExtendedMetadataPolicy) Merge(src ExtendedMetadataPolicy, def *ExtendedMetadataPolicyDefinition, si snapshot.SourceInfo) {
	mergeXattrMode(&p.Xattrs, src.Xattrs, &def.Xattrs, si)
	mergeStringsReplace(&p.XattrExclude, src.XattrExclude, &def.XattrExclude, si)
	mergeOptionalBool(&p.FileFlags, src.FileFlags, &def.FileFlags, si)
	mergeOptionalBool(&p.BirthTime, src.BirthTime, &def.BirthTime, si)
	mergeOptionalBool(&p.ACLs, src.ACLs, &def.ACLs, si)
}

// AttributeOptions returns the options for reading extended metadata under this policy.
func (p *ExtendedMetadataPolicy) AttributeOptions() fs.AttributeOptions {
	return fs.AttributeOptions{
		Xattrs:        xattrModeOrDefault(p.Xattrs, fs.XattrsNone),
		ExcludeXattrs: p.XattrExclude,
		BSDFlags:      p.FileFlags.OrDefault(false),
		BirthTime:     p.BirthTime.OrDefault(false),
		ACLs:          p.ACLs.OrDefault(false),
	}
}

// ValidateExtendedMetadataPolicy returns an error if the policy contains invalid values.
func ValidateExtendedMetadataPolicy(p ExtendedMetadataPolicy) error {
	if p.Xattrs != nil && *p.Xattrs > fs.XattrsAll {
		return errors.Errorf("invalid xattr mode %d", *p.Xattrs)
	}

	return nil
}

// NewXattrMode returns a pointer to the provided extended attribute capture mode.
func NewXattrMode(m fs.XattrMode) *fs.XattrMode {
	return new(m)
}

func xattrModeOrDefault(m *fs.XattrMode, def fs.XattrMode) fs.XattrMode {
	if m == nil {
		return def
	}

	return *m
}

func mergeXattrMode(target **fs.XattrMode, src *fs.XattrMode, def *snapshot.SourceInfo, si snapshot.SourceInfo) {
	if *target == nil && src != nil {
		v := *src
		*target = &v
		*def = si
	}
}
