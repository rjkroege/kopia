---
id: 013
title: Land or close each open dependabot PR on rjkroege/kopia
status: done
priority: low
type: chore
created: 2026-10-27
updated: 2026-10-28
branch: (dependabot's own branches; fix branches deps/<name> if needed)
tags: [deps, dependabot, ci]
---

## Goal

Every open dependabot PR on `rjkroege/kopia` is either merged into `main`
(green on the 8 required checks) or closed with a one-line reason. Leave
nothing in the `BEHIND`/failing limbo the queue is in now.

## Context (as of 2026-10-27, `main` = `e8cc4e0a`)

- `main` is gated. See memory `project-main-gated` and `prompts/010`:
  - PR-only, `strict` (branch must be up to date), 8 required checks, merge
    commits only.
  - Title regex `^(feat|fix|…|build|…)\((…|deps|…)\)!?: .*$`. Dependabot's
    `build(deps): …` titles already match.
- `upstream/master` (`08350057`, 2026-10-19) has **none** of these bumps; its
  go.mod matches `main` for every module below. Merging them makes the fork's
  go.mod newer than upstream's. That's acceptable. At the next upstream sync,
  resolve go.mod/go.sum by keeping the higher version of each module, then
  `go mod tidy` (see the `upstream-sync` skill).
- `storj.io/uplink` is fork-only (upstream has no storj), so it never
  conflicts with upstream.
- Dependabot config: `.github/dependabot.yml`. gomod is weekly (Sunday 12:17
  UTC), actions and npm are monthly, all with a cooldown and a limit of 5 open
  PRs per ecosystem. Closed group PRs are recreated on the next run; to stop
  a specific bump, add an `ignore:` rule instead.

## The queue

| PR | Bump | CI (last run) | State | Plan |
|---|---|---|---|---|
| #513 | `github.com/chromedp/cdproto` pseudo-version → v0.158.4 | **Lint + Tests fail** on all OSes, Compat passes | BLOCKED | See "#513" below. Needs `chromedp` bumped in the same PR, or close plus an ignore rule. |
| #512 | `github.com/klauspost/pgzip` 1.2.7 → 1.3.0 | all 8 pass | BEHIND | Update the branch, rerun, merge. |
| #509 | `google.golang.org/api` 0.304.0 → 0.306.0 (common-golang-dependencies group) | all 8 pass | BEHIND | Update the branch, rerun, merge. |
| #502 | `storj.io/uplink` 1.14.3 → 1.14.5 (fork only) | Tests (ubuntu-latest) failed in `TestACL` (`tests/end_to_end_test/acl_test.go:161`); everything else passes | BEHIND | Probably a flake unrelated to uplink. Update the branch and rerun. If `TestACL` fails again, investigate (see below). |
| #500 | `github.com/klauspost/compress` 1.20.1 → 1.20.3 | all 8 pass | BEHIND | Update the branch, rerun, merge. |
| #482 | actions: `upload-artifact` 7.0.1→7.0.2, `codeql-action/upload-sarif` 4.38.2→4.39.0, `download-artifact` 8.0.1→8.0.2 | all 8 pass | BEHIND | Update the branch, rerun, merge. Patch bumps only. |

Re-list the queue first; it may have changed:
```
gh pr list --repo rjkroege/kopia --author 'app/dependabot' --state open \
  --json number,title,mergeStateStatus,createdAt
```

### #513: cdproto without chromedp

The lint failures are typecheck errors inside `chromedp@v0.16.0/kb/kb.go`:
`undefined: input.KeyDown`, `input.KeyUp`, `input.KeyChar`,
`input.ModifierShift`. cdproto v0.158.x renamed or removed these, and chromedp
v0.16.0 (what `main` pins) still uses them. The only users are
`tests/htmlui_e2e_test/` (`context_test.go`, `htmlui_e2e_test.go`). Options,
in order of preference:

1. **Bump both together** on a branch `deps/chromedp` cut from `origin/main`:
   `go get github.com/chromedp/chromedp@latest github.com/chromedp/cdproto@<the version chromedp requires>`,
   then `go mod tidy`. The latest chromedp release was v0.21.0 (2026-10-11).
   Let chromedp's own go.mod choose cdproto; don't force 0.158.4. Then:
   - fix any API breaks in `tests/htmlui_e2e_test/`;
   - `go vet -tags testing ./tests/htmlui_e2e_test/`;
   - check the lint below.

   Open the PR as `build(deps): bump chromedp and cdproto together`, and close
   #513 with "Superseded by #<n> (cdproto needs a matching chromedp)."
2. If chromedp's latest still doesn't build: close #513 and add an
   `ignore` rule for `github.com/chromedp/cdproto` to `dependabot.yml`
   (comment: "bumped together with chromedp by hand"). That rule is a fork
   change to an upstream file, so note it in the PR. Upstream has the same
   problem, though, so it may never conflict.

## Recipe per PR (the green ones)

Do them **one at a time**. Every merge moves `main`, which makes the rest
`BEHIND` again (strict), so batching CI runs gains nothing. Do the low-risk
ones first: #482, #500, #512, #509, then #502, then #513.

1. **Bring the branch up to date.** Pick one:
   - Preferred: comment `@dependabot rebase` on the PR
     (`gh pr comment <n> --repo rjkroege/kopia --body '@dependabot rebase'`).
     Dependabot force-pushes a rebased branch within a few minutes, and the
     PR keeps a single dependabot commit.
   - Or `gh pr update-branch <n> --repo rjkroege/kopia`, which adds a merge
     commit from `main`. After that, dependabot stops managing the PR (it
     won't rebase it again), which is fine if it merges right away.
2. **Watch the checks** in the background:
   `gh pr checks <n> --repo rjkroege/kopia --required --watch --interval 60`.
   Known flakes: `TestSnapshotActionsEmbeddedScript` (`text file busy`, arm),
   `TestServerScheduling` (Windows), `TestNewTimer/exactly_maxSleepTime` (`internal/sleepable`, macOS: timer fired at 39ms vs ~10ms, seen on #525). Rerun with
   `gh run rerun <run-id> --repo rjkroege/kopia --failed`.
3. **Ask the user to merge**: `gh pr merge <n> --repo rjkroege/kopia --merge`
   (auto mode blocks `gh pr merge`). Once it reports MERGED, go to the next PR.
4. Group PRs (#509, #482) may be **recreated or closed** by dependabot when
   `main` changes ("no longer needed" / "updatable in another way"). That's
   expected. Re-list and continue.

Local verification is optional for the green ones, since CI covers the 8 checks.
For #513's replacement and any PR that needs a hand-made change, run the local
checks from `prompts/011` "Local checks": `make lint`, cross-OS golangci-lint
(windows, linux, darwin, freebsd, openbsd), checklocks, and
`go test -tags testing` on the touched packages. Build in a worktree cut from
`origin/main` (for example `../kopia-sync`, `git switch -C deps/<name> origin/main`).
The main checkout has untracked storj files.

### If #502's `TestACL` fails again

- Find the run with `gh pr checks 502 --repo rjkroege/kopia --required`, then
  `gh run view <run-id> --repo rjkroege/kopia --log-failed | ug -n 'TestACL' -A30`.
- Read `tests/end_to_end_test/acl_test.go` around line 161. The assertion
  printed a snapshot list, so it's most likely a snapshot-count or timing
  expectation, not something uplink affects. TestACL doesn't use storj.
- If it reproduces on `main` without the bump, it's a pre-existing flake.
  Merge #502 after a green rerun, and record the flake in the "Known flakes"
  list here and in `prompts/011`.

## Rules

- Auto mode blocks these, so the user runs them: `gh pr merge`,
  `gh pr close` (sometimes), `git push --force-with-lease`.
- Always pass `--repo rjkroege/kopia` to gh; otherwise it resolves to
  kopia/kopia.
- Don't merge `main` into the xattr stack branches, and don't touch them for
  this task.
- Bash tool shell quirks: not POSIX sh, `cd` doesn't persist, no `<(…)`.

## Acceptance criteria

- [x] #482, #500, #509, #512 merged (or closed by dependabot as superseded,
      with the replacement merged).
- [x] #502 merged, or the TestACL failure diagnosed and recorded.
- [x] #513 closed, with either a chromedp+cdproto PR merged or an ignore
      rule merged.
- [x] `gh pr list --repo rjkroege/kopia --author 'app/dependabot' --state open`
      shows only PRs opened after this work started.
- [x] Progress section below updated with PR → merge commit.

## Progress

| PR | Outcome | Merge commit |
|---|---|---|
| #500 compress 1.20.1→1.20.3 | merged | `53b0e143` |
| #502 uplink 1.14.3→1.14.5 | merged; `TestACL` passed on the rerun (flake, not uplink) | `21647856` |
| #509 api 0.304→0.306 | closed by dependabot ("updatable in another way"), replaced by #522 | — |
| #522 storage 1.69→1.70, api →0.306 | merged | `df39c4c4` |
| #513 cdproto alone | closed, superseded by #523 | — |
| #523 chromedp v0.16→v0.21 + cdproto v0.157.8 | merged; ported `tests/htmlui_e2e_test/` to the generic `Action[T]` API; `.golangci.yml` excludes gocritic `dupOption` for that dir | `79af753e` |
| #512 pgzip 1.2.7→1.3.0 | closed (go.sum conflict; dependabot refused rebase as "edited", ignored `recreate`), replaced by #524 | — |
| #524 pgzip 1.2.7→1.3.0 | merged | `5d5352d9` |
| #482 actions group (upload-artifact, upload-sarif, download-artifact) | merged | `90517c9f` |

Notes:
- cdproto v0.158.4 also builds with chromedp v0.21.0, so no `ignore` rule was added. A future dependabot cdproto bump should be fine.
- Upstream sync: upstream still pins chromedp v0.16.0. Keep the fork's higher versions, the htmlui_e2e_test port, and the `.golangci.yml` dupOption rule.
- HTMLUI E2E on #523: TestChangeTheme and TestByteRepresentation PASS. TestEndToEndTest and TestConnectDisconnectReconnect still hit their pre-existing mid-test `t.Skip` ("Disconnect times out").
