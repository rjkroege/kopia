package cachefs

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kopia/kopia/fs"
	"github.com/kopia/kopia/internal/mockfs"
)

func TestWrap_ForwardsExtendedAttributes(t *testing.T) {
	ctx := t.Context()
	attrs := &fs.ExtendedAttributes{BSDFlags: 2}
	opts := fs.AttributeOptions{BSDFlags: true}

	root := mockfs.NewDirectory()
	root.SetExtendedAttributes(attrs)
	root.AddFile("f", nil, 0o644).SetExtendedAttributes(attrs)
	root.AddSymlink("l", "f", 0o777).SetExtendedAttributes(attrs)

	wrapped, ok := Wrap(root, NewCache(&Options{})).(fs.Directory)
	require.True(t, ok)

	got, err := fs.ReadExtendedAttributes(ctx, wrapped, opts)
	require.NoError(t, err)
	require.True(t, attrs.Equal(got))

	for _, name := range []string{"f", "l"} {
		e, err := wrapped.Child(ctx, name)
		require.NoError(t, err)

		got, err := fs.ReadExtendedAttributes(ctx, e, opts)
		require.NoError(t, err)
		require.True(t, attrs.Equal(got), name)
	}
}
