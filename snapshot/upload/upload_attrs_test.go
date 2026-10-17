package upload

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kopia/kopia/fs"
	"github.com/kopia/kopia/internal/testlogging"
	"github.com/kopia/kopia/snapshot"
	"github.com/kopia/kopia/snapshot/policy"
	"github.com/kopia/kopia/snapshot/snapshotfs"
)

func extendedMetadataPolicyTree(mp policy.ExtendedMetadataPolicy) *policy.Tree {
	pol := *policy.DefaultPolicy
	pol.ExtendedMetadataPolicy = mp

	return policy.BuildTree(map[string]*policy.Policy{".": &pol}, &pol)
}

func allMetadataPolicyTree() *policy.Tree {
	return extendedMetadataPolicyTree(policy.ExtendedMetadataPolicy{
		Xattrs:       policy.NewXattrMode(fs.XattrsAll),
		XattrExclude: []string{"com.apple.quarantine"},
		FileFlags:    policy.NewOptionalBool(true),
		BirthTime:    policy.NewOptionalBool(true),
	})
}

// storedAttributes returns the attributes stored in the snapshot for the entry at the given path.
func storedAttributes(ctx context.Context, t *testing.T, th *uploadTestHarness, man *snapshot.Manifest, path ...string) *fs.ExtendedAttributes {
	t.Helper()

	e, err := snapshotfs.SnapshotRoot(th.repo, man)
	require.NoError(t, err)

	for _, name := range path {
		d, ok := e.(fs.Directory)
		require.True(t, ok)

		e, err = d.Child(ctx, name)
		require.NoError(t, err)
	}

	hde, ok := e.(snapshot.HasDirEntry)
	require.True(t, ok)

	return hde.DirEntry().Attributes
}

func testAttrs(v string) fs.ExtendedAttributes {
	return fs.ExtendedAttributes{
		Xattrs: map[string][]byte{
			"user.v":               []byte(v),
			"com.apple.quarantine": []byte("q"),
		},
		BSDFlags:  0x8000,
		BirthTime: 1_600_000_000_000_000_000,
	}
}

func TestUpload_ExtendedAttributesCapturedByDefault(t *testing.T) {
	ctx := testlogging.Context(t)
	th := newUploadTestHarness(ctx, t)
	t.Cleanup(th.cleanup)

	th.sourceDir.Subdir("d1").SetExtendedAttributes(testAttrs("a"))

	man, err := NewUploader(th.repo).Upload(ctx, th.sourceDir, policy.BuildTree(nil, policy.DefaultPolicy), snapshot.SourceInfo{})
	require.NoError(t, err)

	got := storedAttributes(ctx, t, th, man, "d1")
	require.NotNil(t, got)
	require.Equal(t, []byte("a"), got.Xattrs["user.v"])
	require.NotContains(t, got.Xattrs, "com.apple.quarantine", "default excludes apply")
}

func TestUpload_ExtendedAttributesDisabledByPolicy(t *testing.T) {
	ctx := testlogging.Context(t)
	th := newUploadTestHarness(ctx, t)
	t.Cleanup(th.cleanup)

	tree := extendedMetadataPolicyTree(policy.ExtendedMetadataPolicy{Xattrs: policy.NewXattrMode(fs.XattrsNone)})

	before, err := NewUploader(th.repo).Upload(ctx, th.sourceDir, tree, snapshot.SourceInfo{})
	require.NoError(t, err)

	th.sourceDir.SetExtendedAttributes(testAttrs("a"))
	th.sourceDir.Subdir("d1").SetExtendedAttributes(testAttrs("a"))

	after, err := NewUploader(th.repo).Upload(ctx, th.sourceDir, tree, snapshot.SourceInfo{})
	require.NoError(t, err)

	require.Equal(t, before.RootObjectID(), after.RootObjectID(), "manifests must be identical when capture is disabled")
	require.Nil(t, after.RootEntry.Attributes)
	require.Nil(t, storedAttributes(ctx, t, th, after, "d1"))
}

