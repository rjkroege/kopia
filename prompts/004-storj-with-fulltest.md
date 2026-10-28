---
id: 004
title: storj-with-fulltest
status: in-progress
priority: medium
type: feature
created: 2026-08-11
updated: 2026-08-12
tags: [storj, ci, testing, upstream, pr-3450]
---

## Goal

Two audiences, one branch:

1. **Upstream maintainer (@jkowalski):** demonstrate that wiring Storj into
   the existing `providers-core.yml` / `providers-extra.yml` CI pattern is a
   trivial, low-risk diff — the test file already matches the shape the
   other backends use, and the missing pieces are small and enumerated
   below, not architectural.
2. **Me (rjkroege), on my own fork:** get a working setup where I can run
   the full `TestStorjStorage`-class provider-validation suite against a
   real Storj bucket, gated the same way GitHub Actions secrets would gate
   it, without needing upstream's repo-secret access.

## Context

- Builds on `001-address-review-comments.md`, specifically **Task 6**: "CI
  wiring [is] a maintainer follow-up... neither `providers-core.yml` nor
  `providers-extra.yml` has a Storj step... requires repo-secret access, so
  this is an ask for @jkowalski post-merge, not something to do from this
  branch." This issue is the substantiation for that ask — instead of just
  flagging the gap, show it filled and validated.
- `003-storj-is-extra.md` already tagged Storj with
  `//go:build !no_extra_providers`, matching b2/gdrive/rclone/sftp/webdav.
  That places Storj naturally alongside the "extra" providers in
  `providers-extra.yml` rather than `providers-core.yml`.
