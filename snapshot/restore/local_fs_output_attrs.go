package restore

import (
	"context"
	"maps"
	"slices"
	"sync"

	"github.com/pkg/errors"

	"github.com/kopia/kopia/fs"
)

// maxAttributeWarningExamples limits how many distinct failures are listed in the summary.
const maxAttributeWarningExamples = 5

// attributeWarnings collects failures to restore extended metadata. They do not fail the
// restore, since the target filesystem may not support them, and are summarized on Close.
type attributeWarnings struct {
	mu sync.Mutex

	// +checklocks:mu
	count int
	// +checklocks:mu
	examples map[string]string // error message -> example path
}

func (w *attributeWarnings) add(ctx context.Context, path string, err error) {
	log(ctx).Debugf("unable to restore extended metadata of %v: %v", path, err)

	if w == nil {
		log(ctx).Warnf("unable to restore extended metadata of %v: %v", path, err)
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	w.count++

	if w.examples == nil {
		w.examples = map[string]string{}
	}

	if len(w.examples) < maxAttributeWarningExamples {
		if _, ok := w.examples[err.Error()]; !ok {
			w.examples[err.Error()] = path
		}
	}
}

func (w *attributeWarnings) report(ctx context.Context) {
	if w == nil {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.count == 0 {
		return
	}

	log(ctx).Warnf("%v extended attributes, file flags or birth times could not be restored", w.count)

	for _, msg := range slices.Sorted(maps.Keys(w.examples)) {
		log(ctx).Warnf("  %v (for example %v)", msg, w.examples[msg])
	}
}

// restoreAttributeOptions selects the extended metadata that restore applies.
func (o *FilesystemOutput) restoreAttributeOptions() fs.AttributeOptions {
	opts := fs.AttributeOptions{
		BSDFlags:  !o.SkipFileFlags,
		BirthTime: !o.SkipTimes,
		ACLs:      !o.SkipPermissions,
	}

	if !o.SkipXattrs {
		opts.Xattrs = fs.XattrsAll
	}

	return opts
}

// snapshotAttributes returns the extended metadata of e to restore, or nil.
func (o *FilesystemOutput) snapshotAttributes(ctx context.Context, path string, e fs.Entry) *fs.ExtendedAttributes {
	a, err := fs.ReadExtendedAttributes(ctx, e, o.restoreAttributeOptions())
	if err != nil {
		o.warnings.add(ctx, path, err)
		return nil
	}

	return a
}

// restoreXattrs sets the extended attributes in a that are (or, if acls is false, are not) ACLs.
func (o *FilesystemOutput) restoreXattrs(ctx context.Context, path string, a *fs.ExtendedAttributes, acls bool) {
	if a == nil {
		return
	}

	for name, value := range a.Xattrs {
		// POSIX ACLs must be set after chmod, which rewrites the ACL mask.
		if fs.IsPOSIXACLXattr(name) != acls {
			continue
		}

		if isProtectedXattr(name) {
			log(ctx).Debugf("not restoring protected extended attribute %v of %v", name, path)
			continue
		}

		if err := setXattr(path, name, value); err != nil {
			o.warnings.add(ctx, path, err)
		}
	}
}

func (o *FilesystemOutput) restoreACL(ctx context.Context, path string, a *fs.ExtendedAttributes) {
	if a == nil || len(a.ACL) == 0 {
		return
	}

	if err := setACL(path, a.ACL); err != nil {
		o.warnings.add(ctx, path, errors.Wrap(err, "ACL"))
	}
}

func (o *FilesystemOutput) restoreBirthTime(ctx context.Context, path string, a *fs.ExtendedAttributes) {
	if a == nil || a.BirthTime == 0 {
		return
	}

	if err := setBirthTime(path, a.BirthTime); err != nil {
		o.warnings.add(ctx, path, errors.Wrap(err, "birth time"))
	}
}

func (o *FilesystemOutput) restoreBSDFlags(ctx context.Context, path string, a *fs.ExtendedAttributes, isSymlink bool) {
	if a == nil || a.BSDFlags == 0 {
		return
	}

	if err := setBSDFlags(path, a.BSDFlags, isSymlink); err != nil {
		o.warnings.add(ctx, path, errors.Wrap(err, "file flags"))
	}
}
