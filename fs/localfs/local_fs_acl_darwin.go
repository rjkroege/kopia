package localfs

import (
	"encoding/binary"
	"unsafe"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

const (
	// maxFilesecSize bounds the kauth_filesec buffer: a 44-byte header plus up to
	// KAUTH_ACL_MAX_ENTRIES (128) entries of 24 bytes.
	maxFilesecSize = 4096

	// attrHeaderSize is the u_int32_t length followed by one attrreference_t.
	attrHeaderSize = 12
	attrLengthSize = 4

	filesecEntryCountOffset = 36
	filesecNoACL            = 0xffffffff
)

// readACL returns the kauth_filesec of path (not following symlinks), or nil if it has no ACL.
func readACL(path string) ([]byte, error) {
	p, err := unix.BytePtrFromString(path)
	if err != nil {
		return nil, errors.Wrap(err, "invalid path")
	}

	al := unix.Attrlist{Bitmapcount: unix.ATTR_BIT_MAP_COUNT, Commonattr: unix.ATTR_CMN_EXTENDED_SECURITY}
	buf := make([]byte, attrHeaderSize+maxFilesecSize)

	// x/sys/unix has no getattrlist wrapper, and the ACL cannot be read with getxattr
	// (com.apple.system.Security is not readable) or acl_get_link_np without cgo. Syscall6
	// goes through libSystem's syscall(), so this works, but syscall numbers are not a
	// stable macOS ABI; switch to a wrapper if x/sys gains one.
	//nolint:gosec,staticcheck
	if _, _, e := unix.Syscall6(unix.SYS_GETATTRLIST,
		uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&al)),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unix.FSOPT_NOFOLLOW), 0); e != 0 {
		if isNotSupportedError(e) {
			return nil, nil
		}

		return nil, errors.Wrap(e, "getattrlist")
	}

	// the attrreference offset is relative to the attrreference itself, which follows the length
	offset := int(int32(binary.NativeEndian.Uint32(buf[4:8]))) + attrLengthSize //nolint:gosec
	length := int(binary.NativeEndian.Uint32(buf[8:12]))

	if length < filesecEntryCountOffset+4 {
		return nil, nil
	}

	if offset < attrHeaderSize || offset+length > len(buf) {
		return nil, errors.Errorf("unexpected getattrlist result: offset %v length %v", offset, length)
	}

	filesec := buf[offset : offset+length]

	if n := binary.NativeEndian.Uint32(filesec[filesecEntryCountOffset:]); n == 0 || n == filesecNoACL {
		return nil, nil
	}

	return append([]byte(nil), filesec...), nil
}
