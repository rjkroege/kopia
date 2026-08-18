//go:build !no_extra_providers

package cdnread

import "github.com/kopia/kopia/repo/blob/s3"

// Options defines options for split-path storage: writes go through an
// S3-compatible endpoint (typically Backblaze B2's S3 API), while reads are
// served from an unauthenticated CDN URL fronting the same bucket.
type Options struct {
	s3.Options

	// CDNURL is the base URL of the CDN serving the read-only path,
	// e.g. "https://assets.example.com". No trailing slash.
	CDNURL string `json:"cdnURL"`
}
