---
id: 012
title: snapshot migrate --skip, checkpoint resume, and CDN read retries
status: done
priority: medium
type: feature
created: 2026-10-02
updated: 2026-10-16
tags: [cli, migrate, cdnread]
---

## Goal

Make long `kopia snapshot migrate` runs into the B2/Cloudflare `cdnread`
repository survivable:

- Skip known-bad source snapshots.
- Resume from checkpoints after an interruption.
- Retry CDN throttling instead of failing.

## Context

- This supports the Storj → B2/Cloudflare migration
  ([[008-second-storj-cdnread-bucket]],
  [[storj-to-b2-cloudflare-migration]]).
- The work was done on branch `migrate-skip-resume` (commits 035f12ba,
  76ba85f2, 0bf958be, a771f2d4) before `main` had a CI gate. No prompts doc
  was written at the time; this one was written afterwards.
- It is the first feature PR through the `main` gate from
  [[010-fork-main-ci-gate]].

## Task

- `snapshot migrate --skip <manifest-id>` (repeatable) skips a source
  snapshot manifest.
- Migrate resumes from existing checkpoints and picks the checkpoint by
  `EndTime`.
- `cdnread` retries 429, 5xx, and transport failures with exponential
  backoff (`transientCDNError`).
- Correct the environment variable names in the `cdnread` flag help.
- Merge the current `main` and resolve the `--parallel` conflict (upstream
  made it `uint`). Fix the cdnread lint findings.

## Acceptance criteria

- [x] Unit tests for the cdnread retry. An end-to-end test covers migrate
      `--skip` and resume (`tests/end_to_end_test/snapshot_migrate_test.go`).
- [x] Local `make lint` is clean on darwin and GOOS=linux. The cli and
      cdnread tests pass.
- [x] PR #486 passes all 8 required checks and is merged into `main` with a
      merge commit.

## Notes

- 2026-10-16: merged `origin/main` (cd385e83) into the branch. The only
  conflict was in `cli/command_snapshot_migrate.go`. Opened PR #486.
- 2026-10-16: all checks were green and the merge state was CLEAN. Merged #486 into
  `main` with a merge commit.
