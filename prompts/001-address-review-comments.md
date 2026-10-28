---
id: 001
title: Address review comments on upstream PR #3450 (Storj blob storage)
status: in-progress
priority: medium
type: feature
created: 2026-08-10
updated: 2026-08-10
tags: [storj, upstream, pr-3450, testing]
---

## Goal

Get upstream PR [kopia/kopia#3450](https://github.com/kopia/kopia/pull/3450)
("feat(repository): Add a blob implementation for Storj") merged by
addressing the maintainer's outstanding review feedback — chiefly, adding
provider-validation test coverage that matches the pattern used by the
other blob backends.

## Context

- PR #3450 was opened 2023-11-17 and is still `OPEN` as of 2026-08-10. It
  adds a native Storj blob storage backend, helping implement
  [#3426](https://github.com/kopia/kopia/issues/3426) (the S3-compatible
  gateway caps `ListBlobs` at 1000 objects with prefix matching, which is
  inadequate for large repos).
- There are no formal GitHub reviews (no approve/request-changes) on the
  PR — only issue comments. The sole substantive reviewer feedback is from
  maintainer **@jkowalski** (2023-11-18):
  > have you tried adapting existing testing patterns, including provider
  > validation?
- Community contributor **@ptandler** independently forked the branch,
  added tests (`storj-repo-support` branch), and opened a parallel PR
  [#5319](https://github.com/kopia/kopia/pull/5319) ("...with tests") that
  is still open. Its `repo/blob/storj/storj_storage_test.go` is exactly
  the missing piece: a `TestStorjStorage` that gates on
  `STORJ_ACCESS_GRANT`/`STORJ_TEST_BUCKET` env vars via
  `testutil.ProviderTest`, then runs `blobtesting.VerifyStorage`,
  `blobtesting.AssertConnectionInfoRoundTrips`, and
  `providervalidation.ValidateProvider` — the same shape used in
  `repo/blob/b2/b2_storage_test.go` and
  `repo/blob/rclone/rclone_storage_test.go`.
- My last comment on #3450 (2026-07-19): "I've let this languish. I'll add
  some pieces that I've been running with. And then I'll address
  @jkowalski's suggest with better tests." The current `better-storj`
  branch has since picked this back up with three commits: sharding
  support, CLI support, and a fix for an ephemeral-port-exhaustion flake
  under concurrent reads against a 6TB repo.
- Locally, `repo/blob/storj/` currently has only `storj_internal_test.go`
  (ad hoc tests like `TestConnect` against a real bucket) — it lacks a
  `storj_storage_test.go` equivalent, so it does not yet exercise
  `providervalidation.ValidateProvider` the way the other backends do.
- PR #5319 is a ready-to-borrow reference implementation of the missing
  test file, so this doesn't need to be written from scratch.

## Task

1. Port/adapt the `storj_storage_test.go` pattern from PR #5319 (mirrored
   on b2/rclone) into `repo/blob/storj/`, gated on
   `STORJ_ACCESS_GRANT`/`STORJ_TEST_BUCKET` via `testutil.ProviderTest`.
2. Ensure the new test exercises `providervalidation.ValidateProvider`,
   `blobtesting.VerifyStorage`, and
   `blobtesting.AssertConnectionInfoRoundTrips`, with cleanup via
   `blobtesting.CleanupOldData`.
3. Reconcile with PR #5319 before pushing — it covers the same gap.
   Options: (a) close #3450 in favor of #5319, (b) pull ptandler's test
   file into #3450 with attribution and keep #3450 as the vehicle (it
   also carries sharding/CLI/flake-fix work #5319 doesn't have), or
   (c) comment on both PRs to coordinate before landing either.
4. Post a comment on #3450 addressing jkowalski's ask directly, pointing
   at the new test.
5. Push the updated branch and re-request review.
6. Flag CI wiring as a maintainer follow-up in the PR comment: neither
   `.github/workflows/providers-core.yml` nor `providers-extra.yml` has a
   Storj step. Every other backend only gets exercised against a live
   account via a scheduled/`master`-push workflow that injects
   `STORJ_ACCESS_GRANT`/`STORJ_TEST_BUCKET`-equivalent secrets
   (`KOPIA_PROVIDER_TEST=true` + per-provider creds, see `b2`/`gcs`
   entries in `providers-extra.yml`/`providers-core.yml`). Adding that
   step requires repo-secret access, so this is an ask for @jkowalski
   post-merge, not something to do from this branch.

## Acceptance criteria

- `repo/blob/storj/` has a storage test providing the same coverage class
  as b2/rclone: live provider validation, env-var gated, parallel-safe,
  cleans up after itself.
- `go test ./repo/blob/storj/...` passes with credentials set, and skips
  cleanly without them.
- A comment addressing jkowalski's specific ask is posted on
  kopia/kopia#3450.
- The overlap with #5319 is explicitly resolved (not left as two silent
  duplicate PRs).

## Progress log

- 2026-08-10: Ported `repo/blob/storj/storj_storage_test.go` from PR #5319
  (@ptandler). Adapted one incompatibility: this tree's `testlogging`
  package lacks `ContextForCleanup` (present in ptandler's/upstream's
  newer tree), so cleanup uses the same plain `defer st.Close(ctx)` /
  `defer blobtesting.CleanupOldData(ctx, t, st, 0)` pattern already proven
  in `repo/blob/b2/b2_storage_test.go`. Verified: `go build ./repo/...`,
  `go vet ./repo/blob/storj/...`, `gofmt -l`, and `go test
  ./repo/blob/storj/...` all clean — the new `TestStorjStorage` skips via
  `testutil.ProviderTest`/env-var gating exactly like the pre-existing
  tests (no live Storj credentials in this environment). Committed as
  `98b20f9d` ("test(providers): add live provider-validation test for
  Storj"), crediting @ptandler via `Co-authored-by`. Still open:
  reconciliation-with-#5319 decision (Task item 3) and CI-wiring follow-up
  (Task item 6).

## Notes

- No formal "review" object exists on #3450 — "reviewer's concerns" here
  means jkowalski's one comment, plus the gap ptandler's fork/PR made
  concrete.
- Related: PR https://github.com/kopia/kopia/pull/3450 (this work), PR
  https://github.com/kopia/kopia/pull/5319 (ptandler's parallel PR with
  the missing test pattern), issue
  https://github.com/kopia/kopia/issues/3426 (motivating bug).
- Reference copy of ptandler's test file pulled for comparison:
  `/private/tmp/claude-501/-Users-rjkroege-tools--builds-kopia/274d6cf1-b952-408c-beff-f11505865b3d/scratchpad/5319_storage_test.go`
  (scratch only, not committed).
