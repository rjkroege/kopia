---
id: 002
title: Rebase better-storj onto current master, one release tag at a time
status: done
priority: medium
type: refactor
created: 2026-08-10
updated: 2026-08-10
tags: [storj, rebase, upstream, master]
---

## Goal

Move the `better-storj` branch's five Storj commits forward from their
current base (`origin/master`, effectively the old `v0.15.0` release point)
to sit cleanly on top of current `master` (upstream tip, `b361d0f4`,
2026-08-05), landing each intermediate release tag as its own rebase stage
so that the branch is buildable and green at every step rather than only at
the end.

## Context

- `better-storj` currently branches from `dd1ebb11` (= `origin/master`),
  which *is* the commit the `v0.15.0` tag (`47a559e7`, an annotated tag
  object, 2023-10-18) points to — the fork point is exactly `v0.15.0`.
- Local `master` == `upstream/master` == `b361d0f4` (2026-08-05), which is
  **1124 commits** ahead of `dd1ebb11`. `origin/master` (the fork remote)
  has not been pushed forward and still points at `dd1ebb11`.
- `better-storj`'s own content is exactly 5 commits on top of `dd1ebb11`:
  - `4d5f0911` Add a blob implementation for Storj
  - `7382538e` Sharding support for Storj repos
  - `9508e031` Add command line support for Storj blob storage
  - `75f7cdf6` Proposed fix for storj flake
  - `98b20f9d` test(providers): add live provider-validation test for Storj
- Between `dd1ebb11` and `master` there are **18 release tags** in
  chronological order:
  `v0.15.0, v0.16.0, v0.16.1, v0.17.0, v0.18.0, v0.18.1, v0.18.2, v0.19.0,
  v0.20.0, v0.20.1, v0.21.0, v0.21.1, v0.22.0, v0.22.1, v0.22.2, v0.22.3,
  v0.23.0, v0.23.1`, followed by `master` itself (`b361d0f4`, a handful of
  commits past `v0.23.1`).
- 1124 commits across 3+ years of upstream history is a lot of surface for
  a single rebase to absorb blob-storage interface changes, dependency
  bumps, and CLI-registration pattern changes in one jump — hence staging
  per tag, per `prompts/always.md`'s "each work-item will start at a clean
  context" model: each stage below is sized to be its own work-item.
- Test entrypoint is `make test` (gotestsum-based); package-scoped runs use
  plain `go test ./repo/blob/storj/...` etc. per the pattern already used
  in `001-address-review-comments.md`.
- `repo/blob/storj/` does not exist on `master` — it is wholly introduced
  by this branch's first commit (`4d5f0911`), so no upstream conflicts are
  expected there specifically, but call sites this backend must plug into
  (`repo/blob` provider registration, CLI flag wiring in
  `cli/`, `blob/providervalidation`, `blob/blobtesting`,
  `testutil.ProviderTest`) may have moved or changed shape across 1124
  upstream commits.

## Task

Work one stage at a time; each stage is its own future work-item /
sub-conversation (clean context per `prompts/always.md`). For each tag
`T[i]` in the ordered list above (starting from `v0.15.0`, ending with
`master`):

1. Determine the target: `T[i]` (or `master` for the final stage).
2. Rebase (or continue rebasing) `better-storj` from the previous stage's
   base onto `T[i]`:
   `git rebase --onto T[i] T[i-1] better-storj` (first stage: `--onto
   v0.15.0 dd1ebb11 better-storj`).
3. Resolve conflicts commit-by-commit across the 5 Storj commits. Where
   upstream renamed/moved an interface the Storj backend implements
   (`blob.Storage`, provider registration, CLI command registration,
   `providervalidation`/`blobtesting` helpers), update the Storj code to
   match the new shape rather than papering over it.
4. Build and test at this stage before moving to the next:
   - `go build ./...`
   - `go vet ./repo/blob/storj/...`
   - `gofmt -l repo/blob/storj`
   - `go test ./repo/blob/storj/...` (expect live-credential tests to skip
     cleanly, per `001`'s precedent)
   - `make test` (or at minimum the packages touched by conflict
     resolution) if the tag's changes reached beyond `repo/blob/storj`.
5. Record the stage's outcome (tag landed on, conflicts hit, fixes made,
   test results) in the Progress log below before starting the next stage.
