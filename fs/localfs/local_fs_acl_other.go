//go:build !darwin

package localfs

// readACL returns nil: POSIX ACLs are captured as system.posix_acl_* xattrs.
func readACL(string) ([]byte, error) {
	return nil, nil
}
