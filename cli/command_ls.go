package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"github.com/kopia/kopia/fs"
	"github.com/kopia/kopia/repo"
	"github.com/kopia/kopia/repo/object"
	"github.com/kopia/kopia/snapshot/snapshotfs"
)

type commandList struct {
	long          bool
	humanReadable bool
	recursive     bool
	showOID       bool
	showAttrs     bool
	errorSummary  bool
	path          string

	out textOutput
}

func (c *commandList) setup(svc appServices, parent commandParent) {
	cmd := parent.Command("list", "List a directory stored in repository object.").Alias("ls")

	cmd.Flag("long", "Long output").Short('l').BoolVar(&c.long)
	cmd.Flag("human-readable", "Show human-readable sizes").Short('h').BoolVar(&c.humanReadable)
	cmd.Flag("recursive", "Recursive output").Short('r').BoolVar(&c.recursive)
	cmd.Flag("show-object-id", "Show object IDs").Short('o').BoolVar(&c.showOID)
	cmd.Flag("extended-attributes", "Show extended attributes, file flags, birth time and ACLs").Short('@').BoolVar(&c.showAttrs)
	cmd.Flag("error-summary", "Emit error summary").Default(trueStr).BoolVar(&c.errorSummary)
	cmd.Arg("object-path", "Path").Required().StringVar(&c.path)
	cmd.Action(svc.repositoryReaderAction(c.run))

	c.out.setup(svc)
}

func (c *commandList) run(ctx context.Context, rep repo.Repository) error {
	dir, err := snapshotfs.FilesystemDirectoryFromIDWithPath(ctx, rep, c.path, false)
	if err != nil {
		return errors.Wrap(err, "unable to get filesystem directory entry")
	}

	var prefix string
	if !c.long {
		prefix = c.path
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
	}

	return c.listDirectory(ctx, dir, prefix, "")
}

func (c *commandList) listDirectory(ctx context.Context, d fs.Directory, prefix, indent string) error {
	iter, err := d.Iterate(ctx)
	if err != nil {
		return err //nolint:wrapcheck
	}
	defer iter.Close()

	e, err := iter.Next(ctx)
	for e != nil {
		if err2 := c.printDirectoryEntry(ctx, e, prefix, indent); err2 != nil {
			return err2
		}

		e, err = iter.Next(ctx)
	}

	if err != nil {
		return err //nolint:wrapcheck
	}

	if dws, ok := d.(fs.DirectoryWithSummary); ok && c.errorSummary {
		if ds, _ := dws.Summary(ctx); ds != nil && ds.FatalErrorCount > 0 {
			errorColor.Fprintf(c.out.stderr(), "\nNOTE: Encountered %v errors while snapshotting this directory:\n\n", ds.FatalErrorCount) //nolint:errcheck

			for _, e := range ds.FailedEntries {
				errorColor.Fprintf(c.out.stderr(), "- Error in \"%v\": %v\n", e.EntryPath, e.Error) //nolint:errcheck
			}
		}
	}

	return nil
}

func (c *commandList) printDirectoryEntry(ctx context.Context, e fs.Entry, prefix, indent string) error {
	hoid, ok := e.(object.HasObjectID)
	if !ok {
		return errors.New("entry without object ID")
	}

	objectID := hoid.ObjectID()
	oid := objectID.String()
	col := defaultColor

	var (
		errorSummary string
		info         string
	)

	if dws, ok := e.(fs.DirectoryWithSummary); ok && c.errorSummary {
		if ds, _ := dws.Summary(ctx); ds != nil && ds.FatalErrorCount > 0 {
			errorSummary = fmt.Sprintf(" (%v errors)", ds.FatalErrorCount)
			col = errorColor
		}
	}

	switch {
	case c.long:
		info = fmt.Sprintf(
			"%v %12s %v %-34v %v%v",
			e.Mode(),
			maybeHumanReadableBytes(c.humanReadable, e.Size()),
			formatTimestamp(e.ModTime().Local()),
			oid,
			c.nameToDisplay(prefix, e),
			errorSummary,
		)
	case c.showOID:
		info = fmt.Sprintf("%-34v %v%v", oid, c.nameToDisplay(prefix, e), errorSummary)

	default:
		info = fmt.Sprintf("%v%v", c.nameToDisplay(prefix, e), errorSummary)
	}

	col.Fprintln(c.out.stdout(), info) //nolint:errcheck

	if c.showAttrs {
		c.printExtendedAttributes(ctx, e, indent)
	}

	if c.recursive {
		if subdir, ok := e.(fs.Directory); ok {
			if listerr := c.listDirectory(ctx, subdir, prefix+e.Name()+"/", indent+"  "); listerr != nil {
				return listerr
			}
		}
	}

	return nil
}

func (c *commandList) nameToDisplay(prefix string, e fs.Entry) string {
	suffix := ""
	if e.IsDir() {
		suffix = "/"
	}

	if c.long || c.recursive {
		return prefix + e.Name() + suffix
	}

	return e.Name()
}

// printExtendedAttributes prints the extended metadata of e, one item per line, like ls -l@.
func (c *commandList) printExtendedAttributes(ctx context.Context, e fs.Entry, indent string) {
	a, err := fs.ReadExtendedAttributes(ctx, e, fs.AttributeOptions{
		Xattrs:       fs.XattrsAll,
		MaxXattrSize: int(^uint(0) >> 1),
		BSDFlags:     true,
		BirthTime:    true,
		ACLs:         true,
	})
	if err != nil {
		errorColor.Fprintf(c.out.stderr(), "unable to read extended attributes of %v: %v\n", e.Name(), err) //nolint:errcheck
		return
	}

	if a == nil {
		return
	}

	prefix := indent + "\t"

	for _, name := range a.XattrNames() {
		c.out.printStdout("%v%v\t%v\n", prefix, name, len(a.Xattrs[name]))
	}

	for _, name := range a.SkippedXattrs {
		c.out.printStdout("%v%v\t(not captured: too large)\n", prefix, name)
	}

	if a.BSDFlags != 0 {
		c.out.printStdout("%vflags: %#x\n", prefix, a.BSDFlags)
	}

	if a.BirthTime != 0 {
		c.out.printStdout("%vbirth time: %v\n", prefix, formatTimestamp(a.BirthTime.ToTime().Local()))
	}

	if len(a.ACL) > 0 {
		c.out.printStdout("%vACL: %v bytes\n", prefix, len(a.ACL))
	}
}
