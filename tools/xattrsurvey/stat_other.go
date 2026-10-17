//go:build !darwin

package main

const ufHidden = 0

// platformInfo is empty on platforms without BSD flags and birth times in stat.
type platformInfo struct {
	flags        uint32
	hasBirth     bool
	birthDiffers bool
}

func platformStat(string) (platformInfo, error) {
	return platformInfo{}, nil
}

var bsdFlagNames = map[uint32]string{} //nolint:gochecknoglobals
