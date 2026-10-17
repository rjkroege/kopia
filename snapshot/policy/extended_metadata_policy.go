package policy

import (
	"github.com/pkg/errors"

	"github.com/kopia/kopia/fs"
	"github.com/kopia/kopia/snapshot"
)

// ExtendedMetadataPolicy describes which metadata beyond contents, permissions, ownership and
// modification time is captured in snapshots. The default policy captures all of it.
type ExtendedMetadataPolicy struct {
	// Xattrs selects which extended attributes are captured; empty means inherit.
	Xattrs fs.XattrMode `json:"xattrs,omitempty"`

	// XattrExclude lists extended attribute names that are never captured. A name ending in
	// "*" matches as a prefix. A non-empty list replaces the inherited one.
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
		Xattrs:        p.Xattrs,
		ExcludeXattrs: p.XattrExclude,
		BSDFlags:      p.FileFlags.OrDefault(false),
		BirthTime:     p.BirthTime.OrDefault(false),
		ACLs:          p.ACLs.OrDefault(false),
	}
}

// ValidateExtendedMetadataPolicy returns an error if the policy contains invalid values.
func ValidateExtendedMetadataPolicy(p ExtendedMetadataPolicy) error {
	switch p.Xattrs {
	case "", fs.XattrsNone, fs.XattrsUser, fs.XattrsAll:
		return nil
	default:
		return errors.Errorf("invalid xattr mode %q, must be one of %q, %q or %q", p.Xattrs, fs.XattrsNone, fs.XattrsUser, fs.XattrsAll)
	}
}

func mergeXattrMode(target *fs.XattrMode, src fs.XattrMode, def *snapshot.SourceInfo, si snapshot.SourceInfo) {
	if *target == "" && src != "" {
		*target = src
		*def = si
	}
}