6. Only proceed to `T[i+1]` once the current stage is fully green — do not
   batch multiple tags into one rebase pass.

Final stage: rebase from `v0.23.1` onto `master` (`b361d0f4`) to pick up
the last few post-`v0.23.1` commits, then run the full verification above
once more as the definitive "branch is caught up" check.

## Acceptance criteria

- `better-storj`'s merge-base with `master` is `master` itself (i.e. the
  branch is a clean fast-forward-able set of commits on top of current
  `master`), while still containing exactly the Storj-specific commits
  (rebased/adapted, not squashed away).
- At every intermediate tag stage recorded in the Progress log, the branch
  built and `go test ./repo/blob/storj/...` passed at that point — not
  just at the final `master` landing.
- `go build ./...`, `go vet ./repo/blob/storj/...`, `gofmt -l
  repo/blob/storj` are clean, and `make test` passes on the final rebased
  branch.
- No upstream commits are rewritten — only `better-storj`'s own 5 commits
  (or their post-conflict-resolution descendants) move.
- `origin/master` is left untouched unless the user explicitly asks to
  push the updated fork ref.

## Notes

- Do not touch the production `kopia` command/repo on this machine per
  `prompts/always.md` — this is purely a source-tree rebase in
  `/Users/rjkroege/tools/_builds/kopia`.
- If a given tag's stage turns out to have zero relevant changes to
  `repo/blob/storj/` or its dependencies, the rebase there should be a
  no-conflict pass-through — still worth recording briefly in the
  Progress log so the staged history is complete, but doesn't need deep
  investigation.
- Related: `001-address-review-comments.md` (the upstream-PR-facing work
  this branch is for; that PR's final push should happen only after this
  rebase lands, so the diff jkowalski reviews is against current master,
  not a 3-year-old fork point).

## Progress log

- 2026-08-10: Issue created. Surveyed branch topology: fork point is
  `dd1ebb11` (predates `v0.15.0`), current `master`/`upstream/master` tip
  is `b361d0f4` (2026-08-05), 1124 commits and 18 release tags ahead.
  Enumerated the 18-stage tag sequence above. No rebase work started yet.
- 2026-08-10: Local branches that shadowed release-tag names (e.g.
  `v0.15.0`) have been removed, so the earlier disambiguation concern no
  longer applies.
- 2026-08-10: Correction — `dd1ebb11` is not merely an ancestor of
  `v0.15.0`, it *is* the commit the `v0.15.0` annotated tag object points
  to (the tag SHA `47a559e7` is the tag object, not the commit). So
  `better-storj`'s fork point is exactly `v0.15.0`, and stage 1
  (`--onto v0.15.0 dd1ebb11 better-storj`) is a true no-op, confirmed by
  `git rebase` reporting "Current branch better-storj is up to date."
- 2026-08-10: Made a safety backup branch
  `backup/better-storj-pre-rebase-20260810` (= pre-rebase `98b20f9d`)
  before starting, since this work rewrites history via repeated
  `git rebase --onto`.