- Compared `repo/blob/storj/storj_storage_test.go` against
  `repo/blob/gcs/gcs_storage_test.go` and `repo/blob/b2/b2_storage_test.go`
  (the two closest analogues — b2 especially, since it's also gated behind
  `no_extra_providers` and env-var-only, no OAuth/JSON-blob credential
  dance like GCS/GDrive). Findings, **excluding anything GitHub-infra
  related**:
  - Storj has exactly one test, `TestStorjStorage`, combining
    `VerifyStorage` + `AssertConnectionInfoRoundTrips` +
    `ValidateProvider`. b2 and GCS both additionally have:
    - A **standalone `TestCleanupOldData`** that runs
      `blobtesting.CleanupOldData` with `blobtesting.MinCleanupAge`
      against the shared bucket, independent of the main test's own
      `Prefix`-scoped cleanup. This is what keeps the real test bucket
      from accumulating orphaned objects across CI runs (e.g. from a
      previous run that panicked before its `defer`/`t.Cleanup` fired).
    - **Negative-path tests.** b2 has `TestB2StorageInvalidBlob` (GetBlob
      on a nonexistent blob), `TestB2StorageInvalidBucket` (New() against
      a bucket name that doesn't exist), and `TestB2StorageInvalidCreds`
      (New() with bad key/keyID). GCS has the bucket-equivalent
      (`TestGCSStorageInvalid`). Storj has none of these at the
      `storj_test` black-box level — `storj_internal_test.go` covers
      invalid-range/not-found at the white-box level via `TestPutGetBlob`,
      but not invalid-bucket or invalid-credentials via the public `New()`
      entry point the way b2/GCS do.
  - Everything else lines up: `t.Parallel()`, `testutil.ProviderTest(t)`
    gating, `testlogging.Context`, `blob.PutOptions{}`, the three-call
    `VerifyStorage`/`AssertConnectionInfoRoundTrips`/`ValidateProvider`
    sequence. No sharding-specific gap — `storj_internal_test.go`'s
    `TestListBlobs` already exercises the `.shards` layout at the
    white-box level, which b2/GCS don't need since they don't implement
    directory sharding the same way.
  - `Makefile`'s `provider-tests` target
    (`$(GO_TEST) ... ./repo/blob/$(PROVIDER_TEST_TARGET)`) is fully generic
    — `PROVIDER_TEST_TARGET=storj` works today with no Makefile changes.
    Unlike rclone (needs the `rclone` binary) or S3-via-minio (needs
    `MINIO_MC_PATH`), Storj needs no extra binary in
    `provider-tests-deps` — it's a pure Go SDK. That's one less thing to
    explain to the maintainer.
  - The job-level gate on both workflow files —
    `if: github.repository == 'kopia/kopia' && !github.event.pull_request.head.repo.fork`
    — means neither workflow runs at all on `origin` (`rjkroege/kopia`) as
    committed today. That gate is correct and should stay untouched in
    whatever gets proposed upstream; it's the thing that has to be
    worked around locally for goal (2), not fixed.
- `internal/testutil.ProviderTest` skips unless `KOPIA_PROVIDER_TEST` is
  set (no value check, just non-empty) — this is the same switch
  `make provider-tests` sets via `export KOPIA_PROVIDER_TEST=true`.

## Task

1. **DONE (on `better-storj`, commit `dc349a2b`).** Close the test-coverage
   gap in `repo/blob/storj/storj_storage_test.go` so it's parity with b2,
   not just GCS-minus-JSON-creds:
   - Add a standalone `TestCleanupOldData` (mirrors b2's, using
     `blobtesting.MinCleanupAge`).
   - Add `TestStorjStorageInvalidBucket` — **implementation note:** the
     originally-planned "New() against a bucket name that doesn't exist"
     doesn't apply to Storj. `storj.New()` → `_new()` calls
     `project.EnsureBucket()`, which is create-if-missing, so a
     well-formed-but-nonexistent bucket name succeeds instead of erroring
     (unlike b2/GCS, which require the bucket to preexist). Verified
     against `storj.io/uplink@v1.14.3`'s `bucket.go`: `EnsureBucket` →
     `CreateBucket`, which does return `ErrBucketNameInvalid` for a
     malformed name (empty string, confirmed via `common.go`'s
     `convertKnownErrors`/`metaclient.ErrNoBucket` mapping) — no local
     client-side validation, it's a real server round-trip. Implemented
     using `BucketName: ""` instead.
   - Add `TestStorjStorageInvalidCreds` (New() with a syntactically-invalid
     access grant string — expect error from `uplink.ParseAccess`, which
     fails locally before any network call).
   - Skipped the blob-level negative test analogous to
     `TestB2StorageInvalidBlob`: confirmed redundant with
     `storj_internal_test.go`'s `TestPutGetBlob` not-found case
     (`GetBlob` on `"no such blob"` already asserts
     `blob.ErrBlobNotFound`).
2. **DONE (on `better-storj`, commit `a1127479`).** Draft the CI diff as a
   new "Storj" step in `.github/workflows/providers-extra.yml`, shaped
   exactly like the `B2` step (env-var secrets, no extra deps,
   `if: success() || failure()`). This is the artifact to hand the
   maintainer — kept minimal and upstream-clean, no fork-specific
   workarounds in it.
3. **DONE (on `better-storj-fulltest`, commit `27ba579d`).** Separately,
   add a fork-only workflow
   (`.github/workflows/storj-fulltest.yml`) that is *not* proposed
   upstream: triggered by `workflow_dispatch` (and `push` to
   `better-storj-fulltest`), without the
   `github.repository == 'kopia/kopia'` gate, running the same
   `make provider-tests PROVIDER_TEST_TARGET=storj` against
   `secrets.STORJ_ACCESS_GRANT` / `secrets.STORJ_TEST_BUCKET` defined on
   `rjkroege/kopia`. This is the harness for goal (2); keeping it in its
   own file means the upstream-facing diff (item 2) stays a clean,
   isolated addition. `better-storj-fulltest` was rebased onto
   `better-storj`'s new tip first (fast-forward, no conflicts — it had no
   commits of its own yet) so items 1/2 sit underneath this as shared
   history rather than being duplicated.
4. Write the **Proposal** and **Runbook** below into this document (done —
   see sections) so the setup is reproducible without re-deriving it.
5. Push `better-storj-fulltest` to `origin`. **Status: paused before this
   step** — stopping after item 3 per instruction, to plan next steps
   before pushing/iterating on the workflow further.
6. (Follow-up, not blocking push): once secrets are configured per the
   runbook, trigger the fork-only workflow and confirm a real green run;
   record the run URL in the Progress log.

## Acceptance criteria

