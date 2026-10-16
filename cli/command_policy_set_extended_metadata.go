package cli

import (
	"context"

	"github.com/alecthomas/kingpin/v2"

	"github.com/kopia/kopia/fs"
	"github.com/kopia/kopia/snapshot/policy"
)

type policyExtendedMetadataFlags struct {
	xattrs             string
	addXattrExclude    []string
	removeXattrExclude []string
	clearXattrExclude  bool
	fileFlags          string
	birthTime          string
}

func (c *policyExtendedMetadataFlags) setup(cmd *kingpin.CmdClause) {
	cmd.Flag("xattrs", "Extended attributes to capture ('none', 'user', 'all', 'inherit')").EnumVar(&c.xattrs,
		string(fs.XattrsNone), string(fs.XattrsUser), string(fs.XattrsAll), inheritPolicyString, defaultPolicyString)
	cmd.Flag("add-xattr-exclude", "Extended attribute names to never capture (a trailing '*' matches a prefix)").PlaceHolder("NAME").StringsVar(&c.addXattrExclude)
	cmd.Flag("remove-xattr-exclude", "Extended attribute names to remove from the exclude list").PlaceHolder("NAME").StringsVar(&c.removeXattrExclude)
	cmd.Flag("clear-xattr-exclude", "Clear the extended attribute exclude list").BoolVar(&c.clearXattrExclude)
	cmd.Flag("file-flags", "Capture BSD file flags ('true', 'false', 'inherit')").EnumVar(&c.fileFlags, booleanEnumValues...)
	cmd.Flag("birth-time", "Capture file creation times ('true', 'false', 'inherit')").EnumVar(&c.birthTime, booleanEnumValues...)
}

func (c *policyExtendedMetadataFlags) setExtendedMetadataPolicyFromFlags(ctx context.Context, p *policy.ExtendedMetadataPolicy, changeCount *int) error {
	applyPolicyXattrMode(ctx, &p.Xattrs, c.xattrs, changeCount)
	applyPolicyStringList(ctx, "xattr exclude list", &p.XattrExclude, c.addXattrExclude, c.removeXattrExclude, c.clearXattrExclude, changeCount)

	if err := applyPolicyBoolPtr(ctx, "file flags", &p.FileFlags, c.fileFlags, changeCount); err != nil {
		return err
	}

	return applyPolicyBoolPtr(ctx, "birth time", &p.BirthTime, c.birthTime, changeCount)
}

func applyPolicyXattrMode(ctx context.Context, val *fs.XattrMode, str string, changeCount *int) {
	switch str {
	case "":
		return

	case inheritPolicyString, defaultPolicyString:
		*changeCount++

		log(ctx).Info(" - resetting \"xattrs\" to a default value inherited from parent.")

		*val = ""

	default:
		*changeCount++

		log(ctx).Infof(" - setting \"xattrs\" to %v.", str)

		*val = fs.XattrMode(str)
	}
}
