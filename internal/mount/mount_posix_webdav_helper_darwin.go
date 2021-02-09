package mount

import (
	"os/exec"

	"github.com/pkg/errors"
)

func mountWebDavHelper(url, path string) error {
	mount := exec.Command("/sbin/mount", "-t", "webdav", "-r", url, path) //nolint:gosec
	if err := mount.Run(); err != nil {
		return errors.Errorf("webdav mount %q on %q failed: %v", url, path, err)
	}

	return nil
}

func unmountWebDevHelper(path string) error {
	unmount := exec.Command("/usr/sbin/diskutil", "unmount", path) //nolint:gosec
	if err := unmount.Run(); err != nil {
		return errors.Errorf("unmount %q failed: %v", path, err)
	}

	return nil
}