- [x] `repo/blob/storj/storj_storage_test.go` has `TestCleanupOldData`,
  `TestStorjStorageInvalidBucket`, `TestStorjStorageInvalidCreds`, all
  gated by `testutil.ProviderTest` the same way the existing test is.
  (`TestStorjStorageInvalidBlob` omitted — confirmed redundant with
  `storj_internal_test.go`'s existing coverage.)
- [x] `go build ./... ` and `go vet ./repo/blob/storj/...` clean; `gofmt -l`
  clean on touched files; `go test ./repo/blob/storj/...` skips cleanly
  without credentials. (`golangci-lint` not available in this environment
  to double-check — not run.)
- [x] `.github/workflows/providers-extra.yml` has a `Storj` step matching the
  `B2` step's shape (this is the piece meant to eventually go upstream).
- [x] A new fork-only workflow file exists that can run the Storj provider
  tests on `rjkroege/kopia` via manual dispatch, independent of the
  `github.repository == 'kopia/kopia'` gate.
- [x] This document contains a concrete Proposal and Runbook (below).
- [x] `better-storj-fulltest` exists on `origin` with today's work — pushed
  2026-08-12, latest at `976b25f9`; each push auto-triggered the
  fork-only workflow.
- [x] A real, all-green run against the live Storj backend exists:
  https://github.com/rjkroege/kopia/actions/runs/31588038753
  (`DONE 66 tests, 5 skipped in 34.531s`, 0 failures) — the concrete
  evidence goal (2) was after. Two real bugs (`.shards` leaking into
  `ListBlobs`, and four ad hoc tests hardcoded to the wrong bucket) were
  found and fixed along the way (see Progress log); both fixes are also
  cherry-picked onto `better-storj` (`347acb17`, `86b2f342`).

## Proposal

**What to show the maintainer (minimal, upstream-clean diff):**

```yaml
# .github/workflows/providers-extra.yml, alongside the existing B2/GDrive/Rclone/WebDAV steps
- name: Storj
  run: make provider-tests PROVIDER_TEST_TARGET=storj
  env:
    STORJ_ACCESS_GRANT: ${{ secrets.STORJ_ACCESS_GRANT }}
    STORJ_TEST_BUCKET: ${{ secrets.STORJ_TEST_BUCKET }}
  if: ${{ success() || failure() }}
```

That's the whole upstream ask: two secrets
(`STORJ_ACCESS_GRANT`, `STORJ_TEST_BUCKET` — already the exact env var
names `storj_storage_test.go` reads), no new deps in
`provider-tests-deps`, no Makefile changes. Point PR #3450's comment at
this stanza plus the closed test-coverage gap (Task 1) as the evidence
that "adapting existing testing patterns, including provider validation"
(jkowalski's original ask) is done, not just proposed.

**What NOT to send upstream:** the fork-only workflow (Task 3) and its
relaxed `github.repository` gate. That gate exists to stop
`providers-core.yml`/`providers-extra.yml` from trying to run against
secrets that don't exist on random forks/PRs; loosening it belongs only on
`better-storj-fulltest`, as a personal validation harness, not in what
gets proposed to `kopia/kopia`.

**Why a separate file instead of editing the gate in place:** editing
`providers-extra.yml`'s `if:` condition to add `rjkroege/kopia` as a
special case would leave a fork-specific string baked into the same file
being proposed upstream, undermining the "this is a clean two-line diff"
pitch. A second, throwaway workflow file keeps the two concerns — "prove
it to upstream" and "let me actually run it" — physically separate, and
the second file simply doesn't get included when/if the PR is cut down to
just the `providers-extra.yml` change.

## Runbook

Setup to let me run the Storj provider-validation suite against a real
backend from my fork, without upstream's CI secrets.

1. **DONE — Storj account + project.** Signed up at storj.io and created the
   project used for this work.
2. **DONE — Test bucket.** Created `kopia-storj-test` as the dedicated
   bucket for this work — not reused from anything holding real data,
   since `blobtesting.VerifyStorage`/`CleanupOldData` write and delete
   freely.
3. **DONE — Scoped access grant.** Generated an access grant named
   `kopia-storj-test`, restricted to the `kopia-storj-test` bucket, with
   the Read/Write/List/Delete permissions recommended below, plus an
   encryption passphrase.
   - Via the web console: Access → Create Access Grant → restrict to the
     `kopia-storj-test` bucket, permissions Read/Write/List/Delete, no
     other buckets.
   - Or via the `uplink` CLI: `uplink access create` (project-wide), then
     `uplink share --buckets=kopia-storj-test --access=read,write,list,delete
     <full-access-name>` to derive a bucket-scoped grant string. Use the
     *derived, restricted* grant, not the raw project-wide one, so a
     leaked CI secret can't touch anything outside the test bucket.
   - Set an expiration on the grant if the console/CLI offers one, and
     note the renewal date somewhere (e.g. a calendar reminder) since an
     expired grant will start failing the workflow with an auth error, not
     a silent skip.
   > **Redacted:** the resulting access-grant token and the encryption
   > passphrase were pasted directly into this step during editing —
   > removed here since this file, although excluded from git commits per
   > `always.md`, still sits in plaintext on disk. Per step 4/5 below they
   > should live only in a password manager and the GitHub Actions secret
   > store, not in this document.
4. **DONE — Store the grant string safely** (password manager) — it's a
   bearer credential for the `kopia-storj-test` bucket; treat it like any
   other secret, don't paste it into a commit, issue, or PR comment.
5. **DONE — Add GitHub repository secrets** on `rjkroege/kopia`: Settings →
   Secrets and variables → Actions → New repository secret:
   - `STORJ_ACCESS_GRANT` = the derived access grant string from step 3.
   - `STORJ_TEST_BUCKET` = `kopia-storj-test`.
6. **DONE — Confirmed the fork-only workflow is live on GitHub.** Three
   parts, all done:
   1. **Pushed `better-storj-fulltest` to `origin`** (`4a371064..27ba579d`,
      fast-forward). This is Task 5, previously paused pending your
      go-ahead. As anticipated, the push immediately triggered a real run
      via the workflow's own `on: push: branches: [ better-storj-fulltest
      ]` trigger — see step 7 below.
   2. **Confirmed the workflow registers**: `gh workflow list --repo
      rjkroege/kopia` lists "Storj Full Test (fork-only, not for
      upstream)" as `active` (id `332629832`).
   3. **Confirmed the two secrets exist**: `gh secret list --repo
      rjkroege/kopia` shows `STORJ_ACCESS_GRANT` and `STORJ_TEST_BUCKET`
      both present (set 2026-08-11, existence only — GitHub never
      exposes values).
7. **Trigger it.** **Already happened as a side effect of step 6.1** — the
   push-triggered run is
   https://github.com/rjkroege/kopia/actions/runs/31583991018 (event
   `push`, branch `better-storj-fulltest`, started 2026-08-12T09:40:34Z).
   A separate manual `workflow_dispatch` run (via the Actions UI or `gh
   workflow run storj-fulltest.yml --repo rjkroege/kopia --ref
   better-storj-fulltest`) is only needed for a *re-run*, not this first
   one. Still true for any future manual trigger: it spends real Storj
   API/bandwidth against the live bucket, so it needs your go-ahead each
   time, same as the push did.
8. **DONE — Check the run.** Run
   https://github.com/rjkroege/kopia/actions/runs/31588038753 (triggered
   by the `976b25f9` push): **success**, `DONE 66 tests, 5 skipped in
   34.531s`, zero failures. The 5 skips are the `PutBlobsWithSetTime`
   subtests, expected since Storj doesn't support `SetModTime`.
   `TestStorjStorage`, `TestCleanupOldData`,
   `TestStorjStorageInvalidBucket`, `TestStorjStorageInvalidCreds`, and
   the now-fixed `TestConnect`/`TestListBlobs`/`TestPutOpts`/
   `TestPutGetBlob` all ran for real (not skipped) and passed — this is
   the first real, all-green run of the full package against a live
   Storj backend. Checked via `gh run view --json
   status,conclusion`/`--log`, read-only. Did not separately spot-check
   the bucket contents in the Storj console (no credentials in this
   session, per the step 3 redaction) — a clean `DONE ... 0 failures`
   from `TestCleanupOldData` and `TestStorjStorage`'s own deferred
   cleanup is reasonable evidence the bucket isn't accumulating orphans,
   but a manual spot-check is still worth doing at your convenience.
9. **Faster local loop, no GitHub Actions round-trip needed:** from a
   workstation with the same values exported — `STORJ_ACCESS_GRANT=...
   STORJ_TEST_BUCKET=kopia-storj-test KOPIA_PROVIDER_TEST=true go test
   ./repo/blob/storj/...` — runs the identical suite locally. Use this
   for iterating on Task 1's new tests; reserve the Actions trigger
   (step 7) for confirming CI-environment parity before treating it as
   "validated." *Claude can do this* only if those three variables are
   exported into a shell Claude runs commands in (same caveat as step
   8 — not present in this session now); otherwise it's a you-run-it
   step.
10. Once a real green run exists, that's the concrete evidence — link
    the run in the `#3450` PR comment (per
    `001-address-review-comments.md` Task 4) alongside the
    `providers-extra.yml` diff from the Proposal section above. *Claude
    can draft and post this* via `gh pr comment` once authorized, but
    posting to a public upstream PR is visible to others (and to
    jkowalski specifically) — needs your explicit sign-off on the
    drafted text before it goes out, every time, not a standing
    authorization.

