---
title: "Extended Metadata"
linkTitle: "Extended Metadata"
weight: 75
---

## Extended Metadata

By default, Kopia snapshots store each entry's contents, permissions, owner, group and
modification time. On macOS and Linux, Kopia can also capture:

* **Extended attributes** (xattrs), such as Finder information (including the "bundle" bit that
  makes Finder show a directory as a single item), Finder tags and labels, resource forks, and
  code signatures of non-Mach-O files (`com.apple.cs.*`).
* **BSD file flags** (macOS `chflags`): `hidden`, `uchg`, `uappnd`, `nodump`, `opaque` and the
  corresponding system flags.
* **Birth (creation) time**.
* **Access control lists**: macOS ACLs, and POSIX ACLs on Linux.

Capturing them is disabled by default. Enable it with a policy, for example for all snapshots:

```shell
kopia policy set --global --xattrs=all --file-flags=true --birth-time=true --acls=true
```

| Policy option | Values | Default |
| --- | --- | --- |
| `--xattrs` | `none`, `user` (the `user.` namespace on Linux; all attributes on macOS), `all` | `none` |
| `--add-xattr-exclude`, `--remove-xattr-exclude`, `--clear-xattr-exclude` | attribute names; a trailing `*` matches a prefix | `com.apple.lastuseddate#PS`, `com.apple.macl`, `com.apple.provenance`, `com.apple.quarantine` |
| `--file-flags` | `true`, `false` | `false` |
| `--birth-time` | `true`, `false` | `false` |
| `--acls` | `true`, `false` | `false` |

A non-empty exclude list replaces the one inherited from the parent policy.

### What is stored

Extended metadata is stored inline in the directory listing of each snapshot. A single
attribute value is limited to 8 KiB; larger values are not captured, are logged as a warning,
and are listed by `kopia ls -l -@`. Kopia clients that predate this feature ignore the extra
data, so they can still list, verify and restore such snapshots (without the extended metadata).

A change that affects only extended metadata, for example adding a Finder tag, is picked up by
the next snapshot without re-reading the file's contents.

Some metadata is never captured because it describes how the data is stored rather than the
data itself: compressed (`UF_COMPRESSED`), tracked (`UF_TRACKED`) and dataless iCloud
placeholder (`SF_DATALESS`) flags. Dataless files are downloaded when Kopia reads them and are
restored as regular files.

### Restoring

`kopia snapshot restore` to a local directory applies whatever extended metadata the snapshot
contains. Use `--skip-xattrs` and `--skip-file-flags` to leave them out; `--skip-times` also
skips birth times and `--skip-permissions` also skips ACLs.

Attributes protected by macOS System Integrity Protection (`com.apple.macl`,
`com.apple.provenance`, `com.apple.rootless`) are skipped. When the target filesystem does not
support some metadata, or the user is not allowed to set it (for example system file flags
without root), restore continues and prints a summary warning.

Birth times cannot be set on Linux. ACL entries refer to users and groups by their IDs, as
owners do, so on a different machine they apply to whichever users and groups have those IDs.

Restoring to a `.tar` file writes extended attributes, file flags and birth times as PAX
records (`SCHILY.xattr.*`, `SCHILY.fflags`, `LIBARCHIVE.creationtime`) that GNU tar and
`bsdtar` understand, for example `bsdtar -x --xattrs --fflags -f snapshot.tar`. ACLs are not
included in tar output, and zip output carries no extended metadata.

### Inspecting

`kopia ls -l -@ <path>` lists each entry's extended attribute names and sizes, file flags,
birth time and ACL. `kopia diff` reports entries whose extended metadata changed.
