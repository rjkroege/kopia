//go:build !no_extra_providers

package cdnread

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kopia/kopia/internal/blobtesting"
	"github.com/kopia/kopia/internal/clock"
	"github.com/kopia/kopia/internal/gather"
	"github.com/kopia/kopia/internal/testlogging"
	"github.com/kopia/kopia/repo/blob"
)

const testBlobContent = "hello from the cdn"

func newTestStorage(t *testing.T, handler http.HandlerFunc) *splitPathStorage {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	base := blobtesting.NewMapStorage(blobtesting.DataMap{}, nil, clock.Now)

	return &splitPathStorage{
		Storage:    base,
		opt:        Options{},
		cdnBaseURL: server.URL,
		httpClient: server.Client(),
	}
}

func TestGetBlob_ServesPackBlobsFromCDN(t *testing.T) {
	s := newTestStorage(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/psome-blob", r.URL.Path)
		require.Equal(t, http.MethodGet, r.Method)
		_, _ = w.Write([]byte(testBlobContent))
	})

	var output gather.WriteBuffer
	defer output.Close()

	err := s.GetBlob(testlogging.Context(t), "psome-blob", 0, -1, &output)
	require.NoError(t, err)
	require.Equal(t, testBlobContent, string(output.ToByteSlice()))
}

func TestGetBlob_NotFound(t *testing.T) {
	s := newTestStorage(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	var output gather.WriteBuffer
	defer output.Close()

	err := s.GetBlob(testlogging.Context(t), "pmissing-blob", 0, -1, &output)
	require.ErrorIs(t, err, blob.ErrBlobNotFound)
}

func TestGetBlob_Range(t *testing.T) {
	s := newTestStorage(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "bytes=2-4", r.Header.Get("Range"))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte(testBlobContent[2:5]))
	})

	var output gather.WriteBuffer
	defer output.Close()

	err := s.GetBlob(testlogging.Context(t), "psome-blob", 2, 3, &output)
	require.NoError(t, err)
	require.Equal(t, testBlobContent[2:5], string(output.ToByteSlice()))
}

func TestGetBlob_QPrefixServedFromCDN(t *testing.T) {
	s := newTestStorage(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/qsome-blob", r.URL.Path)
		_, _ = w.Write([]byte(testBlobContent))
	})

	var output gather.WriteBuffer
	defer output.Close()

	err := s.GetBlob(testlogging.Context(t), "qsome-blob", 0, -1, &output)
	require.NoError(t, err)
	require.Equal(t, testBlobContent, string(output.ToByteSlice()))
}

func TestGetBlob_MutableControlBlobsBypassCDN(t *testing.T) {
	s := newTestStorage(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected CDN request for %v - mutable control blobs must always read from the write-path storage", r.URL.Path)
	})

	ctx := testlogging.Context(t)

	var data gather.WriteBuffer
	defer data.Close()

	data.Append([]byte(testBlobContent))

	for _, id := range []blob.ID{"kopia.repository", "kopia.blobcfg", "kopia.maintenance", "nsome-index-blob", "sSomeSession"} {
		require.NoError(t, s.Storage.PutBlob(ctx, id, data.Bytes(), blob.PutOptions{}))

		var output gather.WriteBuffer

		err := s.GetBlob(ctx, id, 0, -1, &output)
		require.NoError(t, err)
		require.Equal(t, testBlobContent, string(output.ToByteSlice()))

		output.Close()
	}
}

func TestPutDeleteListGetMetadata_DelegateToBaseStorage(t *testing.T) {
	s := newTestStorage(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected CDN request for %v - write/list/metadata paths must not use the CDN", r.URL.Path)
	})

	ctx := testlogging.Context(t)

	var data gather.WriteBuffer
	defer data.Close()

	data.Append([]byte(testBlobContent))

	require.NoError(t, s.PutBlob(ctx, "put-blob", data.Bytes(), blob.PutOptions{}))

	md, err := s.GetMetadata(ctx, "put-blob")
	require.NoError(t, err)
	require.Equal(t, blob.ID("put-blob"), md.BlobID)

	var found []blob.ID

	require.NoError(t, s.ListBlobs(ctx, "", func(bm blob.Metadata) error {
		found = append(found, bm.BlobID)
		return nil
	}))
	require.Contains(t, found, blob.ID("put-blob"))

	require.NoError(t, s.DeleteBlob(ctx, "put-blob"))

	_, err = s.GetMetadata(ctx, "put-blob")
	require.ErrorIs(t, err, blob.ErrBlobNotFound)
}

func TestGetBlob_RetriesThrottlingAndServerErrors(t *testing.T) {
	var calls int

	s := newTestStorage(t, func(w http.ResponseWriter, r *http.Request) {
		calls++

		switch calls {
		case 1:
			w.WriteHeader(http.StatusTooManyRequests)
		case 2:
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			_, _ = w.Write([]byte(testBlobContent))
		}
	})

	var output gather.WriteBuffer
	defer output.Close()

	err := s.GetBlob(testlogging.Context(t), "psome-blob", 0, -1, &output)
	require.NoError(t, err)
	require.Equal(t, testBlobContent, string(output.ToByteSlice()))
	require.Equal(t, 3, calls)
}

func TestGetBlob_DoesNotRetryNotFound(t *testing.T) {
	var calls int

	s := newTestStorage(t, func(w http.ResponseWriter, r *http.Request) {
		calls++

		w.WriteHeader(http.StatusNotFound)
	})

	var output gather.WriteBuffer
	defer output.Close()

	err := s.GetBlob(testlogging.Context(t), "pmissing-blob", 0, -1, &output)
	require.ErrorIs(t, err, blob.ErrBlobNotFound)
	require.Equal(t, 1, calls)
}