## Notes

- Per `prompts/always.md`: this document itself is not committed to git.
- Related: `001-address-review-comments.md` (Task 6, the original CI-gap
  flag), `003-storj-is-extra.md` (the build-tag work that makes Storj fit
  the `providers-extra.yml` shape), upstream PR
  https://github.com/kopia/kopia/pull/3450.
- The fork-only workflow (Task 3) is deliberately scoped to prove out the
  setup, not to become a permanent fixture — once upstream adopts the
  `providers-extra.yml` step and configures its own secrets, this branch's
  personal-validation workflow has served its purpose and can be dropped.

## Progress log

- 2026-08-11: Issue created. Branch `better-storj-fulltest` cut from
  `better-storj`. Compared `storj_storage_test.go` against
  `gcs_storage_test.go` and `b2_storage_test.go`; identified the
  missing-coverage gap (standalone `TestCleanupOldData`, invalid-bucket/
  invalid-creds negative tests) and confirmed `make provider-tests
  PROVIDER_TEST_TARGET=storj` needs no Makefile/deps changes. Wrote
  Proposal (upstream-clean `providers-extra.yml` diff) and Runbook (real
  Storj account/bucket/grant → fork secrets → fork-only workflow →
  local-loop fallback). Tasks 1-3 (test coverage, workflow diffs) not yet
  implemented — this pass only produced the issue document per the
  requested scope. Next: implement Task 1's tests, Task 2's
  `providers-extra.yml` stanza, Task 3's fork-only workflow file, then
  push.
