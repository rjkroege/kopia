package snapshot_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kopia/kopia/fs"
	"github.com/kopia/kopia/snapshot"
)

func TestDirEntry_JSONWithoutAttributesIsUnchanged(t *testing.T) {
	de := &snapshot.DirEntry{Name: "f", Type: snapshot.EntryTypeFile, FileSize: 3}

	b, err := json.Marshal(de)
	require.NoError(t, err)
	require.JSONEq(t, `{"name":"f","type":"f","size":3,"obj":""}`, string(b))
}

func TestDirEntry_JSONRoundTripWithAttributes(t *testing.T) {
	de := &snapshot.DirEntry{
		Name: "Foo.app",
		Type: snapshot.EntryTypeDirectory,
		Attributes: &fs.ExtendedAttributes{
			Xattrs:   map[string][]byte{"com.apple.FinderInfo": make([]byte, 32)},
			BSDFlags: 0x8000,
		},
	}

	b, err := json.Marshal(de)
	require.NoError(t, err)

	var got snapshot.DirEntry

	require.NoError(t, json.Unmarshal(b, &got))
	require.True(t, de.Attributes.Equal(got.Attributes))
}

// legacyDirEntry mirrors DirEntry as it was before the Attributes field was added.
type legacyDirEntry struct {
	Name     string             `json:"name,omitempty"`
	Type     snapshot.EntryType `json:"type,omitempty"`
	FileSize int64              `json:"size,omitempty"`
	ObjectID string             `json:"obj"`
}

func TestDirEntry_LegacyDecoderIgnoresAttributes(t *testing.T) {
	de := &snapshot.DirEntry{
		Name:       "f",
		Type:       snapshot.EntryTypeFile,
		FileSize:   3,
		Attributes: &fs.ExtendedAttributes{Xattrs: map[string][]byte{"user.a": []byte("b")}},
	}

	b, err := json.Marshal(de)
	require.NoError(t, err)

	var got legacyDirEntry

	require.NoError(t, json.Unmarshal(b, &got))
	require.Equal(t, legacyDirEntry{Name: "f", Type: snapshot.EntryTypeFile, FileSize: 3}, got)
}

func TestDirEntry_CloneCopiesAttributes(t *testing.T) {
	de := &snapshot.DirEntry{Attributes: &fs.ExtendedAttributes{Xattrs: map[string][]byte{"user.a": []byte("b")}}}
	c := de.Clone()

	c.Attributes.Xattrs["user.a"][0] = 'x'

	require.Equal(t, []byte("b"), de.Attributes.Xattrs["user.a"])
}
