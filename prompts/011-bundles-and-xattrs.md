---
id: 011
title: Support macOS bundles and extended attributes (xattrs) in snapshot and restore
status: complete
priority: medium
type: feature
created: 2026-10-16
updated: 2026-10-27
branch: feat/xattrs-bundles
tags: [fs, snapshot, restore, macos, xattr]
---

## Goal

A snapshot of a macOS directory tree, once restored, matches the original in
the ways that matter to macOS. In particular:

- **Extended attributes** are captured and restored, including
  `com.apple.FinderInfo`, `com.apple.ResourceFork`, `com.apple.metadata:*`
  (tags), `com.apple.quarantine`, and arbitrary user xattrs.
- **Bundles and packages** (`.app`, `.photoslibrary`, `.pages`, `.rtfd`,
  `.bundle`, and so on) restore as working bundles. Finder shows them as a
  single item, and apps launch and code signatures stay valid.

## Context

As of `main` @ abd5ff69:

- Kopia has **no xattr support at all**. No Go file references
  `xattr`/`Getxattr`/`Listxattr`.
- `fs.Entry` (`fs/entry.go:19`) exposes only `os.FileInfo`, owner, and
  device.

my intuition is to add a new interface here for xattr.

- `snapshot.DirEntry` (`snapshot/manifest.go:121`) stores name, type, mode,
  size, mtime, uid, gid, object ID, and dir summary. There is no field for
  xattrs, flags, ACLs, or birth time.

my inutition is to add them here.

- Kopia already walks bundles as ordinary directories, so their *contents*
  are backed up. What gets lost is the metadata that makes them bundles or
  keeps them valid:
  - `com.apple.FinderInfo` carries the "has bundle bit" and Finder flags,
    which matter for packages that don't rely on a known extension.
  - Code-signed `.app` bundles: the signature lives in files
    (`_CodeSignature/`), but some apps also keep signature-relevant data in
    xattrs on files inside the bundle. Restoring without them can break
    `codesign --verify`.
  - Resource forks (`com.apple.ResourceFork`) are still in use by older
    files, fonts, and some aliases.
- Restore paths to touch: `snapshot/restore/local_fs_output*.go` (local),
  `tar_output.go` (PAX `SCHILY.xattr.*` / `LIBARCHIVE.xattr.*` headers), and
  `zip_output.go`, where AppleDouble `._` files are a possible fallback.
- Upload path: `fs/localfs` produces the entries, and `snapshot/upload`
  writes the `DirEntry` values.
- Related: macOS FSKit work (kopiabrowser, mounted snapshots). A mounted
  snapshot would want to expose xattrs too, through `getxattr` and
  `listxattr` on the FSKit volume.

True. But that's not in scope yet. 

- Upstream: check for an existing kopia/kopia issue or PR on xattrs before
  designing. Upstream acceptance shapes the format decision.

There's at least one comment that I made somewhere. But let's figure out first *how* to build the feature.

## Task

Research first, then a proposal, then an implementation. Build this in a new branch.

1. **Survey.** Look for upstream issues and PRs on xattrs, ACLs, and
   bundles. See how restic, borg, and Time Machine store xattrs: inline
   bytes or content-addressed, and with what size limits.
2. **Format design.** Choose how `DirEntry` carries xattrs:
   - (a) Inline: `Xattrs map[string][]byte \`json:"xattr,omitempty"\``. This
     is simple, but large values (resource forks can be MBs) bloat
     directory manifests.
   - (b) Content-addressed: store each xattr value as an object and keep
     `map[string]object.ID` in `DirEntry`. This deduplicates, and values
     common across many files are stored once.
   - (c) A hybrid: inline below a threshold (for example 1 KiB), object
     above it.

Write a program as part of the research phase to walk home directory obeying the
the .kopiaignore directives to histogram the size of the xattr and additional status bits.
show me the histogram and we will discuss.

   - Repository format and compatibility: older kopia clients must ignore the
     new field safely. JSON `omitempty` plus unknown-field tolerance probably
     covers this. Verify it with `compat-test`.