- 2026-08-11: Implemented Tasks 1-3.
  - Switched to `better-storj`. Task 1: added `TestCleanupOldData`,
    `TestStorjStorageInvalidBucket`, `TestStorjStorageInvalidCreds` to
    `storj_storage_test.go`. Discovered while implementing
    `TestStorjStorageInvalidBucket` that the originally-planned
    "nonexistent bucket name" case doesn't error for Storj — `New()` →
    `_new()` → `project.EnsureBucket()` is create-if-missing (confirmed by
    reading `storj.io/uplink@v1.14.3`'s `bucket.go`/`common.go` in the
    local module cache). Switched that test to an empty bucket name,
    which `EnsureBucket`→`CreateBucket` does reject server-side
    (`ErrBucketNameInvalid` via `metaclient.ErrNoBucket`). Confirmed
    `TestB2StorageInvalidBlob`'s equivalent is already redundant with
    `storj_internal_test.go`'s `TestPutGetBlob` and skipped adding it.
    Verified `gofmt -l`, `go vet ./repo/blob/storj/...`,
    `go build ./repo/blob/storj/...`, and `go test ./repo/blob/storj/...
    -v` all clean — new tests skip via `testutil.ProviderTest` exactly
    like the pre-existing ones (no live credentials in this environment).
    `golangci-lint` isn't installed locally, so `make lint` wasn't run.
    Committed as `dc349a2b` ("test(providers): close Storj test-coverage
    gap vs b2/GCS").
  - Task 2: added the `Storj` step to `providers-extra.yml`, matching the
    `B2` step's shape exactly. Sanity-checked indentation/structure by eye
    (no `yq`/`actionlint`/PyYAML available in this environment). Committed
    as `a1127479` ("ci(providers): wire Storj into providers-extra.yml").
  - Switched to `better-storj-fulltest`, rebased it onto `better-storj`'s
    new tip (`git rebase better-storj` — fast-forward, no conflicts, since
    the branch had no commits of its own beyond the original
    `better-storj` history it was cut from).
  - Task 3: added `.github/workflows/storj-fulltest.yml` — `push`
    (branch `better-storj-fulltest`) + `workflow_dispatch` triggers, no
    `github.repository == 'kopia/kopia'` gate, same `Storj` step as
    Task 2. Committed as `27ba579d` ("ci(storj): add fork-only workflow to
    validate against a real backend").
  - **Stopped here per instruction** — Task 5 (push
    `better-storj-fulltest` to `origin`) intentionally not done yet;
    `origin/better-storj-fulltest` still only has last session's single
    commit. Planning next steps with the user before pushing/iterating on
    the workflow.
- 2026-08-11: Merged user's notes (edited directly into this file) into
  the Runbook: Runbook steps 1-5 marked done — Storj account/project,
  `kopia-storj-test` bucket, a scoped access grant (named
  `kopia-storj-test`, Read/Write/List/Delete, restricted to that bucket)
  with its encryption passphrase, both stored in a password manager, and
  the two GitHub repo secrets already set on `rjkroege/kopia`. Redacted
  an access-grant token and the encryption passphrase that had been
  pasted directly into the raw edit — both are sensitive credential
  material and, per the runbook's own step 4/5 guidance, belong only in
  the password manager and the GitHub secret store, not sitting in
  plaintext in a file on disk (even one excluded from git commits).
  Updated the `kopia-ci-test` placeholder bucket name to the real
  `kopia-storj-test` throughout steps 2/3/5/9. Expanded step 6 into its
  three constituent parts (push, confirm workflow registers, confirm
  secrets exist) and added a "can Claude do this" note to steps 6-10
  after checking the environment: `gh` is installed and authenticated as
  `rjkroege` (so listing workflows/secrets, triggering dispatch, watching
  runs, and posting a PR comment are all mechanically possible); `uplink`
  is installed but this session has no Storj credentials in its
  environment; none of `STORJ_ACCESS_GRANT`/`STORJ_TEST_BUCKET`/
  `KOPIA_PROVIDER_TEST` are set here. Push (step 6.1) and dispatch (step
  7) both spend real Storj usage and are visible on the fork, so both
  need explicit go-ahead before Claude runs them — not done in this
  pass. Nothing pushed or triggered; still holding at the same point as
  last entry pending the user's plan for next steps.
- 2026-08-12: Given go-ahead, executed step 6's three phases. Pushed
  `better-storj-fulltest` to `origin` (`4a371064..27ba579d`,
  fast-forward — Task 5, done). As anticipated, the push immediately
  triggered a real run of "Storj Full Test (fork-only, not for
  upstream)" via its own `on: push` trigger:
  https://github.com/rjkroege/kopia/actions/runs/31583991018 (event
  `push`, branch `better-storj-fulltest`, started
  2026-08-12T09:40:34Z) — this also satisfies step 7 (trigger it) as a
  side effect; no separate `workflow_dispatch` was needed or run.
  Confirmed via `gh workflow list --repo rjkroege/kopia` that the
  workflow registered (`active`, id `332629832`). Confirmed via `gh
  secret list --repo rjkroege/kopia` that both `STORJ_ACCESS_GRANT` and
  `STORJ_TEST_BUCKET` exist (existence only, not values). Updated Task
  5/Runbook steps 6-7/Acceptance criteria above to reflect all of this.
  Run `31583991018` was still `in_progress` as of this entry — checking
  its outcome (Runbook step 8) not yet done; that and the bucket
  spot-check are the natural next step.