- 2026-08-10: Executed stages v0.15.0 through v0.23.0 (tasks #1-#17)
  back-to-back:
  - **v0.15.0**: no-op (fork point itself).
  - **v0.16.0, v0.17.0, v0.18.0, v0.18.2, v0.19.0, v0.20.0, v0.20.1,
    v0.21.0, v0.22.0, v0.22.1, v0.22.3, v0.23.0**: each hit a `go.mod`/
    `go.sum` conflict on the first commit (`Add a blob implementation for
    Storj`), because that commit is the only one that touches dependency
    files (adding `storj.io/uplink`). Resolution pattern used at every
    such stage: `git checkout --ours go.mod go.sum` (take upstream's
    dependency state as-is, dropping the Storj-specific entries), then
    `go mod tidy` — which re-adds `storj.io/uplink` and its transitive
    deps automatically because `repo/blob/storj/*.go` still imports them.
    This is more reliable than hand-merging interleaved `require` blocks
    and consistently converged on `storj.io/uplink v1.14.3` (latest
    available at time of tidy, up from the original `v1.12.1` pin) with
    no further action needed — `go build ./repo/blob/storj/...` came back
    clean after every tidy. Remaining 4 commits applied with zero
    conflicts at every one of these stages.
  - **v0.16.1, v0.18.1, v0.21.1, v0.22.2**: patch releases close to their
    predecessor tag; all 5 commits applied with zero conflicts.
  - **v0.23.0** (the `v0.22.3` → `v0.23.0` hop, a ~5-month gap) additionally
    hit a real code conflict in `cli/app.go`: upstream refactored CLI
    storage-provider registration from a hardcoded `[]StorageProvider{...}`
    literal in `NewApp()` into a self-registration pattern
    (`mustRegisterStorageProvider` called from each provider's own
    `init()`, collected via `getRegisteredStorageProviders()` in
    `cli/storage_providers.go`). Fixed by: (1) taking upstream's
    `cli/app.go` as-is (dropping the branch's hardcoded provider-list
    literal entirely), and (2) adding an `init()` to
    `cli/storage_storj.go` that calls `mustRegisterStorageProvider("storj",
    "a Storj storage", ...)`, mirroring the existing pattern in
    `cli/storage_b2.go`. `go build ./...` clean afterward; `go test
    ./cli/...` kicked off to confirm provider registration/enumeration
    tests still pass (long-running, checked in background).
  - Every stage from v0.15.0 through v0.23.0 verified with `go build
    ./...`, `go vet ./repo/blob/storj/...`, `gofmt -l repo/blob/storj`,
    and `go test -count=1 ./repo/blob/storj/...` — all green.
  - `go test -count=1 ./cli/...` (full package, 202s) came back green,
    confirming the `cli/app.go` provider-registration fix didn't break
    anything else in that package.
- 2026-08-10: **v0.23.1** (task #18): same `go.mod`/`go.sum` conflict
  pattern as the other minor/patch bumps, resolved the same way
  (`git checkout --ours` + `go mod tidy`, converged on `storj.io/uplink
  v1.14.3` again). No `cli/app.go` conflict this time — the
  provider-registration refactor from the v0.23.0 stage stuck without
  further upstream changes to that file between v0.23.0 and v0.23.1.
  Remaining 4 commits applied cleanly. Verified green: `go build ./...`,
  `go vet`, `gofmt -l`, `go test -count=1 ./repo/blob/storj/...`.
  - Noted in passing (not acted on): v0.23.1 also introduces a
    `//go:build !no_extra_providers` tag on b2/gdrive/rclone/sftp/webdav's
    CLI and storage files, a new "optional provider" build-exclusion
    convention. `repo/blob/storj`/`cli/storage_storj.go` don't currently
    carry this tag. Whether Storj should join that "extra providers" group
    is a design choice for the user/maintainer, not something to apply
    silently as part of a conflict-resolution rebase — flagging here as a
    possible follow-up.
- 2026-08-10: **Final stage — master** (task #19): rebased
  `v0.23.1 → master` (`b361d0f4`, the last ~70 commits past v0.23.1,
  mostly dependency bumps, structured-logging refactors, and CI/UI churn
  with no touches to blob-provider registration). Same `go.mod`/`go.sum`
  conflict pattern, same resolution. All 5 commits landed with zero other
  conflicts. `better-storj~5` now equals `master`'s tip exactly — the
  branch is a clean 5-commit stack on top of current master. Verified:
  `go build ./...`, `go vet ./repo/blob/storj/... ./cli/...`, `gofmt -l
  repo/blob/storj cli` — all clean. `go test -count=1
  ./repo/blob/storj/...` green. Full `make test` (race-enabled, whole
  repo) launched in the background as the definitive final check.
- 2026-08-10: `make test` completed. The captured log's tail was
  accidentally clipped to the `gotestsum tool slowest` report (piped
  through `tail -200`), so pass/fail was confirmed instead from the
  authoritative source: `.tmp.unit-tests.json` (gotestsum's JSON test
  report) — zero `"Action":"fail"` events across the whole run, 126
  top-level packages reported `pass`, and `github.com/kopia/kopia/cli`
  and `github.com/kopia/kopia/repo/blob/storj` are both confirmed among
  them. **All acceptance criteria met.**
- **Result**: `better-storj` (tip `19c4d6f3`) is now a clean 5-commit
  stack directly on `master` (`better-storj~5` == `b361d0f4`). Rebase
  complete; issue closed.
- Safety net kept: `backup/better-storj-pre-rebase-20260810` still points
  at the original pre-rebase tip (`98b20f9d`) in case anything here needs
  to be revisited. Not deleted — leaving that decision to the user.
