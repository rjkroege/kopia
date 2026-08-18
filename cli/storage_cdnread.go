//go:build !no_extra_providers

package cli

import (
	"context"

	"github.com/alecthomas/kingpin/v2"

	"github.com/kopia/kopia/repo/blob"
	"github.com/kopia/kopia/repo/blob/cdnread"
)

type storageCDNReadFlags struct {
	options cdnread.Options
}

func (c *storageCDNReadFlags) Setup(svc StorageProviderServices, cmd *kingpin.CmdClause) {
	cmd.Flag("bucket", "Name of the bucket (S3-compatible write path)").Required().StringVar(&c.options.BucketName)
	cmd.Flag("endpoint", "S3-compatible endpoint for the write path, e.g. s3.us-west-004.backblazeb2.com").Required().StringVar(&c.options.Endpoint)
	cmd.Flag("region", "Region to use for the write path").Default("").StringVar(&c.options.Region)
	cmd.Flag("access-key", "Access key ID (overrides KOPIA_CDNREAD_ACCESS_KEY_ID environment variable)").Required().Envar(svc.EnvName("CDNREAD_ACCESS_KEY_ID")).StringVar(&c.options.AccessKeyID)
	cmd.Flag("secret-access-key", "Secret access key (overrides KOPIA_CDNREAD_SECRET_ACCESS_KEY environment variable)").Required().Envar(svc.EnvName("CDNREAD_SECRET_ACCESS_KEY")).StringVar(&c.options.SecretAccessKey)
	cmd.Flag("prefix", "Prefix to use for objects in the bucket").StringVar(&c.options.Prefix)
	cmd.Flag("cdn-url", "Base URL of the CDN serving the unauthenticated read path, e.g. https://assets.example.com").Required().StringVar(&c.options.CDNURL)

	commonThrottlingFlags(cmd, &c.options.Limits)
}

func (c *storageCDNReadFlags) Connect(ctx context.Context, isCreate bool, formatVersion int) (blob.Storage, error) {
	_ = formatVersion

	//nolint:wrapcheck
	return cdnread.New(ctx, &c.options, isCreate)
}

func init() {
	mustRegisterStorageProvider(
		"cdnread",
		"a bucket with an S3-compatible write path and an unauthenticated CDN read path (split read/write)",
		func() StorageFlags { return &storageCDNReadFlags{} },
	)
}
