package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	emitJSON      bool
	path          string
	notfirst      bool

	out textOutput
	enc *json.Encoder
}

func (c *commandList) setup(svc appServices, parent commandParent) {
	cmd := parent.Command("list", "List a directory stored in repository object.").Alias("ls")

	cmd.Flag("long", "Long output").Short('l').BoolVar(&c.long)
	cmd.Flag("human-readable", "Show human-readable sizes").Short('h').BoolVar(&c.humanReadable)
	cmd.Flag("recursive", "Recursive output").Short('r').BoolVar(&c.recursive)
	cmd.Flag("show-object-id", "Show object IDs").Short('o').BoolVar(&c.showOID)
	cmd.Flag("extended-attributes", "Show extended attributes, file flags, birth time and ACLs").Short('@').BoolVar(&c.showAttrs)
	cmd.Flag("error-summary", "Emit error summary").Default(trueStr).BoolVar(&c.errorSummary)
	cmd.Flag("emit-json", "Dump JSON format").Short('j').Default("false").BoolVar(&c.emitJSON)
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

	if c.emitJSON {
		c.enc = json.NewEncoder(c.out.stdout())

		if _, err := io.WriteString(c.out.stdout(), "[\n"); err != nil {
			return errors.Wrap(err, "unable to write JSON output")
		}
	}

	if err := c.listDirectory(ctx, dir, prefix, ""); err != nil {
		return err
	}

	if c.emitJSON {
		if _, err := io.WriteString(c.out.stdout(), "]\n"); err != nil {
			return errors.Wrap(err, "unable to write JSON output")
		}
	}

	return nil
}

func (c *commandList) listDirectory(ctx context.Context, d fs.Directory, prefix, indent string) error {
	iter, err := d.Iterate(ctx)
	if err != nil {
		return err //nolint:wrapcheck
	}
	defer iter.Close()

	e, err := iter.Next(ctx)
	for e != nil {
		if c.emitJSON {
			if err2 := c.printDirectoryJSONEntry(ctx, e, prefix, indent); err2 != nil {
				return err2
			}
		} else {
			if err2 := c.printDirectoryEntry(ctx, e, prefix, indent); err2 != nil {
				return err2
			}
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

// jsonWithLong is the --emit-json entry format with --long.
type jsonWithLong struct {
	Mode               string
	Size               int64
	Time               string
	Oid                string
	DisplayName        string
	ErrorSummary       string
	ExtendedAttributes *jsonExtendedAttributes `json:",omitempty"`
}

// jsonWithOID is the --emit-json entry format with --show-object-id.
type jsonWithOID struct {
	Oid                string
	DisplayName        string
	ErrorSummary       string
	ExtendedAttributes *jsonExtendedAttributes `json:",omitempty"`
}

// jsonDefault is the default --emit-json entry format.
type jsonDefault struct {
	DisplayName        string
	ErrorSummary       string
	ExtendedAttributes *jsonExtendedAttributes `json:",omitempty"`
}

// jsonExtendedAttributes is the --emit-json form of --extended-attributes: the same items
// as the text output, with attribute value sizes rather than values.
type jsonExtendedAttributes struct {
	Xattrs    map[string]int `json:",omitempty"` // name -> value size in bytes
	Flags     uint32         `json:",omitempty"`
	BirthTime string         `json:",omitempty"`
	ACLSize   int            `json:",omitempty"`
}

// newJSONExtendedAttributes returns the --emit-json form of a, or nil if a is empty.
func newJSONExtendedAttributes(a *fs.ExtendedAttributes) *jsonExtendedAttributes {
	if a.IsEmpty() {
		return nil
	}

	j := &jsonExtendedAttributes{
		Flags:   a.BSDFlags,
		ACLSize: len(a.ACL),
	}

	if len(a.Xattrs) > 0 {
		j.Xattrs = make(map[string]int, len(a.Xattrs))
		for name, value := range a.Xattrs {
			j.Xattrs[name] = len(value)
		}
	}

	if a.BirthTime != 0 {
		j.BirthTime = formatTimestamp(a.BirthTime.ToTime().Local())
	}

	return j
}

func (c *commandList) printDirectoryJSONEntry(ctx context.Context, e fs.Entry, prefix, indent string) error {
	hoid, ok := e.(object.HasObjectID)
	if !ok {
		return errors.Errorf("entry without object ID")
	}

	objectID := hoid.ObjectID()
	oid := objectID.String()

	errorSummary := ""

	if dws, ok := e.(fs.DirectoryWithSummary); ok && c.errorSummary {
		if ds, _ := dws.Summary(ctx); ds != nil && ds.FatalErrorCount > 0 {
			errorSummary = fmt.Sprintf(" (%v errors)", ds.FatalErrorCount)
		}
	}

	if c.notfirst {
		if _, err := io.WriteString(c.out.stdout(), ",\n"); err != nil {
			return errors.Wrap(err, "unable to write JSON output")
		}
	}

	c.notfirst = true

	var attrs *jsonExtendedAttributes
	if c.showAttrs {
		attrs = newJSONExtendedAttributes(c.readExtendedAttributes(ctx, e))
	}

	var obj any

	switch {
	case c.long:
		obj = &jsonWithLong{
			Mode:               e.Mode().String(),
			Size:               e.Size(),
			Time:               formatTimestamp(e.ModTime().Local()),
			Oid:                oid,
			DisplayName:        c.nameToDisplay(prefix, e),
			ErrorSummary:       errorSummary,
			ExtendedAttributes: attrs,
		}
	case c.showOID:
		obj = &jsonWithOID{
			Oid:                oid,
			DisplayName:        c.nameToDisplay(prefix, e),
			ErrorSummary:       errorSummary,
			ExtendedAttributes: attrs,
		}
	default:
		obj = &jsonDefault{
			DisplayName:        c.nameToDisplay(prefix, e),
			ErrorSummary:       errorSummary,
			ExtendedAttributes: attrs,
		}
	}

	if err := c.enc.Encode(obj); err != nil {
		return errors.Wrap(err, "unable to encode JSON entry")
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

// readExtendedAttributes returns all extended metadata of e, or nil if it has none or it
// cannot be read. Read errors are reported on stderr.
func (c *commandList) readExtendedAttributes(ctx context.Context, e fs.Entry) *fs.ExtendedAttributes {
	a, err := fs.ReadExtendedAttributes(ctx, e, fs.AttributeOptions{
		Xattrs:    fs.XattrsAll,
		BSDFlags:  true,
		BirthTime: true,
		ACLs:      true,
	})
	if err != nil {
		errorColor.Fprintf(c.out.stderr(), "unable to read extended attributes of %v: %v\n", e.Name(), err) //nolint:errcheck
		return nil
	}

	return a
}

// printExtendedAttributes prints the extended metadata of e, one item per line, like ls -l@.
func (c *commandList) printExtendedAttributes(ctx context.Context, e fs.Entry, indent string) {
	a := c.readExtendedAttributes(ctx, e)
	if a == nil {
		return
	}

	prefix := indent + "\t"

	for _, name := range a.XattrNames() {
		c.out.printStdout("%v%v\t%v\n", prefix, name, len(a.Xattrs[name]))
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
