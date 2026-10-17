package restore

import (
	"context"
	"fmt"
	"strings"

	"github.com/kopia/kopia/fs"
)

// PAX record keys understood by GNU tar and libarchive (bsdtar).
const (
	paxXattrPrefix   = "SCHILY.xattr."
	paxFileFlags     = "SCHILY.fflags"
	paxCreationTime  = "LIBARCHIVE.creationtime"
	nanosecondDigits = 9
)

// bsdFlagNames maps BSD file flags to the names used by strtofflags(3), chflags(1) and
// libarchive. Values are the macOS/FreeBSD bit values.
var bsdFlagNames = []struct { //nolint:gochecknoglobals
	bit  uint32
	name string
}{
	{0x00000001, "nodump"},
	{0x00000002, "uchg"},
	{0x00000004, "uappnd"},
	{0x00000008, "opaque"},
	{0x00008000, "hidden"},
	{0x00010000, "arch"},
	{0x00020000, "schg"},
	{0x00040000, "sappnd"},
	{0x00100000, "sunlnk"},
}

func bsdFlagsText(flags uint32) string {
	var names []string

	for _, f := range bsdFlagNames {
		if flags&f.bit != 0 {
			names = append(names, f.name)
		}
	}

	return strings.Join(names, ",")
}

// tarPAXRecords returns PAX records describing the extended metadata of e, or nil.
func tarPAXRecords(ctx context.Context, e fs.Entry) map[string]string {
	a, err := fs.ReadExtendedAttributes(ctx, e, fs.AttributeOptions{
		Xattrs:       fs.XattrsAll,
		MaxXattrSize: int(^uint(0) >> 1),
		BSDFlags:     true,
		BirthTime:    true,
	})
	if err != nil {
		log(ctx).Warnf("unable to read extended metadata of %v: %v", e.Name(), err)
		return nil
	}

	if a == nil {
		return nil
	}

	recs := map[string]string{}

	for name, v := range a.Xattrs {
		recs[paxXattrPrefix+name] = string(v)
	}

	if s := bsdFlagsText(a.BSDFlags); s != "" {
		recs[paxFileFlags] = s
	}

	if a.BirthTime > 0 {
		ns := int64(a.BirthTime)
		recs[paxCreationTime] = fmt.Sprintf("%d.%0*d", ns/1e9, nanosecondDigits, ns%1e9)
	}

	if len(recs) == 0 {
		return nil
	}

	return recs
}
