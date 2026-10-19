package snapshotfs_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kopia/kopia/fs"
	"github.com/kopia/kopia/snapshot"
	"github.com/kopia/kopia/snapshot/snapshotfs"
)

func TestEntryFromDirEntry_ExposesExtendedAttributes(t *testing.T) {
	ctx := t.Context()
	attrs := &fs.ExtendedAttributes{
		Xattrs:    map[string][]byte{"user.a": []byte("b"), "com.apple.quarantine": []byte("q")},
		BirthTime: 42,
	}

	for _, typ := range []snapshot.EntryType{snapshot.EntryTypeFile, snapshot.EntryTypeDirectory, snapshot.EntryTypeSymlink} {
		t.Run(string(typ), func(t *testing.T) {
			e := snapshotfs.EntryFromDirEntry(nil, &snapshot.DirEntry{Name: "x", Type: typ, Attributes: attrs})

			got, err := fs.ReadExtendedAttributes(ctx, e, fs.AttributeOptions{
				Xattrs:        fs.XattrsAll,
				ExcludeXattrs: []string{"com.apple.quarantine"},
				BirthTime:     true,
			})
			require.NoError(t, err)
			require.True(t, (&fs.ExtendedAttributes{Xattrs: map[string][]byte{"user.a": []byte("b")}, BirthTime: 42}).Equal(got))
		})
	}
}

func TestEntryFromDirEntry_NoAttributes(t *testing.T) {
	e := snapshotfs.EntryFromDirEntry(nil, &snapshot.DirEntry{Name: "x", Type: snapshot.EntryTypeFile})

	got, err := fs.ReadExtendedAttributes(t.Context(), e, fs.AttributeOptions{Xattrs: fs.XattrsAll})
	require.NoError(t, err)
	require.Nil(t, got)
}
