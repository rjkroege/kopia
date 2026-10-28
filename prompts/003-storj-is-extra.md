---
id: 003
title: storj is extra
status: done
priority: medium
type: refactor
created: 2026-08-11
updated: 2026-08-11
tags: [storj, build-tags, providers]
---

## Goal

Tag the Storj blob-storage backend with the same `//go:build !no_extra_providers`
convention already used by the other optional CLI backends, so Storj can be
excluded from minimal builds like b2, gdrive, rclone, sftp, and webdav.

## Context

- `v0.23.1` (upstream commit `8b134193`, "refactor(providers): no_extra_providers
  build tag (#5375)") introduced a `//go:build !no_extra_providers` convention
  for optional CLI backends: b2, gdrive, rclone, sftp, webdav. Each provider's
  CLI wiring file (`cli/storage_X.go`), its storage package files, and its
  test files all carry the tag.
- Storj isn't tagged that way but should be — it was flagged as a follow-up
  in `002-rebase-to-head.md`'s Progress log when the `v0.23.1` rebase stage
  landed, but deliberately not acted on there since it's a design choice, not
  a conflict-resolution necessity.
- Storj is self-registering: `cli/storage_storj.go`'s `init()` calls
  `mustRegisterStorageProvider("storj", ...)`. No other file references the
  Storj types by name, so adding the build tag only requires touching the
  Storj files themselves.

## Task

1. Add `//go:build !no_extra_providers` (matching the exact placement used by
   b2/gdrive/rclone/sftp/webdav — first line, blank line, then any package
   doc comment, then `package ...`) to:
   - `cli/storage_storj.go`
   - `repo/blob/storj/storj_options.go`
   - `repo/blob/storj/storj_storage.go`
   - `repo/blob/storj/storj_storage_test.go`
   - `repo/blob/storj/storj_internal_test.go`
2. Verify `go build ./cli/... ./repo/blob/storj/...` succeeds both with and
   without `-tags no_extra_providers`, and that the tagged build excludes the
   `storj` package entirely.
3. Commit the change on `better-storj`.
4. Create a new branch `localisms` (from `better-storj`, after the commit
   above) where Storj is always enabled — i.e. reverts/omits the build-tag
   restriction on that branch so local builds always include Storj regardless
   of `no_extra_providers`.
5. Cherry-pick `1a6d50ef04669219cf06a6219b8f3aadd071cfd3` ("Add support for
   JSON listing" — modifies `cli/command_ls.go` to emit JSON) into
   `localisms`.
6. Return to `better-storj`.

## Acceptance criteria

- All five Storj files carry the `//go:build !no_extra_providers` tag,
  formatted identically to the existing b2/gdrive/rclone/sftp/webdav files.
- `go build ./...` succeeds; `go build -tags no_extra_providers ./...`
  succeeds and excludes `repo/blob/storj`.
- `gofmt -l` and `go vet` clean on the touched files.
- The build-tag change is committed on `better-storj`.
- `localisms` branch exists, built on top of the above commit, with Storj
  always enabled (not gated behind `no_extra_providers`).
- Commit `1a6d50ef04669219cf06a6219b8f3aadd071cfd3` is cherry-picked onto
  `localisms`.
- Working tree ends back on `better-storj`.

## Notes

- Per `prompts/always.md`: this document itself is not committed to git.
- Related: `002-rebase-to-head.md`, where this was originally flagged.

## Progress log

- 2026-08-11: Issue created. Added `//go:build !no_extra_providers` to all
  five Storj files (`cli/storage_storj.go`,
  `repo/blob/storj/storj_options.go`, `repo/blob/storj/storj_storage.go`,
  `repo/blob/storj/storj_storage_test.go`,
  `repo/blob/storj/storj_internal_test.go`), matching the b2 pattern exactly
  (tag, blank line, then package doc comment where present, then `package`).
  Verified `go build ./cli/... ./repo/blob/storj/...` clean both with and
  without `-tags no_extra_providers`; with the tag, `./repo/blob/storj/...`
  matches no packages (correctly excluded). `go vet` clean, `gofmt -l` clean.
  Committed on `better-storj` as `4a371064` ("refactor(providers): tag Storj
  as an extra provider").
- 2026-08-11: Created `localisms` branch from `4a371064`. Made Storj always
  enabled there via `git revert --no-edit 4a371064` (commit `acecd677`),
  which is the cleanest way to un-gate Storj while keeping the gating commit
  itself intact in history for reference. Cherry-picked
  `1a6d50ef04669219cf06a6219b8f3aadd071cfd3` ("Add support for JSON
  listing") onto `localisms` (new commit `b045b117`); hit one conflict in
  `cli/command_ls.go`'s `commandList` struct (upstream had since added
  `humanReadable`; the incoming commit added `emitJson`/`notfirst`) —
  resolved by keeping both sets of fields. `go build ./...` and `go vet
  ./cli/...` clean on `localisms` after the cherry-pick.
- 2026-08-11: Returned to `better-storj` (`git checkout better-storj`);
  confirmed `cli/command_ls.go` and the Storj files are back to their
  pre-`localisms` state. **All acceptance criteria met. Issue closed.**
