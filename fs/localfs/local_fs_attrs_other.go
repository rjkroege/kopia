//go:build !darwin && !linux

package localfs

import (
	"github.com/kopia/kopia/fs"
)

func readXattrs(string, fs.AttributeOptions, *fs.ExtendedAttributes) ([]string, error) {
	return nil, nil
}

func flagsAndBirthTime(string) (uint32, fs.UTCTimestamp, error) {
	return 0, 0, nil
}