- 2026-08-12: Run `31583991018` completed: **failure**. Checked via `gh
  run view --log-failed`. Root cause, two layers:
  1. `TestConnect` (in `storj_internal_test.go`, not touched by today's
     Task 1 work) failed with `could not parse access grant: uplink:
     invalid access grant format` — the `STORJ_ACCESS_GRANT` secret
     currently stored on `rjkroege/kopia` doesn't parse as a valid
     `uplink` access grant. Likely culprit: how the value was
     transcribed into the GitHub secret (truncation, stray
     whitespace/newline, or the wrong string entirely) — worth
     re-checking against whatever's in the password manager (Runbook
     step 4) and re-running `gh secret set STORJ_ACCESS_GRANT` if it
     doesn't match exactly.
  2. That parse failure exposed a **preexisting bug** in `TestConnect`
     (predates this issue, not introduced by Task 1): it checks the
     `New()` error with `t.Errorf` (which logs and continues) instead of
     `t.Fatalf`, then unconditionally calls `removeTheBucket(t, ctx, rs,
     testbucket)` with `rs == nil`. That nil-pointer-dereferences inside
     `removeTheBucket`'s `rsj.ListBlobs` call, panics, and — because
     `storj_internal_test.go` (package `storj`) and
     `storj_storage_test.go` (package `storj_test`) share one compiled
     test binary for the directory — the unrecovered panic kills that
     whole binary. Effect: **none of Task 1's new tests
     (`TestStorjStorage`, `TestCleanupOldData`,
     `TestStorjStorageInvalidBucket`, `TestStorjStorageInvalidCreds`) got
     a chance to run or report** — the run log shows exactly one test,
     `TestConnect`, before the crash. So this run doesn't yet validate
     Task 1 either way; it only proved the credential is bad and
     surfaced the `TestConnect` robustness bug.
  Not yet fixed or acted on — reporting to the user for direction before
  touching either the secret or `storj_internal_test.go`.
