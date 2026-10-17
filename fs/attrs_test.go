package fs_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kopia/kopia/fs"
	"github.com/kopia/kopia/internal/mockfs"
)

func sampleAttrs() *fs.ExtendedAttributes {
	return &fs.ExtendedAttributes{
		Xattrs: map[string][]byte{
			"com.apple.FinderInfo": make([]byte, 32),
			"user.small":           []byte("v"),
			"com.apple.quarantine": []byte("0083;..."),
		},
		BSDFlags:  0x8000,
		BirthTime: 1_700_000_000_000_000_000,
	}
}

func TestExtendedAttributes_IsEmpty(t *testing.T) {
	var nilAttrs *fs.ExtendedAttributes

	require.True(t, nilAttrs.IsEmpty())
	require.True(t, (&fs.ExtendedAttributes{Xattrs: map[string][]byte{}}).IsEmpty())
	require.False(t, (&fs.ExtendedAttributes{BSDFlags: 1}).IsEmpty())
	require.False(t, (&fs.ExtendedAttributes{BirthTime: 1}).IsEmpty())
	require.False(t, (&fs.ExtendedAttributes{Xattrs: map[string][]byte{"a": nil}}).IsEmpty())
}

func TestExtendedAttributes_CloneIsDeep(t *testing.T) {
	a := sampleAttrs()
	c := a.Clone()

	require.True(t, a.Equal(c))

	c.Xattrs["user.small"][0] = 'x'

	require.Equal(t, []byte("v"), a.Xattrs["user.small"])
	require.False(t, a.Equal(c))

	var nilAttrs *fs.ExtendedAttributes

	require.Nil(t, nilAttrs.Clone())
}

func TestExtendedAttributes_Equal(t *testing.T) {
	var nilAttrs *fs.ExtendedAttributes

	require.True(t, nilAttrs.Equal(&fs.ExtendedAttributes{}))
	require.False(t, nilAttrs.Equal(sampleAttrs()))
	require.False(t, sampleAttrs().Equal(nilAttrs))

	b := sampleAttrs()
	b.BSDFlags = 0

	require.False(t, sampleAttrs().Equal(b))
}

func TestExtendedAttributes_JSONRoundTrip(t *testing.T) {
	a := sampleAttrs()

	b, err := json.Marshal(a)
	require.NoError(t, err)

	var got fs.ExtendedAttributes

	require.NoError(t, json.Unmarshal(b, &got))
	require.True(t, a.Equal(&got))

	empty, err := json.Marshal(&fs.ExtendedAttributes{})
	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(empty))
}

func TestExtendedAttributes_JSONIsDeterministic(t *testing.T) {
	first, err := json.Marshal(sampleAttrs())
	require.NoError(t, err)

	for range 20 {
		b, err := json.Marshal(sampleAttrs())
		require.NoError(t, err)
		require.Equal(t, first, b)
	}
}

func TestXattrMode_Text(t *testing.T) {
	for _, m := range []fs.XattrMode{fs.XattrsNone, fs.XattrsUser, fs.XattrsAll} {
		b, err := json.Marshal(m)
		require.NoError(t, err)

		var got fs.XattrMode

		require.NoError(t, json.Unmarshal(b, &got))
		require.Equal(t, m, got)
	}

	b, err := json.Marshal(fs.XattrsUser)
	require.NoError(t, err)
	require.JSONEq(t, `"user"`, string(b))

	var m fs.XattrMode

	require.Error(t, m.UnmarshalText([]byte("bogus")))
}

func TestAttributeOptions_IsXattrExcluded(t *testing.T) {
	opts := fs.AttributeOptions{ExcludeXattrs: []string{"com.apple.quarantine", "com.apple.macl"}}

	cases := map[string]bool{
		"com.apple.quarantine":  true,
		"com.apple.quarantine2": false,
		"com.apple.macl":        true,
		"user.x":                false,
	}

	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, want, opts.IsXattrExcluded(name))
		})
	}
}