func TestUpload_CapturesExtendedAttributes(t *testing.T) {
	ctx := testlogging.Context(t)
	th := newUploadTestHarness(ctx, t)
	t.Cleanup(th.cleanup)

	th.sourceDir.SetExtendedAttributes(testAttrs("root"))
	th.sourceDir.Subdir("d1").SetExtendedAttributes(testAttrs("dir"))
	th.sourceDir.AddSymlink("link", "f1", defaultPermissions).SetExtendedAttributes(testAttrs("link"))

	th.sourceDir.AddFile("withattrs", []byte("x"), defaultPermissions).SetExtendedAttributes(testAttrs("file"))

	man, err := NewUploader(th.repo).Upload(ctx, th.sourceDir, allMetadataPolicyTree(), snapshot.SourceInfo{})
	require.NoError(t, err)
	require.Zero(t, atomic.LoadInt32(&man.Stats.ErrorCount))

	want := func(v string) *fs.ExtendedAttributes {
		w := testAttrs(v)
		delete(w.Xattrs, "com.apple.quarantine")

		return &w
	}

	require.True(t, want("root").Equal(man.RootEntry.Attributes), "root: %+v", man.RootEntry.Attributes)
	require.True(t, want("dir").Equal(storedAttributes(ctx, t, th, man, "d1")))
	require.True(t, want("link").Equal(storedAttributes(ctx, t, th, man, "link")))

	require.True(t, want("file").Equal(storedAttributes(ctx, t, th, man, "withattrs")), "%+v", storedAttributes(ctx, t, th, man, "withattrs"))

	require.Nil(t, storedAttributes(ctx, t, th, man, "f1"), "entries without metadata store none")
}

func TestUpload_ExtendedAttributeOnlyChangeIsDetected(t *testing.T) {
	ctx := testlogging.Context(t)
	th := newUploadTestHarness(ctx, t)
	t.Cleanup(th.cleanup)

	tree := allMetadataPolicyTree()
	f := th.sourceDir.Subdir("d1").AddFile("tagged", []byte("contents"), defaultPermissions)
	f.SetExtendedAttributes(testAttrs("red"))

	s1, err := NewUploader(th.repo).Upload(ctx, th.sourceDir, tree, snapshot.SourceInfo{})
	require.NoError(t, err)

	f.SetExtendedAttributes(testAttrs("blue"))

	s2, err := NewUploader(th.repo).Upload(ctx, th.sourceDir, tree, snapshot.SourceInfo{}, s1)
	require.NoError(t, err)

	require.NotEqual(t, s1.RootObjectID(), s2.RootObjectID())
	require.Zero(t, atomic.LoadInt32(&s2.Stats.NonCachedFiles), "file contents are reused from the previous snapshot")
	require.Equal(t, atomic.LoadInt32(&s1.Stats.NonCachedFiles), atomic.LoadInt32(&s2.Stats.CachedFiles))
	require.Equal(t, []byte("blue"), storedAttributes(ctx, t, th, s2, "d1", "tagged").Xattrs["user.v"])
	require.Equal(t, []byte("red"), storedAttributes(ctx, t, th, s1, "d1", "tagged").Xattrs["user.v"])
}

func TestUpload_ExtendedAttributeReadErrorKeepsEntry(t *testing.T) {
	for _, ignore := range []bool{false, true} {
		t.Run(map[bool]string{false: "reported", true: "ignored"}[ignore], func(t *testing.T) {
			ctx := testlogging.Context(t)
			th := newUploadTestHarness(ctx, t)
			t.Cleanup(th.cleanup)

			pol := *policy.DefaultPolicy
			pol.ExtendedMetadataPolicy.Xattrs = policy.NewXattrMode(fs.XattrsAll)
			pol.ErrorHandlingPolicy.IgnoreFileErrors = policy.NewOptionalBool(policy.OptionalBool(ignore))
			tree := policy.BuildTree(map[string]*policy.Policy{".": &pol}, &pol)

			th.sourceDir.Subdir("d1").AddFile("bad", []byte("x"), defaultPermissions).FailExtendedAttributes(errTest)

			man, err := NewUploader(th.repo).Upload(ctx, th.sourceDir, tree, snapshot.SourceInfo{})
			require.NoError(t, err)

			if ignore {
				require.Equal(t, int32(1), atomic.LoadInt32(&man.Stats.IgnoredErrorCount))
				require.Zero(t, atomic.LoadInt32(&man.Stats.ErrorCount))
			} else {
				require.Equal(t, int32(1), atomic.LoadInt32(&man.Stats.ErrorCount))
			}

			require.Nil(t, storedAttributes(ctx, t, th, man, "d1", "bad"), "entry is kept, without attributes")
		})
	}
}
