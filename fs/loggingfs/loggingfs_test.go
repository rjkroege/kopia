package loggingfs_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kopia/kopia/fs"
	"github.com/kopia/kopia/fs/loggingfs"
	"github.com/kopia/kopia/internal/mockfs"
)

func TestWrap_ForwardsExtendedAttributes(t *testing.T) {
	ctx := t.Context()
	attrs := fs.ExtendedAttributes{Xattrs: map[string][]byte{"user.a": []byte("b")}}
	opts := fs.AttributeOptions{Xattrs: fs.XattrsAll}

	root := mockfs.NewDirectory()
	root.SetExtendedAttributes(attrs)
	root.AddFile("f", nil, 0o644).SetExtendedAttributes(attrs)
	root.AddSymlink("l", "f", 0o777).SetExtendedAttributes(attrs)

	nop := func(string, ...any) {}
	wrapped, ok := loggingfs.Wrap(root, nop).(fs.Directory)
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