3. **Capture** in `fs/localfs`. Add an optional interface, for example
   `fs.XattrProvider { Xattrs(ctx) (map[string][]byte, error) }`, so that
   non-local filesystems are unaffected. Use `golang.org/x/sys/unix`
   `Listxattr`/`Getxattr` with `XATTR_NOFOLLOW` for symlinks. Handle Linux
   namespaces (`user.*`, plus `security.*` and `trusted.*` only when
   privileged).
4. **Policy.** Add a snapshot policy knob (`--xattrs=none|user|all`) and an
   exclude list (for example, skip `com.apple.quarantine` and
   `com.apple.lastuseddate#PS`). Changes to xattrs must count as changes
   for incremental snapshots, meaning the cached-entry comparison has to
   look at xattrs.
5. **Restore.** Write xattrs after file contents and before the final
   mtime and mode. Order matters: some xattrs are cleared by writes.
   Add a `--skip-xattrs` flag. Treat a failure on unsupported filesystems
   as a warning, not an error.
6. **Bundles.** Confirm that restoring FinderInfo plus contents is enough.
   Test a signed `.app` with `codesign --verify --deep`, plus a
   `.photoslibrary` and an `.rtfd`.
7. **Tests.** Add unit tests for capture and restore round trips on darwin
   and linux, add tar output with PAX xattr headers, and extend the e2e
   snapshot/restore tests with xattr'd files.

## Acceptance criteria

- [ ] Design note in this doc picks the inline, object, or hybrid storage
      format, with its rationale and compatibility analysis.

- [ ] `kopia snapshot create` captures xattrs on macOS and Linux when the
      policy enables it.
- [ ] `kopia snapshot restore` to the local fs restores them, so that
      `xattr -l` / `getfattr -d` output is identical before and after.
- [ ] A restored, code-signed `.app` passes `codesign --verify --deep
      --strict` and launches.

Addition:
- [ ] `kopia snapshot create` will preserve the bundle attributes
- [ ] and restore the bundle attributes on restore

- [ ] Finder shows restored bundles as single items. Tags and colors
      survive.
- [ ] Tar restore emits PAX xattr headers.
- [ ] Older kopia clients can still read snapshots that contain xattrs
      (`compat-test` is green).
- [ ] Incremental snapshots detect a change that affects only xattrs.

## Notes

- Open question: what does "bundles" mean for your use case? It could be
  restore fidelity (above), or presenting bundles correctly in a mounted
  snapshot or kopiabrowser (FSKit side), or both.

I meant restore bundles. So that the restore carries the "has bundle bit" and Finder flags

- Open question: should ACLs and BSD file flags (`chflags`: `hidden`,
  `uchg`) and birth time come along in the same change? They're adjacent
  and have the same "missing metadata" shape.

Probably.

- Open question: is this aimed at upstream (needs discussion with
  maintainers first) or fork-only on `main`?

Fork first to assess the approach.

- macOS `com.apple.provenance` and `com.apple.macl` are SIP-protected and
  can't be written back. Restore must skip them quietly.

Yes. This seems reasonable.
## Research findings (2026-10-17)

### Upstream

- kopia/kopia#3643 (miekg, closed 2024-08, abandoned) implemented xattrs
  on Linux. jkowalski's review asked for:
  - an extensible container for non-core attributes (xattr, winacl, extra
    stat fields), for example a strongly typed struct;
  - `make lint-all` clean on every platform;
  - comprehensive tests;
  - policy-gated capture and restore, **off by default**, since fetching
    xattrs is expensive;
  - filtering by attribute name.
- kopia/kopia#544 is the umbrella issue for missing metadata (setuid,
  xattrs, ACLs, hardlinks, ...).
