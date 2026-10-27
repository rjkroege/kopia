package main

import (
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

const ufHidden = unix.UF_HIDDEN

// platformInfo is the non-xattr metadata reported by lstat.
type platformInfo struct {
	flags        uint32 // st_flags, unmasked
	hasBirth     bool
	birthDiffers bool // birth time differs from mtime by more than a second
}

func platformStat(path string) (platformInfo, error) {
	var st unix.Stat_t
	if err := unix.Lstat(path, &st); err != nil {
		return platformInfo{}, errors.Wrap(err, "lstat")
	}

	d := st.Btim.Sec - st.Mtim.Sec

	return platformInfo{
		flags:        st.Flags,
		hasBirth:     st.Btim.Sec != 0,
		birthDiffers: st.Btim.Sec != 0 && (d < -1 || d > 1),
	}, nil
}

var bsdFlagNames = map[uint32]string{ //nolint:gochecknoglobals
	unix.UF_NODUMP: "UF_NODUMP", unix.UF_IMMUTABLE: "UF_IMMUTABLE", unix.UF_APPEND: "UF_APPEND",
	unix.UF_OPAQUE: "UF_OPAQUE", unix.UF_COMPRESSED: "UF_COMPRESSED", unix.UF_TRACKED: "UF_TRACKED",
	unix.UF_DATAVAULT: "UF_DATAVAULT", unix.UF_HIDDEN: "UF_HIDDEN", unix.SF_ARCHIVED: "SF_ARCHIVED",
	unix.SF_IMMUTABLE: "SF_IMMUTABLE", unix.SF_APPEND: "SF_APPEND", unix.SF_RESTRICTED: "SF_RESTRICTED",
	unix.SF_NOUNLINK: "SF_NOUNLINK", unix.SF_FIRMLINK: "SF_FIRMLINK", unix.SF_DATALESS: "SF_DATALESS",
}
