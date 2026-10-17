package restore

import (
	"encoding/binary"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"

	"github.com/kopia/kopia/fs"
)

// restorableBSDFlags must match the flags captured by localfs.
const restorableBSDFlags = unix.UF_NODUMP | unix.UF_IMMUTABLE | unix.UF_APPEND | unix.UF_OPAQUE |
	unix.UF_HIDDEN | unix.SF_ARCHIVED | unix.SF_IMMUTABLE | unix.SF_APPEND | unix.SF_NOUNLINK

// protectedXattrs cannot be written by restore: they are maintained by the system and
// protected by System Integrity Protection.
var protectedXattrs = map[string]bool{ //nolint:gochecknoglobals
	"com.apple.macl":       true,
	"com.apple.provenance": true,
	"com.apple.rootless":   true,
}

func isProtectedXattr(name string) bool {
	return protectedXattrs[name]
}

// setXattr sets an extended attribute of path itself, not of a symlink's target.
func setXattr(path, name string, value []byte) error {
	return errors.Wrapf(unix.Lsetxattr(path, name, value, 0), "lsetxattr %v", name)
}

// setBirthTime sets the birth time of path itself, not of a symlink's target.
func setBirthTime(path string, btime fs.UTCTimestamp) error {
	// The ATTR_CMN_CRTIME attribute buffer is a struct timespec.
	buf, err := binary.Append(nil, binary.NativeEndian, unix.NsecToTimespec(int64(btime)))
	if err != nil {
		return errors.Wrap(err, "encode timespec")
	}

	return errors.Wrap(unix.Setattrlist(path, &unix.Attrlist{
		Bitmapcount: unix.ATTR_BIT_MAP_COUNT,
		Commonattr:  unix.ATTR_CMN_CRTIME,
	}, buf, unix.FSOPT_NOFOLLOW), "setattrlist")
}

// setACL sets the kauth_filesec of path, not following symlinks.
func setACL(path string, filesec []byte) error {
	const attrReferenceSize = 8

	// attrreference_t {offset, length} followed by the data; offset is relative to the reference
	buf := make([]byte, attrReferenceSize+len(filesec))
	binary.NativeEndian.PutUint32(buf[0:4], attrReferenceSize)
	binary.NativeEndian.PutUint32(buf[4:8], uint32(len(filesec))) //nolint:gosec
	copy(buf[attrReferenceSize:], filesec)

	return errors.Wrap(unix.Setattrlist(path, &unix.Attrlist{
		Bitmapcount: unix.ATTR_BIT_MAP_COUNT,
		Commonattr:  unix.ATTR_CMN_EXTENDED_SECURITY,
	}, buf, unix.FSOPT_NOFOLLOW), "setattrlist")
}

// setBSDFlags sets the restorable flags of path to flags, keeping other flags as they are.
func setBSDFlags(path string, flags uint32, isSymlink bool) error {
	var st unix.Stat_t
	if err := unix.Lstat(path, &st); err != nil {
		return errors.Wrap(err, "lstat")
	}

	want := (st.Flags &^ restorableBSDFlags) | (flags & restorableBSDFlags)
	if want == st.Flags {
		return nil
	}

	if !isSymlink {
		return errors.Wrap(unix.Chflags(path, int(want)), "chflags")
	}

	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_SYMLINK|unix.O_CLOEXEC, 0)
	if err != nil {
		return errors.Wrap(err, "open symlink")
	}

	defer unix.Close(fd) //nolint:errcheck

	return errors.Wrap(unix.Fchflags(fd, int(want)), "fchflags")
}
