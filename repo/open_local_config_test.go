package repo_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kopia/kopia/internal/testlogging"
	"github.com/kopia/kopia/internal/testutil"
	"github.com/kopia/kopia/repo"
	"github.com/kopia/kopia/repo/blob/filesystem"
	"github.com/kopia/kopia/repo/blob/throttling"
	"github.com/kopia/kopia/repo/content"
	"github.com/kopia/kopia/repo/object"
)

const inMemoryTestPassword = "in-memory-password"

// newLocalConfigForTest initializes a filesystem repository and returns the
// LocalConfig that would otherwise have been persisted to a config file.
func newLocalConfigForTest(t *testing.T) (*repo.LocalConfig, string) {
	t.Helper()

	ctx := testlogging.Context(t)
	repoDir := testutil.TempDirectory(t)

	st, err := filesystem.New(ctx, &filesystem.Options{Path: repoDir}, false)
	require.NoError(t, err)

	require.NoError(t, repo.Initialize(ctx, st, &repo.NewRepositoryOptions{}, inMemoryTestPassword))

	ci := st.ConnectionInfo()
	require.NoError(t, st.Close(ctx))

	return &repo.LocalConfig{
		Storage: &ci,
		Caching: &content.CachingOptions{
			CacheDirectory:             testutil.TempDirectory(t),
			ContentCacheSizeBytes:      1 << 20,
			ContentCacheSizeLimitBytes: 1 << 20,
		},
	}, repoDir
}

func TestOpenWithLocalConfigRoundTripWithoutConfigFile(t *testing.T) {
	ctx := testlogging.Context(t)
	lc, _ := newLocalConfigForTest(t)

	// run from an empty directory so any stray file (e.g. ".mlock") is visible.
	cwd := testutil.TempDirectory(t)
	old, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(cwd))

	t.Cleanup(func() { require.NoError(t, os.Chdir(old)) })

	r, err := repo.OpenWithLocalConfig(ctx, lc, inMemoryTestPassword, nil)
	require.NoError(t, err)

	dr, ok := r.(repo.DirectRepository)
	require.True(t, ok)
	require.Empty(t, dr.ConfigFilename(), "no config file exists for an in-memory config")

	var oid object.ID

	require.NoError(t, repo.WriteSession(ctx, r, repo.WriteSessionOptions{Purpose: "test"}, func(ctx context.Context, w repo.RepositoryWriter) error {
		ow := w.NewObjectWriter(ctx, object.WriterOptions{})
		_, werr := ow.Write([]byte("hello from memory"))
		require.NoError(t, werr)

		var cerr error

		oid, cerr = ow.Result()

		return cerr
	}))

	// updating throttling limits must not try to persist to a config file.
	limits := throttling.Limits{}
	require.NoError(t, dr.Throttler().SetLimits(limits))

	require.NoError(t, r.Close(ctx))

	// reopen from the same in-memory config and read the object back.
	r2, err := repo.OpenWithLocalConfig(ctx, lc, inMemoryTestPassword, nil)
	require.NoError(t, err)

	rd, err := r2.OpenObject(ctx, oid)
	require.NoError(t, err)

	buf := make([]byte, 64)
	n, _ := rd.Read(buf)
	require.Equal(t, "hello from memory", string(buf[:n]))
	require.NoError(t, rd.Close())
	require.NoError(t, r2.Close(ctx))

	entries, err := os.ReadDir(cwd)
	require.NoError(t, err)
	require.Empty(t, entries, "opening from memory must not create files in the working directory")
}

func TestOpenWithLocalConfigRejectsBadInput(t *testing.T) {
	ctx := testlogging.Context(t)
	lc, _ := newLocalConfigForTest(t)

	_, err := repo.OpenWithLocalConfig(ctx, lc, "wrong-password", nil)
	require.ErrorIs(t, err, repo.ErrInvalidPassword)

	_, err = repo.OpenWithLocalConfig(ctx, nil, inMemoryTestPassword, nil)
	require.Error(t, err)

	_, err = repo.OpenWithLocalConfig(ctx, &repo.LocalConfig{}, inMemoryTestPassword, nil)
	require.Error(t, err, "storage is required")

	relative := *lc
	relative.Caching = &content.CachingOptions{CacheDirectory: filepath.Join("relative", "cache")}
	_, err = repo.OpenWithLocalConfig(ctx, &relative, inMemoryTestPassword, nil)
	require.ErrorContains(t, err, "must be absolute")
}
