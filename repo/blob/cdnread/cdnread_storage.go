//go:build !no_extra_providers

// Package cdnread implements a split read/write Storage: writes go through an
// S3-compatible bucket (typically Backblaze B2), reads are served from an
// unauthenticated CDN fronting that same bucket.
package cdnread

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/pkg/errors"

	"github.com/kopia/kopia/internal/iocopy"
	"github.com/kopia/kopia/repo/blob"
	"github.com/kopia/kopia/repo/blob/s3"
)

const storageType = "cdnread"

// packBlobIDPrefixes are the only blob ID prefixes safe to serve from the
// CDN: kopia's pack blobs are content-addressed and immutable once written
// (see PackBlobIDPrefixRegular/PackBlobIDPrefixSpecial in
// repo/content/content_manager.go). Everything else - kopia.repository,
// kopia.blobcfg, kopia.maintenance, index/manifest/session blobs - can be
// rewritten or deleted over the life of the repository, so those must
// always be read straight from the write-path (Backblaze) storage to avoid
// serving content that is stale relative to the CDN's cache TTL.
var packBlobIDPrefixes = [...]byte{'p', 'q'}

func isImmutablePackBlob(id blob.ID) bool {
	if len(id) == 0 {
		return false
	}

	for _, prefix := range packBlobIDPrefixes {
		if id[0] == prefix {
			return true
		}
	}

	return false
}

// splitPathStorage serves GetBlob from an unauthenticated CDN URL and
// delegates every other Storage method (PutBlob, DeleteBlob, ListBlobs,
// GetMetadata, ...) to the embedded S3-compatible storage.
type splitPathStorage struct {
	blob.Storage

	opt        Options
	cdnBaseURL string
	httpClient *http.Client
}

func (s *splitPathStorage) objectName(id blob.ID) string {
	return s.opt.Prefix + string(id)
}

func (s *splitPathStorage) GetBlob(ctx context.Context, id blob.ID, offset, length int64, output blob.OutputBuffer) error {
	if !isImmutablePackBlob(id) {
		//nolint:wrapcheck
		return s.Storage.GetBlob(ctx, id, offset, length, output)
	}

	output.Reset()

	if offset < 0 {
		return blob.ErrInvalidRange
	}

	url := s.cdnBaseURL + "/" + s.objectName(id)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return errors.Wrap(err, "unable to create request")
	}

	switch {
	case length > 0:
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+length-1))
	case length == 0:
		// zero-length ranges require special handling - request a single byte and
		// discard the body below without copying anything into output.
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset))
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return errors.Wrapf(err, "GET %v", url)
	}
	defer resp.Body.Close() //nolint:errcheck

	switch resp.StatusCode {
	case http.StatusNotFound:
		return blob.ErrBlobNotFound

	case http.StatusRequestedRangeNotSatisfiable:
		return blob.ErrInvalidRange

	case http.StatusOK, http.StatusPartialContent:
		// proceed

	default:
		return errors.Errorf("unexpected CDN response %v for %v", resp.Status, url)
	}

	if length == 0 {
		return nil
	}

	if err := iocopy.JustCopy(output, resp.Body); err != nil {
		return errors.Wrap(err, "unable to copy data")
	}

	//nolint:wrapcheck
	return blob.EnsureLengthExactly(output.Length(), length)
}

func (s *splitPathStorage) DisplayName() string {
	return fmt.Sprintf("CDN read (%v) + %v", s.cdnBaseURL, s.Storage.DisplayName())
}

func (s *splitPathStorage) ConnectionInfo() blob.ConnectionInfo {
	return blob.ConnectionInfo{
		Type:   storageType,
		Config: &s.opt,
	}
}

func (s *splitPathStorage) String() string {
	return fmt.Sprintf("cdnread://%v/%v", s.cdnBaseURL, s.opt.Prefix)
}

// New creates new split read/write storage: the write path is an
// S3-compatible bucket, the read path is an unauthenticated CDN URL fronting
// the same bucket.
func New(ctx context.Context, opt *Options, isCreate bool) (blob.Storage, error) {
	if opt.CDNURL == "" {
		return nil, errors.New("CDN URL must be specified")
	}

	base, err := s3.New(ctx, &opt.Options, isCreate)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create S3-compatible write-path storage")
	}

	return &splitPathStorage{
		Storage:    base,
		opt:        *opt,
		cdnBaseURL: strings.TrimSuffix(opt.CDNURL, "/"),
		httpClient: &http.Client{},
	}, nil
}

func init() {
	blob.AddSupportedStorage(storageType, Options{}, New)
}
