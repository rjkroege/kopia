package policy_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kopia/kopia/fs"
	"github.com/kopia/kopia/snapshot"
	"github.com/kopia/kopia/snapshot/policy"
)

func TestExtendedMetadataPolicy_EnabledByDefault(t *testing.T) {
	require.Equal(t, fs.AttributeOptions{
		Xattrs:        fs.XattrsAll,
		ExcludeXattrs: policy.DefaultXattrExcludes,
		BSDFlags:      true,
		BirthTime:     true,
		ACLs:          true,
	}, policy.DefaultPolicy.ExtendedMetadataPolicy.AttributeOptions())
}

func TestExtendedMetadataPolicy_AttributeOptions(t *testing.T) {
	p := policy.ExtendedMetadataPolicy{
		Xattrs:       policy.NewXattrMode(fs.XattrsAll),
		XattrExclude: []string{"user.a"},
		FileFlags:    policy.NewOptionalBool(true),
	}

	require.Equal(t, fs.AttributeOptions{
		Xattrs:        fs.XattrsAll,
		ExcludeXattrs: []string{"user.a"},
		BSDFlags:      true,
	}, p.AttributeOptions())
}

func TestExtendedMetadataPolicy_MergeInheritsDefaults(t *testing.T) {
	child := &policy.Policy{ExtendedMetadataPolicy: policy.ExtendedMetadataPolicy{Xattrs: policy.NewXattrMode(fs.XattrsUser)}}
	child.Labels = policy.LabelsForSource(snapshot.SourceInfo{Host: "h", UserName: "u", Path: "/p"})

	merged, def := policy.MergePolicies([]*policy.Policy{child}, child.Target())

	require.Equal(t, policy.NewXattrMode(fs.XattrsUser), merged.ExtendedMetadataPolicy.Xattrs)
	require.Equal(t, child.Target(), def.ExtendedMetadataPolicy.Xattrs)
	require.Equal(t, policy.DefaultXattrExcludes, merged.ExtendedMetadataPolicy.XattrExclude)
	require.Equal(t, policy.GlobalPolicySourceInfo, def.ExtendedMetadataPolicy.XattrExclude)
}

func TestExtendedMetadataPolicy_MergeExplicitNoneOverridesParent(t *testing.T) {
	parent := &policy.Policy{ExtendedMetadataPolicy: policy.ExtendedMetadataPolicy{Xattrs: policy.NewXattrMode(fs.XattrsAll)}}
	parent.Labels = policy.LabelsForSource(snapshot.SourceInfo{Host: "h", UserName: "u"})

	child := &policy.Policy{ExtendedMetadataPolicy: policy.ExtendedMetadataPolicy{Xattrs: policy.NewXattrMode(fs.XattrsNone)}}
	child.Labels = policy.LabelsForSource(snapshot.SourceInfo{Host: "h", UserName: "u", Path: "/p"})

	merged, def := policy.MergePolicies([]*policy.Policy{child, parent}, child.Target())
	require.Equal(t, policy.NewXattrMode(fs.XattrsNone), merged.ExtendedMetadataPolicy.Xattrs)
	require.Equal(t, child.Target(), def.ExtendedMetadataPolicy.Xattrs)

	child.ExtendedMetadataPolicy.Xattrs = nil

	merged, def = policy.MergePolicies([]*policy.Policy{child, parent}, child.Target())
	require.Equal(t, policy.NewXattrMode(fs.XattrsAll), merged.ExtendedMetadataPolicy.Xattrs)
	require.Equal(t, parent.Target(), def.ExtendedMetadataPolicy.Xattrs)
}

func TestExtendedMetadataPolicy_JSON(t *testing.T) {
	b, err := json.Marshal(policy.ExtendedMetadataPolicy{Xattrs: policy.NewXattrMode(fs.XattrsUser)})
	require.NoError(t, err)
	require.JSONEq(t, `{"xattrs":"user"}`, string(b))

	b, err = json.Marshal(policy.ExtendedMetadataPolicy{})
	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(b), "unset mode is omitted")

	var p policy.ExtendedMetadataPolicy

	require.NoError(t, json.Unmarshal([]byte(`{"xattrs":"none"}`), &p))
	require.Equal(t, policy.NewXattrMode(fs.XattrsNone), p.Xattrs)

	require.Error(t, json.Unmarshal([]byte(`{"xattrs":"some"}`), &p))
}

func TestValidateExtendedMetadataPolicy(t *testing.T) {
	require.NoError(t, policy.ValidateExtendedMetadataPolicy(policy.ExtendedMetadataPolicy{}))

	for _, m := range []fs.XattrMode{fs.XattrsNone, fs.XattrsUser, fs.XattrsAll} {
		require.NoError(t, policy.ValidateExtendedMetadataPolicy(policy.ExtendedMetadataPolicy{Xattrs: policy.NewXattrMode(m)}))
	}
}

func TestValidateExtendedMetadataPolicy_InvalidMode(t *testing.T) {
	require.Error(t, policy.ValidateExtendedMetadataPolicy(policy.ExtendedMetadataPolicy{Xattrs: policy.NewXattrMode(fs.XattrsAll + 1)}))
}