- 2026-08-12: Fixed item 2 (the `TestConnect` robustness bug) as a
  standalone patch: `t.Errorf` → `t.Fatalf` in `TestConnect`
  (`repo/blob/storj/storj_internal_test.go`), so a `New()` failure stops
  the test immediately instead of falling through to
  `removeTheBucket(t, ctx, nil, testbucket)` and panicking. One-line
  diff, no other tests in the file touched (`TestListBlobs`/
  `TestPutOpts`/`TestPutGetBlob` already used `t.Fatalf` correctly — only
  `TestConnect` had the bug). Verified `gofmt -l`, `go vet
  ./repo/blob/storj/...`, `go build ./repo/blob/storj/...`, and
  `go test ./repo/blob/storj/... -run TestConnect -v` (skips cleanly, no
  credentials here). Committed on `better-storj-fulltest` as `515f5620`
  ("fix(storj): stop TestConnect nil-dereferencing on New() failure"),
  then cherry-picked onto `better-storj` as `86b2f342` — clean, no
  conflicts. Re-verified build/vet clean on `better-storj` after the
  pick, then returned to `better-storj-fulltest`.
  Item 1 (the bad `STORJ_ACCESS_GRANT` secret) is still open — this fix
  only stops a bad credential from crashing the whole test binary; it
  doesn't make the credential valid. `better-storj-fulltest` is now 1
  commit ahead of `origin/better-storj-fulltest` (not pushed yet — this
  fix, and any re-run to confirm it works, both need a fresh go-ahead
  first, same as before).