- #4026: macOS tags lost, creation date restored as 1970.
- #3587 / PR #5084: docs on which metadata is backed up.
- I found no xattr comment from you upstream (per your note, ignore that).

### Other tools

| | restic | borg | Time Machine |
|---|---|---|---|
| storage | inline base64 in the tree node, no cap | inline in the msgpack item; the item stream is chunked as a whole | native fs |
| flags / ACL / btime | no (restic#1622) | yes (bsdflags, ACL text, birthtime) | native |
| restore order | content, chown, xattr, times, chmod | content, chown, chmod, ACL, xattr, btime, mtime, **flags last** | n/a |
| failures | ENOTSUP silent; a failed get is only a warning (restic#4929 lost data that way) | per-name warning | n/a |

Writing a ResourceFork bumps mtime (borg#7234), so mtime goes after
xattrs. Non-Mach-O code signatures live in `com.apple.cs.*` xattrs.

### Histogram of `~` (tools/xattrsurvey; 400k entries, .kopiaignore honored)

- Raw: 92% of entries have xattrs, but 81% of the values are
  `com.apple.provenance` (SIP, can't be restored) and 13% are
  `com.apple.quarantine`.
- With the default excludes (provenance, macl, quarantine,
  lastuseddate#PS, cscachefs):
  - 2% of entries have xattrs; 18.5k values totaling 879 KiB.
  - The largest value is under 4 KiB, and there are no resource forks.
  - Inlining everything adds 1.8 MiB of JSON across all manifests (before
    compression); the worst single directory grows by 93 KiB.
  - Object references (~70 B each) are larger than the median value, so
    content-addressing saves nothing.
- st_flags: UF_TRACKED 5.7k, UF_HIDDEN 1.1k, UF_COMPRESSED 240,
  SF_DATALESS 5.
- Birth time differs from mtime on 27k entries.
- ACLs: 13 entries, all stock "everyone deny delete".
- kHasBundle is set only on .musiclibrary, .pxd, .tvlibrary, .iapresenter
  and .pages directories (26 in all). `.app`, `.xcodeproj` and
  `.photoslibrary` are bundles by extension and have no FinderInfo.

## Design

**Storage: inline only, capped at 8 KiB per value.** If a value is over the
cap, the xattr is skipped, a warning is logged, and its name is recorded in
the entry so the loss is visible. The data, restic and borg all point the
same way. If resource forks show up in practice, we can add a hybrid later
as a new field without a format break.

```go
// fs/attrs.go: lives in fs because snapshot imports fs (like DirectorySummary).
type ExtendedAttributes struct {
	Xattrs        map[string][]byte `json:"x,omitempty"`     // base64 in JSON; keys sorted by encoding/json
	SkippedXattrs []string          `json:"xskip,omitempty"` // over the size cap, or unreadable and ignored by policy
	BSDFlags      uint32            `json:"flags,omitempty"` // darwin st_flags, minus UF_TRACKED|UF_COMPRESSED|SF_DATALESS
	BirthTime     *UTCTimestamp     `json:"btime,omitempty"`
	ACL           []byte            `json:"acl,omitempty"`   // darwin kauth_filesec, opaque (later CL)
}

// snapshot.DirEntry gains:
Attributes *fs.ExtendedAttributes `json:"attrs,omitempty"`
```

- **Compatibility.** The field is new and omitted when empty. Older
  clients decode `DirEntry` with `encoding/json` and ignore unknown
  fields, and snapshots without xattrs are byte-identical to today's.
  Verified with compat-test in CL 2.
- **Determinism.** encoding/json sorts map keys, so unchanged metadata
  produces identical manifests, which keeps dedup intact.
- **Incremental snapshots.** On a cache hit, `newCachedDirEntry`
  (`snapshot/upload/upload.go:479`) already rebuilds the `DirEntry` from
  the live entry, so an xattr-only change lands in the new manifest
  without re-reading content. `metadataEquals` stays unchanged.
- **Dataless (iCloud) files** are materialized. Reading the content faults
  them in; SF_DATALESS is masked out and never restored. Dropping them
  would look like silent data loss.
- **Bundles.** Restoring FinderInfo (kHasBundle) plus the contents is
  enough. Default excludes must never touch `com.apple.cs.*` or FinderInfo.
- **No cgo.** Release builds use CGO_ENABLED=0, so everything goes through
  `golang.org/x/sys/unix` (listxattr/getxattr NOFOLLOW, Lstat,
  setattrlist for crtime and ACL).

## CLs

### Branch and PR structure

- **One CL = one commit = one branch = one PR**, stacked. Each commit
  builds, passes `make lint` and has its own tests.
- **Upstream part: CLs 1–10, feature off by default.**
  - The stack root is `76e4e8ea`, which is both the current
    `upstream/master` and `main`'s merge-base with it. The stack therefore
    applies cleanly to kopia/kopia and diffs cleanly against `main`.
  - It contains no fork-only code, and it never depends on fork-only code
    such as storj or the CI gate changes.
  - Branches: `xattr/01-fs-attrs`, `xattr/02-direntry`, ...,
    `xattr/10-docs`, each based on the previous one. Fork PRs are stacked
    the same way: PR 1 has base `main`, and PR n has base
    `xattr/<n-1>-…`.
  - Commit and PR titles use upstream's `type(scope): summary` form, which
    also matches main's title regex.
- **Fork-only CLs on top: F1–F2**, never sent upstream.
- **Landing on `main`.** Merge bottom-up as merge commits (main's rule),
  retargeting each next PR to `main` after its parent merges.
- **Upstream later.** The CL 1–10 branches are pushed to the fork as is.
  Upstream PR 1 targets `kopia/kopia:master`. Each later one is opened
  once its parent lands, or as a draft that says "depends on #…", because
  GitHub can't base a cross-fork PR on a fork branch. If upstream has
  moved by then, the stack is rebased onto the new `upstream/master`.

### Upstream stack (CLs 1–10)

1. **fs: ExtendedAttributes type + optional reader interface.**
   - `fs.ExtendedAttributes` with Clone/Equal.
   - `fs.ExtendedAttributesReader { ExtendedAttributes(ctx, fs.AttributeOptions) (*fs.ExtendedAttributes, error) }`.
   - `AttributeOptions`: xattr mode none|user|all, exclude list, 8 KiB cap.
   - Forwarding in the embedding wrappers (`ignorefs`, `loggingfs`,
     `cachefs`), since embedding hides the method.
   - `virtualfs` and `mockfs` support for tests.
2. **snapshot: `DirEntry.Attributes` + compat.** Field, deep-copy in
   `Clone`, JSON round-trip tests. Extend `tests/compat_test` so an older
   kopia reads a snapshot containing attrs. `snapshotfs` repo entries
   expose attrs through the reader interface, so restore and mount get
   them for free.
3. **localfs capture (darwin, linux).**
   - darwin: xattrs (Llistxattr/Lgetxattr), st_flags with the mask, and
     birth time from the existing Lstat.
   - linux: `user.*`; `security.*`, `trusted.*` and `system.*` only in
     "all" mode and only when privileged.
   - Errors and ENOTSUP handling; the size cap goes to `SkippedXattrs`.
   - Unit tests on temp files per OS. Windows is a no-op.
4. **policy: metadata knobs.**
   - `FilesPolicy.Xattrs` (none|user|all; default none, as upstream wants;
     we set `all` on our global policy).
   - `XattrExclude` with built-in defaults (provenance, macl, quarantine,
     lastuseddate#PS, cscachefs).
   - `FileFlags` and `BirthTime` bools.
   - Merge, `policy set/show` flags (`--xattrs`, `--add/remove-xattr-exclude`,
     `--file-flags`, `--birth-time`), tests.
5. **upload: populate `DirEntry.Attributes`.**
   - Plumb the effective policy into `newDirEntry` and `newCachedDirEntry`.
   - Compat test (moved here from CL 2, because nothing writes attrs
     before this CL): kopia 0.17 lists and restores a snapshot created
     with attrs.
   - Read errors follow `IgnoreFileErrors` and are counted in the snapshot
     stats (restic#4929: no silent loss).
   - Tests: an xattr-only change between two snapshots changes the
     manifest and reuses the content object.
6. **restore: local fs.**
   - Order: content, chown, chmod, ACL (later), xattrs, birth time
     (setattrlist ATTR_CMN_CRTIME), mtime, flags last (so uchg can't block
     the other writes).
   - Directories get theirs in `FinishDirectory`.
   - When overwriting, remove target xattrs that aren't in the snapshot.
   - SIP names (provenance, macl) are skipped quietly. Per-name
     EPERM/ENOTSUP is a warning with a count; `IgnorePermissionErrors` is
     respected.
   - Flags `--skip-xattrs` and `--skip-file-flags`. Shallow restore keeps
     attrs in the placeholder.
   - Round-trip tests on darwin and linux.
7. **restore: tar output.**
   - PAX records `SCHILY.xattr.<name>` plus `LIBARCHIVE.xattr.<name>`
     (base64), `SCHILY.fflags`, `LIBARCHIVE.creationtime`.
   - Test: on darwin, `bsdtar -x` reproduces the xattrs.
   - Zip and AppleDouble are deferred (not in scope).
8. **e2e fidelity tests, including signed executables** (darwin-gated).
   Snapshot, restore, then compare `xattr -l`, `ls -lO@e` and
   `stat -f %B`. Cases:
   - an ad-hoc-signed (`codesign -s -`) Mach-O binary;
   - an ad-hoc-signed shell script, whose signature lives in
     `com.apple.cs.*` xattrs; negative control: restoring with
     `--skip-xattrs` must fail `codesign --verify`;
   - a minimal signed `.app` (`codesign --deep`) and a `ditto` copy of a
     system app such as Calculator.app, both verified with
     `codesign --verify --deep --strict` and `spctl`-free launch checks;
   - an `.rtfd`, a kHasBundle directory without a known extension, and
     Finder tags and colors.

   Linux: `setfattr`/`getfattr -d` round trip.
9. **ACLs** (optional, low prevalence).
   - darwin: getattrlist/setattrlist `ATTR_CMN_EXTENDED_SECURITY`, which
     needs no cgo.
   - linux: `system.posix_acl_*` already comes through in "all" mode.
   - Gated by a policy bool.
10. **UX + docs.** `kopia ls -l` marker for attrs, `internal/diff` reports
    attr changes, and site docs on which metadata is backed up (upstream
    #3587).

### Fork-only CLs (F1–F2), stacked on CL 10

F1. **feat(policy): enable extended metadata by default (fork only).**
    - In `DefaultPolicy`: `Xattrs: all`, `FileFlags: true`,
      `BirthTime: true`, plus ACLs if CL 9 landed.
    - Restore defaults are unchanged, since restore already applies
      whatever the snapshot carries.
    - Tests updated for the new defaults.
    - This is the one CL that makes `main` and `origin/main` differ in
      behavior from upstream.
F2. **chore(tools): xattrsurvey research tool (fork only).**
    - `tools/xattrsurvey`, with lint fixes (clock/timetrack, `darwin &&
      cgo` tag).
    - The `plan(ai)` summary commit (from the repo's commit instructions)
      goes here, not into the upstream stack.

The design note in this doc stays in the untracked `prompts/` directory.

Out of scope: FSKit/mount `getxattr`, zip AppleDouble, Linux chattr flags,
Windows ACLs/ADS, upstream PR (fork first).

## Progress (2026-10-17)

Implemented on the stacked branches `xattr/01-fs-attrs` … `xattr/10-docs` (upstream,
disabled by default, rooted at 76e4e8ea) plus `xattr/F1-default-on` and
`xattr/F2-survey-tool` (fork only). `make test` is green at the top of both stacks, and
every commit builds and vets on darwin, linux and windows. Deviations from the plan:

- **Stale xattrs are not removed** when restoring over existing files. A snapshot with
  capture enabled but no xattrs on an entry is indistinguishable from capture disabled.
- **Restore failures** are warnings, summarized once when restore finishes, not one
  per file.
- **Tar output** fixed `Mode` to carry permission bits only. PAX can't encode
  `os.ModeDir`; this was an existing latent bug.
- **ACL capture on darwin** uses a raw `getattrlist` via `Syscall6`, with a nolint and a
  comment. x/sys has no wrapper, and cgo is off in release builds.
- **Shallow placeholders** report the attributes stored in the placeholder. The fork test
  run caught this case.
- **Signed code** was verified by e2e tests: ad-hoc tool, script and .app, and a
  Developer-ID app (Acorn.app via `KOPIA_TEST_SIGNED_APP`), all passing
  `codesign --verify --deep --strict` after restore. GUI launch was not exercised.
- **Code-signature sizes:** a survey of `/Applications` found no `com.apple.cs.*` xattrs,
  and nothing over 2 KiB, so the 8 KiB cap doesn't affect bundles.

## Resume here (updated 2026-10-27): branch-by-branch review

### State

- **In `main`:** 01–10, the whole upstream-clean part (`main` = `93a1d838`).
  - 01+02 via #503, 03 via #504, 04 via #505, 05 (`388a87fa`) via #506, 06 (`5f82a56f`) via #514.
  - 07 (`8ac08285`) via #515 (`3773ac75`). 08 (`67548fdc`) via #516 (`a93185b3`). 09 (`fa06c01e`) via #517 (`d81a5d35`). 10 (`3d53f691`) via #518 (`93a1d838`).
  - **Stack PRs keep their stack base** (decided 2026-10-27: PR *N* is based on `xattr/<N-1>-…`, like the upstream-rooted stack), so GitHub never marks them merged. After each merge PR lands, the stack PR is closed with "Landed in main via #<merge PR> (merge commit <sha>)." #493–#496 are closed.
  - 10's port notes: `MaxXattrSize`/`SkippedXattrs` dropped from `ls -@` and diff; duplicate nil `testBaseEntry.ExtendedAttributes` stub removed from diff_test; review switched `describeExtendedAttributeChanges` to `strings.Builder`. In `main`, `kopia ls -j -@` ignores `-@` (fork `-j` JSON has no extended metadata): possible follow-up.
- **F1** (`47dd95f1`) via #519 (`d51d4c4d`); #497 closed. Port notes: `NewXattrMode(fs.XattrsAll)` in `DefaultPolicy`; CLI test exclude `user.skip*` → `user.skip`; `upload_attrs_test` disabled-by-policy case needed `policy.NewXattrMode(fs.XattrsNone)`.
- **F2** (`acb6cd69` plan(ai) + `f8fde16c` tool, on `47dd95f1`) via #520 (`e8cc4e0a`); #498 closed. Port: dropped the removed `MaxXattrSize` option (values over 8 KiB are now only logged, not histogrammed), and `-exclude` help says exact names.
- **Stack complete (2026-10-27):** 01–10 and F1–F2 are all in `main` (`e8cc4e0a`); every stack PR #487–#498 is closed with a pointer. The merge branches `xattr/merge-*` can be deleted.
- Follow-ups: `kopia ls -j -@` fixed in #521 (`feat/ls-json-attrs`, worktree `../kopia-sync`; JSON `ExtendedAttributes` with xattr sizes, not values). Still open: upstream PRs for 01–10 (rebase onto then-current `upstream/master`).
- **Worktrees:**
  - `../kopia-xattr`: stack branches; `xattr/10-docs` (`3d53f691`) is checked out.
  - `../kopia-sync`: merge branches; `xattr/merge-10` checked out (recreate each with `switch -C`).
  - `../kopia-ckpt`: holds the built checklocks tool.
  - The main checkout has untracked storj files and can't build upstream-based branches.
- **Bash tool shell quirks:** not POSIX sh. `cd` doesn't persist (prefix each command), `<(…)`/`<{…}` process substitution fails (use scratchpad temp files), and `echo ======` errors (quote it).

### Recipe for CL *N* (F1 next: N−1 head = `3d53f691`, old parent = `f52ef5bf`)

1. **Rebase and port** in `../kopia-xattr`:
   ```
   git switch xattr/F1-default-on
   git rebase --onto 3d53f691 f52ef5bf xattr/F1-default-on
   ```
   Resolve conflicts against the reviewed API (list at the end of this section). Conflict resolutions go straight into the commit (`git add`, `GIT_EDITOR=true git rebase --continue`); anything done after the rebase is a `git commit --fixup=HEAD`. Check what the reviews of earlier CLs changed on purpose before taking *N*'s side (`git diff <old N-1> <new N-1> -- <file>`); 09's port nearly undid 06's review this way.
2. **Local checks** (section below), plus e2e:
   `KOPIA_TEST_SIGNED_APP=/Applications/Acorn.app go test -tags testing -count=1 -run 'ExtendedAttributes|SkipXattrs|XattrOnlyChange|MacOSMetadata|SignedCode|InstalledAppSignature' ./tests/end_to_end_test/` (in-process kopia, no binary needed; Calculator.app skips on app-management protection). For 10 also `go test -tags testing ./internal/diff/` and lint `./internal/...`.
3. **User force-pushes** *N*: `git -C ../kopia-xattr push --force-with-lease origin xattr/10-docs`. The PR keeps its stack base and shows one commit.
4. **User reviews.** Fetch with `gh api repos/rjkroege/kopia/pulls/496/comments --paginate`. Each fix is a `fixup!` commit pushed normally (allowed); reply in each thread with `gh api -X POST repos/rjkroege/kopia/pulls/496/comments/<id>/replies -f body=...`.
5. **On "squash":** `GIT_SEQUENCE_EDITOR=: git rebase -i --autosquash fa06c01e`, then `git diff --quiet <fixup head> HEAD`. Build the merge branch in `../kopia-sync`:
   ```
   git fetch origin && git switch -C xattr/merge-10 origin/main
   git merge --no-ff -m 'Merge xattr/10-docs (#496) into main' xattr/10-docs
   ```
   Check that `git diff origin/main HEAD` is byte-identical to `git diff fa06c01e <new 10 head>` (via temp files), then build, vet, test, cross-OS lint and checklocks the merged tree (it has the fork code).
6. **User force-pushes *N* and pushes the merge branch** (`git -C ../kopia-sync push -u origin xattr/merge-10`). I confirm both origin heads match local, then open the PR:
   `gh pr create --repo rjkroege/kopia --base main --head xattr/merge-10 --title 'feat(snapshots): merge extended metadata stack 10 (#496)' --body-file <scratchpad file>`.
   Body: model on #517 (`gh pr view 517 --repo rjkroege/kopia --json body -q .body`): what the CL does, port and review notes, the integration-branch paragraph with the reviewed tip sha, "#<N> is closed with a pointer here; #<N+1> stays based on `xattr/<N>-…`", and what follows.
7. **Watch checks** in the background: `gh pr checks <n> --repo rjkroege/kopia --required --watch --interval 60`. User merges (`gh pr merge <n> --repo rjkroege/kopia --merge`). Then the stack PR is closed with a pointer (user, if auto mode blocks it), update this section, and go on to the next CL.

### Local checks (every branch, before pushing)

- `make lint`. It only lints the host OS.
- **Cross-OS lint.** CI runs `make lint-windows` inside Lint (ubuntu), and #514 failed on it. Run:
  ```
  for os in windows linux darwin freebsd openbsd; do
    GOOS=$os GOARCH=amd64 tools/.tools/golangci-lint-*/golangci-lint run <pkgs>
  done
  ```
- **checklocks.** `make lint` doesn't run it; CI's Check Locks step does, and it failed on 05 and 06. Run `make check-locks` (it builds the tool into `tools/.tools/checklocks-*/bin/` and runs `go vet -vettool=…`). `env -u GOBIN` is no longer needed: `tools/tools.mk` pins GOBIN for that rule (PR `fix/checklocks-gobin`).
  - `snapshot.Stats` fields are `+checkatomic`, so tests read them with `atomic.LoadInt32(&...)`.
- `go vet` with GOOS set to darwin, linux, windows, freebsd and openbsd.
- `go test -tags testing ./snapshot/... ./fs/... ./cli/`.

### Rules

- **Auto mode blocks these, so the user runs them:**
  - `git push --force-with-lease`;
  - `git commit --amend` (use `--fixup` instead);
  - `gh pr merge`;
  - `gh pr edit --base`;
  - `gh pr close` (blocked once, 2026-10-27, for #495).
- Always pass `--repo rjkroege/kopia` to gh; otherwise it resolves to kopia/kopia.
- **Never merge or rebase `main` into stack branches.** They stay upstream-clean, with no storj or CI-gate history, and they lack fork-only fixes like #510 and #511. Those fixes reach `main` through the `xattr/merge-NN` branches, which are cut from `origin/main`.
  - `main` is strict, so if it moves before a merge PR lands, rebuild the merge branch from the new `origin/main` (the user force-pushes it).
- **Known flakes:**
  - `TestSnapshotActionsEmbeddedScript` (`text file busy`, arm);
  - `TestServerScheduling` (Windows).
  - `TestNewTimer/exactly_maxSleepTime` (`internal/sleepable`, macOS): timer fired at 39ms vs ~10ms on #525.

  Rerun them with `gh run rerun <id> --repo rjkroege/kopia --failed`. Code Coverage isn't required; the fork reports it via #511.
- Upstream syncs follow the `upstream-sync` project skill.

API changes from the #487 review that later branches must adopt:
- `ExtendedAttributes(ctx, opts)` is a method on `fs.Entry`. The `ExtendedAttributesReader` interface and the cachefs, loggingfs and ignorefs forwarders are gone; the wrappers get the method by embedding.
- `fs.XattrMode` is a byte iota (`XattrsNone` = 0) that marshals as "none", "user" or "all". In #490 the policy field must become `*fs.XattrMode`, so that "unset" is distinct from none for merging. `policy_merge_test` needs a case for it.
- `ExcludeXattrs` holds exact names (`slices.Contains`), with no `*` prefix form.
- `SkippedXattrs`, the `MaxXattrSize` option and `MaxXattrValueSize()` are removed. The constant `fs.MaxXattrSize` is 8 KiB. localfs logs oversized names; #491's upload log of skipped names must go. `ls -@` and diff in #496 must drop their skipped-name handling.
- `Filtered` returns its receiver when it would drop nothing, and its result is read-only: it shares values with the source. Check that restore (#492) never modifies it.
- mockfs `SetExtendedAttributes` takes a value.
- From 09 (#495): `ExtendedAttributes.ACL []byte` (macOS kauth_filesec), `AttributeOptions.ACLs`, `opts.IncludesXattr(name)` (POSIX-ACL xattrs are selected by `ACLs` alone) and `fs.IsPOSIXACLXattr`. `Filtered`'s result shares `ACL` too.

Decided: keep the excludes as a slice. A map would be rebuilt for every entry; a per-`*Policy` cache could be added in the uploader if the lists grow.
