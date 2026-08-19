//go:build !no_extra_providers

package cdnread_test

import (
	"context"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/kopia/kopia/internal/blobtesting"
	"github.com/kopia/kopia/internal/gather"
	"github.com/kopia/kopia/internal/providervalidation"
	"github.com/kopia/kopia/internal/testlogging"
	"github.com/kopia/kopia/internal/testutil"
	"github.com/kopia/kopia/repo/blob"
	"github.com/kopia/kopia/repo/blob/cdnread"
	"github.com/kopia/kopia/repo/blob/s3"
)

// Environment variables follow the KOPIA_CDNREAD_* convention already
// established by the CLI flags in cli/storage_cdnread.go (KOPIA_CDNREAD_
// ACCESS_KEY_ID / KOPIA_CDNREAD_SECRET_ACCESS_KEY), rather than storj's bare
// STORJ_* names: cdnread wraps s3, and s3's own credentials are the same
// ones a user would set to drive the CLI directly, so reusing those exact
// names lets one pair of secrets serve both purposes. TEST_ is added for the
// three additional values (bucket, endpoint, CDN URL) needed only by tests.
const (
	testBucketEnv          = "KOPIA_CDNREAD_TEST_BUCKET"
	testEndpointEnv        = "KOPIA_CDNREAD_TEST_ENDPOINT"
	testCDNURLEnv          = "KOPIA_CDNREAD_TEST_CDN_URL"
	testAccessKeyIDEnv     = "KOPIA_CDNREAD_ACCESS_KEY_ID"
	testSecretAccessKeyEnv = "KOPIA_CDNREAD_SECRET_ACCESS_KEY"
)

func getEnvOrSkip(t *testing.T, name string) string {
	t.Helper()

	value := os.Getenv(name)
	if value == "" {
		t.Skipf("%s not provided", name)
	}

	return value
}

// s3OptionsFromEnv builds the write-path (S3-compatible) half of
// cdnread.Options with a fresh Prefix, so this run is isolated within a
// shared/reused bucket the same way storj's tests isolate via
// Prefix: uuid.NewString().
func s3OptionsFromEnv(t *testing.T) s3.Options {
	t.Helper()

	return s3.Options{
		BucketName:      getEnvOrSkip(t, testBucketEnv),
		Endpoint:        getEnvOrSkip(t, testEndpointEnv),
		AccessKeyID:     getEnvOrSkip(t, testAccessKeyIDEnv),
		SecretAccessKey: getEnvOrSkip(t, testSecretAccessKeyEnv),
		Prefix:          uuid.NewString(),
	}
}

func testOptions(t *testing.T) *cdnread.Options {
	t.Helper()

	return &cdnread.Options{
		Options: s3OptionsFromEnv(t),
		CDNURL:  getEnvOrSkip(t, testCDNURLEnv),
	}
}

func TestCDNReadStorage(t *testing.T) {
	t.Parallel()
	testutil.ProviderTest(t)

	opt := testOptions(t)

	ctx := testlogging.Context(t)

	st, err := cdnread.New(ctx, opt, false)
	require.NoError(t, err)

	defer st.Close(ctx)
	defer blobtesting.CleanupOldData(ctx, t, st, 0)

	blobtesting.VerifyStorage(ctx, t, st, blob.PutOptions{})
	blobtesting.AssertConnectionInfoRoundTrips(ctx, t, st)
	require.NoError(t, providervalidation.ValidateProvider(ctx, st, blobtesting.TestValidationOptions))
}

// TestCDNReadStorage_ReadsGoThroughCDN supplements TestCDNReadStorage:
// blobtesting.VerifyStorage's fixed blob IDs never start with 'p' or 'q', so
// none of them ever exercise cdnread's actual differentiator - routing pack
// blob reads through the CDN instead of the write-path S3 storage. This test
// writes a pack-blob-shaped ID, reads it back through cdnread.GetBlob, and
// independently confirms - via the Cf-Ray response header, which Cloudflare's
// edge stamps on every response it proxies regardless of cache state - that
// the read actually went through Cloudflare rather than merely returning
// correct content (which the embedded S3 storage would also do, masking a
// routing regression).
func TestCDNReadStorage_ReadsGoThroughCDN(t *testing.T) {
	t.Parallel()
	testutil.ProviderTest(t)

	opt := testOptions(t)

	ctx := testlogging.Context(t)

	st, err := cdnread.New(ctx, opt, false)
	require.NoError(t, err)

	defer st.Close(ctx)
	defer blobtesting.CleanupOldData(ctx, t, st, 0)

	blobID := blob.ID("p" + uuid.NewString())
	content := []byte("cdnread conformance " + uuid.NewString())

	require.NoError(t, st.PutBlob(ctx, blobID, gather.FromSlice(content), blob.PutOptions{}))

	var got gather.WriteBuffer
	defer got.Close()

	require.NoError(t, st.GetBlob(ctx, blobID, 0, -1, &got))
	require.Equal(t, content, got.ToByteSlice())

	cdnObjectURL := opt.CDNURL + "/" + opt.Prefix + string(blobID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cdnObjectURL, http.NoBody)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	defer resp.Body.Close() //nolint:errcheck

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotEmpty(t, resp.Header.Get("Cf-Ray"),
		"response is missing the Cf-Ray header Cloudflare stamps on proxied responses - "+
			"the read may not have gone through the CDN")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, content, body)
}

func TestCleanupOldData(t *testing.T) {
	t.Parallel()
	testutil.ProviderTest(t)

	opt := testOptions(t)

	ctx := testlogging.Context(t)

	st, err := cdnread.New(ctx, opt, false)
	require.NoError(t, err)

	defer st.Close(ctx)

	blobtesting.CleanupOldData(ctx, t, st, blobtesting.MinCleanupAge)
}

func TestCDNReadStorageMissingCDNURL(t *testing.T) {
	t.Parallel()
	testutil.ProviderTest(t)

	ctx := context.Background()

	_, err := cdnread.New(ctx, &cdnread.Options{
		Options: s3OptionsFromEnv(t),
		CDNURL:  "",
	}, false)
	require.Error(t, err)
}

func TestCDNReadStorageInvalidBucket(t *testing.T) {
	t.Parallel()
	testutil.ProviderTest(t)

	opt := s3OptionsFromEnv(t)
	opt.BucketName = ""

	ctx := context.Background()

	_, err := cdnread.New(ctx, &cdnread.Options{
		Options: opt,
		CDNURL:  getEnvOrSkip(t, testCDNURLEnv),
	}, false)
	require.Error(t, err)
}

func TestCDNReadStorageInvalidCreds(t *testing.T) {
	t.Parallel()
	testutil.ProviderTest(t)

	opt := s3OptionsFromEnv(t)
	opt.SecretAccessKey = "invalid-secret-access-key"

	ctx := context.Background()

	_, err := cdnread.New(ctx, &cdnread.Options{
		Options: opt,
		CDNURL:  getEnvOrSkip(t, testCDNURLEnv),
	}, false)
	require.Error(t, err)
}