- 2026-08-12: User updated the `STORJ_ACCESS_GRANT` secret. Pushed
  `better-storj-fulltest` again (`27ba579d..515f5620`), which
  auto-triggered a fresh run:
  https://github.com/rjkroege/kopia/actions/runs/31585704878. Watched it
  via `gh run watch` to completion: **failure**, but a real one this
  time — the `TestConnect` fix worked, all 66 tests ran (up from 1
  before), no panic. Checked via `gh run view --log`/`--log-failed` and
  found two distinct, unrelated real bugs, both surfaced for the first
  time by this being the first run ever against a live Storj backend:
  1. `TestConnect`/`TestListBlobs`/`TestPutOpts`/`TestPutGetBlob` (the
     ad hoc tests in `storj_internal_test.go`) failed with
     `uplink: permission denied ... Unauthorized API credentials`.
     Root cause: they targeted a hardcoded bucket name (`testbucket =
     "kopia-test"`) instead of `STORJ_TEST_BUCKET` — the new grant,
     correctly scoped to `kopia-storj-test` only, correctly can't touch
     a different bucket. Meant `make provider-tests
     PROVIDER_TEST_TARGET=storj` (the whole-package `go test` the CI
     step runs) would always fail on these four regardless of grant
     quality.
  2. `TestStorjStorage`'s `ListBlobs`/`ListBlobs/ListNoPrefix`/
     `DeleteBlobsAndList` subtests failed: `ListBlobs()` returned an
     extra `.shards` entry the generic `blobtesting` suite doesn't
     expect. Root cause, confirmed by reading
     `repo/blob/storj/storj_storage.go`: `impl.GetBlobIDFromFileName`
     was an identity passthrough (`return blob.ID(name), true` for any
     name), unlike `repo/blob/filesystem`'s use of
     `sharded.ImplHelper`, which only recognizes names with the `.f`
     suffix as blobs. Storj has no such suffix convention, so
     `sharded.Storage`'s own bookkeeping object
     (`sharded.ParametersFile`, i.e. `.shards`, written at every sharded
     backend's bucket root) was indistinguishable from a real blob and
     leaked into every listing. Also noticed
     `storj_internal_test.go`'s own `TestListBlobs` explicitly asserted
     `.shards` *should* appear — it had encoded the bug as expected
     behavior.
  Reported both to the user with root causes; asked two questions (fix
  the `.shards` leak now vs. log-only; gate the ad hoc tests on
  `STORJ_TEST_BUCKET` vs. leave as-is). User chose: fix the leak now,
  and gate the ad hoc tests (recommended option).
- 2026-08-12: Implemented both fixes, on `better-storj-fulltest`:
  - `repo/blob/storj/storj_storage.go`: `GetBlobIDFromFileName` now
    excludes `sharded.ParametersFile` by name instead of accepting
    every name. Did *not* adopt `sharded.ImplHelper`'s suffix-based
    (`.f`) scheme wholesale — that would change the on-wire filename
    for every real blob storj writes, which would break reads of
    existing production Storj repos (this backend already has real
    data behind it, e.g. the 6TB repo mentioned in
    `001-address-review-comments.md`). The name-exclusion fix is
    surgical and preserves the existing naming scheme.
  - `repo/blob/storj/storj_internal_test.go`'s `TestListBlobs` updated
    to stop expecting `.shards` in results (it was asserting the bug).
  - `TestConnect`/`TestListBlobs`/`TestPutOpts`/`TestPutGetBlob` now
    read `STORJ_TEST_BUCKET` via a new `getStorjTestBucket()` helper
    instead of the hardcoded `testbucket` constant (removed).
  - Since `STORJ_TEST_BUCKET` is shared/persistent, unlike the old
    hardcoded bucket, dropped the bucket-*deletion* step these tests
    used to do at the end (`removeTheBucket` → deleting the whole
    bucket isn't viable for a shared bucket, and a bucket-scoped grant
    generally can't do it anyway). Replaced with
    `deleteAllTestBlobs`/`cleanupTestBlobs`, which delete only the
    blobs a test wrote, not the bucket. `TestListBlobs` additionally
    clears the bucket up front (its assertions are exact-match and
    need to start from empty) — implemented as a separate
    `deleteAllTestBlobs` call rather than reusing `cleanupTestBlobs`
    directly, since the latter also closes the connection, which would
    break the rest of the test if called before the test body runs.
    Removed the now-dead `(*storjStorage).deleteBucket` method.
  - Verified `gofmt -l`, `go vet ./repo/blob/storj/...`, `go build
    ./repo/blob/storj/...` and `./...`, and `go test
    ./repo/blob/storj/... -v` (all skip cleanly, no credentials here).
    Committed on `better-storj-fulltest` as `976b25f9` ("fix(storj):
    stop leaking .shards into ListBlobs, share one test bucket"), then
    cherry-picked onto `better-storj` as `347acb17` — clean, no
    conflicts. Re-verified build/vet clean on `better-storj`, then
    returned to `better-storj-fulltest`.
  Neither fix has been validated against the real backend yet —
  `better-storj-fulltest` is 1 commit ahead of `origin` again, not
  pushed. Next natural step is another push + watch cycle (Runbook
  steps 6-8), pending go-ahead.
- 2026-08-12: Given go-ahead, pushed `better-storj-fulltest` again
  (`515f5620..976b25f9`), which auto-triggered
  https://github.com/rjkroege/kopia/actions/runs/31588038753. Watched it
  via `gh run watch` to completion: **success** —
  `DONE 66 tests, 5 skipped in 34.531s`, 0 failures. Confirmed via `gh
  run view --json status,conclusion`. This is the first fully-green run
  of the Storj package against a live backend: both bugs found in the
  previous run (`.shards` leaking into `ListBlobs`, and the four ad hoc
  tests hardcoded to the wrong bucket) are confirmed fixed, not just
  built/vetted locally. Updated Runbook step 8 and Acceptance criteria
  above accordingly.
  Goal (2) — "let me run tests against a real Storj backend as if
  GitHub secrets were configured" — is substantively achieved: a real
  green CI run exists, on this fork, gated the same way
  `providers-extra.yml`'s proposed `Storj` step would be upstream.
  Remaining open items: Runbook step 10 (post the run + the
  `providers-extra.yml` diff as a comment on upstream `#3450`, per
  `001-address-review-comments.md` Task 4) — not done, needs explicit
  sign-off per the Runbook's own note on that step; and reconciling
  whether the two bug fixes found here (`storj_storage.go`,
  `storj_internal_test.go`) should be called out specifically in that
  PR comment, since they're genuine correctness fixes beyond the
  original test-coverage-gap ask.
