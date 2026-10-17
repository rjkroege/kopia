package localfs

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kopia/kopia/fs"
	"github.com/kopia/kopia/snapshot"
)

func TestShallowEntries_ExtendedAttributesFromPlaceholder(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	attrs := &fs.ExtendedAttributes{Xattrs: map[string][]byte{"user.a": []byte("1")}, BirthTime: 42}

	_, err := WriteShallowPlaceholder(filepath.Join(root, "f"), &snapshot.DirEntry{Name: "f", Type: snapshot.EntryTypeFile, Attributes: attrs})
	require.NoError(t, err)

	_, err = WriteShallowPlaceholder(filepath.Join(root, "d"), &snapshot.DirEntry{Name: "d", Type: snapshot.EntryTypeDirectory, Attributes: attrs})
	require.NoError(t, err)

	_, err = WriteShallowPlaceholder(filepath.Join(root, "plain"), &snapshot.DirEntry{Name: "plain", Type: snapshot.EntryTypeFile})
	require.NoError(t, err)

	dir, err := Directory(root)
	require.NoError(t, err)

	opts := fs.AttributeOptions{Xattrs: fs.XattrsAll, BirthTime: true}
	seen := map[string]*fs.ExtendedAttributes{}

	require.NoError(t, fs.IterateEntries(ctx, dir, func(ctx context.Context, e fs.Entry) error {
		_, isShallow := e.(snapshot.HasDirEntryOrNil)
		require.True(t, isShallow, e.Name())

		a, err := fs.ReadExtendedAttributes(ctx, e, opts)
		require.NoError(t, err)

		seen[e.Name()] = a

		return nil
	}))

	require.True(t, attrs.Equal(seen["f"]))
	require.True(t, attrs.Equal(seen["d"]))
	require.Nil(t, seen["plain"])
}
