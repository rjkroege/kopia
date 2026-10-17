package policy_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kopia/kopia/fs"
	"github.com/kopia/kopia/snapshot"
	"github.com/kopia/kopia/snapshot/policy"
)

func TestExtendedMetadataPolicy_DisabledByDefault(t *testing.T) {
	opts := policy.DefaultPolicy.ExtendedMetadataPolicy.AttributeOptions()

	require.True(t, opts.IsZero())
	require.Equal(t, policy.DefaultXattrExcludes, opts.ExcludeXattrs)
}

func TestExtendedMetadataPolicy_AttributeOptions(t *testing.T) {
	p := policy.ExtendedMetadataPolicy{
		Xattrs:       fs.XattrsAll,
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
	child := &policy.Policy{ExtendedMetadataPolicy: policy.ExtendedMetadataPolicy{Xattrs: fs.XattrsUser}}
	child.Labels = policy.LabelsForSource(snapshot.SourceInfo{Host: "h", UserName: "u", Path: "/p"})

	merged, def := policy.MergePolicies([]*policy.Policy{child}, child.Target())

	require.Equal(t, fs.XattrsUser, merged.ExtendedMetadataPolicy.Xattrs)
	require.Equal(t, child.Target(), def.ExtendedMetadataPolicy.Xattrs)
	require.Equal(t, policy.DefaultXattrExcludes, merged.ExtendedMetadataPolicy.XattrExclude)
	require.Equal(t, policy.GlobalPolicySourceInfo, def.ExtendedMetadataPolicy.XattrExclude)
}

func TestValidateExtendedMetadataPolicy(t *testing.T) {
	for _, m := range []fs.XattrMode{"", fs.XattrsNone, fs.XattrsUser, fs.XattrsAll} {
		require.NoError(t, policy.ValidateExtendedMetadataPolicy(policy.ExtendedMetadataPolicy{Xattrs: m}))
	}
}

func TestValidateExtendedMetadataPolicy_InvalidMode(t *testing.T) {
	require.Error(t, policy.ValidateExtendedMetadataPolicy(policy.ExtendedMetadataPolicy{Xattrs: "some"}))
}