func TestExtendedAttributes_Filtered(t *testing.T) {
	a := sampleAttrs()

	cases := []struct {
		name string
		opts fs.AttributeOptions
		want *fs.ExtendedAttributes
	}{
		{"zero options", fs.AttributeOptions{}, nil},
		{"flags only", fs.AttributeOptions{BSDFlags: true}, &fs.ExtendedAttributes{BSDFlags: 0x8000}},
		{"birth time only", fs.AttributeOptions{BirthTime: true}, &fs.ExtendedAttributes{BirthTime: a.BirthTime}},
		{
			"xattrs with exclusion",
			fs.AttributeOptions{Xattrs: fs.XattrsAll, ExcludeXattrs: []string{"com.apple.quarantine"}},
			&fs.ExtendedAttributes{
				Xattrs: map[string][]byte{"com.apple.FinderInfo": make([]byte, 32), "user.small": []byte("v")},
			},
		},
		{"excluding everything", fs.AttributeOptions{Xattrs: fs.XattrsAll, ExcludeXattrs: []string{"com.apple.FinderInfo", "user.small", "com.apple.quarantine"}}, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := a.Filtered(tc.opts)
			if tc.want == nil {
				require.Nil(t, got)
				return
			}

			require.True(t, tc.want.Equal(got), "got %+v", got)
		})
	}

	require.True(t, sampleAttrs().Equal(a), "Filtered must not modify the receiver")
}

func TestExtendedAttributes_FilteredReturnsReceiverWhenNothingIsDropped(t *testing.T) {
	a := sampleAttrs()

	require.Same(t, a, a.Filtered(fs.AttributeOptions{Xattrs: fs.XattrsAll, BSDFlags: true, BirthTime: true}))
	require.Same(t, a, a.Filtered(fs.AttributeOptions{Xattrs: fs.XattrsUser, ExcludeXattrs: []string{"other"}, BSDFlags: true, BirthTime: true}))
	require.NotSame(t, a, a.Filtered(fs.AttributeOptions{Xattrs: fs.XattrsAll, BSDFlags: true}))
}

func TestReadExtendedAttributes(t *testing.T) {
	ctx := t.Context()
	dir := mockfs.NewDirectory()
	f := dir.AddFile("f", []byte("x"), 0o644)
	f.SetExtendedAttributes(*sampleAttrs())

	plain := dir.AddFile("plain", nil, 0o644)

	got, err := fs.ReadExtendedAttributes(ctx, f, fs.AttributeOptions{})
	require.NoError(t, err)
	require.Nil(t, got, "zero options read nothing")

	got, err = fs.ReadExtendedAttributes(ctx, f, fs.AttributeOptions{Xattrs: fs.XattrsAll, BSDFlags: true, BirthTime: true})
	require.NoError(t, err)
	require.True(t, sampleAttrs().Equal(got))

	got, err = fs.ReadExtendedAttributes(ctx, plain, fs.AttributeOptions{Xattrs: fs.XattrsAll})
	require.NoError(t, err)
	require.Nil(t, got, "entries without metadata report nil")
}

func TestExtendedAttributes_FilteredACLs(t *testing.T) {
	a := &fs.ExtendedAttributes{
		Xattrs: map[string][]byte{
			"system.posix_acl_access": []byte("acl"),
			"user.a":                  []byte("1"),
		},
		ACL: []byte("filesec"),
	}

	got := a.Filtered(fs.AttributeOptions{ACLs: true})
	require.True(t, (&fs.ExtendedAttributes{
		Xattrs: map[string][]byte{"system.posix_acl_access": []byte("acl")},
		ACL:    []byte("filesec"),
	}).Equal(got), "ACLs are selected independently of the xattr mode: %+v", got)

	got = a.Filtered(fs.AttributeOptions{Xattrs: fs.XattrsAll})
	require.True(t, (&fs.ExtendedAttributes{Xattrs: map[string][]byte{"user.a": []byte("1")}}).Equal(got),
		"ACL xattrs are not captured as plain xattrs: %+v", got)
}

func TestExtendedAttributes_ACLInCloneAndEqual(t *testing.T) {
	a := &fs.ExtendedAttributes{ACL: []byte("x")}
	c := a.Clone()

	require.False(t, a.IsEmpty())
	require.True(t, a.Equal(c))

	c.ACL[0] = 'y'

	require.False(t, a.Equal(c))
	require.Equal(t, []byte("x"), a.ACL)
}
