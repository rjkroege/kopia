---
id: 010
title: Make rjkroege/kopia main a consumable dependency, gated by the standard kopia CI
status: in-progress
priority: medium
type: feature
created: 2026-10-16
updated: 2026-10-16
tags: [ci, github, go-modules, fork]
---

## Goal

Other projects can depend on `rjkroege/kopia@main`. A change lands on `main`
only through a PR, and only after the standard kopia test suite passes.

## Context

State as of 2026-10-16:

- `origin` = `rjkroege/kopia` is a public fork. `main` (b19700a1, formerly
  `localisms`) is now the default branch, and `master` has been deleted.
- Branch protection on `main` is currently minimal: it blocks force pushes and
  deletion. It does not require a PR or any checks, and `enforce_admins=false`.
- GitHub Actions is enabled. Because the repo is public, Actions minutes
  are free, including the Windows and macOS runners.
- `go.mod` declares `module github.com/kopia/kopia`. It must stay that way so
  that syncing with upstream stays cheap.
- Every upstream workflow filters on `branches: [ master ]`. Since `main`
  appeared, PRs into `main` trigger only the unfiltered workflows (code
  coverage, compat, race, stress, license, PR title, auto-merge). The core
  ones (`tests.yml`, `lint.yml`, `make.yml`, `htmlui-tests.yml`) never run.
- Known red in the fork: `License Check` fails at `npm audit` (2 high
  vulnerabilities in `app/`) during `make app-node-modules`. `make ci-setup`,
  which `tests.yml` uses, also depends on `app-node-modules`. **Tests may be
  red for the same reason.** Verify this first.

> Reviewer: suppress the failure. Done: `app/Makefile` audit step prefixed with `-` (non-fatal).

- `auto-merge.yml` fails in the fork because it is Dependabot automation
  written for upstream.

## Task

Proposal, in order:

### 1. Consuming the fork as a dependency (no repo change)

Keep the upstream module path. Consumers use a `replace` directive:

```
go mod edit -replace=github.com/kopia/kopia=github.com/rjkroege/kopia@main
go mod tidy   # resolves @main to a pseudo-version, e.g. v0.23.2-0.20261016...-b19700a1xxxx
```

- This works through `proxy.golang.org` because the repo is public.
- Limitation: a `replace` applies only in the consumer's main module. A
  consumer that is itself a library imported elsewhere can't pass it on, and
  each final binary needs its own `replace`. That suits your own tools.

> Reviewer: `replace` approach accepted.

- Rejected alternative: renaming the module to `github.com/rjkroege/kopia`.
  It touches about 1000 import lines and conflicts on every upstream sync.
- Optional: tag `main` with semver prereleases (`v0.23.2-rjk.1`, ...) so
  consumers get readable versions. Note that `v*` tags trigger
  `make.yml`/`providers-*` release jobs (see step 2).

### 2. Point CI at `main`

Small edits under `.github/workflows/`, kept tiny so upstream rebases and
merges conflict on a single line:

- In `tests.yml`, `lint.yml`, `make.yml`, `htmlui-tests.yml`,
  `volume-shadow-copy-test.yml`, `code-coverage.yml`, `race-detector.yml`,
  `license-check.yml`, `compat-test.yml`, `stress-test.yml`,
  `ossf-scorecard.yml`, and `endurance-test.yml`/`providers-core.yml` (push
  only): change `branches: [ master ]` to `branches: [ master, main ]`.
- Disable in the fork, either by adding
  `if: github.repository == 'kopia/kopia'` or by disabling them in the
  Actions UI (`gh workflow disable`), which needs no code change:
  `auto-merge.yml`, `stale.yml`, `endurance-test.yml`, `providers-core.yml`,
  `providers-extra.yml` (they need cloud secrets the fork doesn't have), and
  the release/publish steps of `make.yml` (27 `secrets.*` uses: signing,
  notarization, publishing). Prefer `gh workflow disable` for whole
  workflows. For `make.yml`, keep the build and gate only its publish steps.
- Fix or accept the `npm audit` failure (`app/` dependency bump or audit
  level). Required checks must be green on `main` before you enable them.

### 3. Required checks: "the standard kopia test suite"

These checks are required before merging:

| Check | Workflow | Why |
|---|---|---|
| `Tests (ubuntu-latest)`, `Tests (macos-latest)`, `Tests (windows-latest)`, `Tests (ubuntu-24.04-arm)` | tests.yml | `make ci-tests` (vet + unit) and `ci-integration-tests` on all four platforms; this is the core suite |
| `Lint` | lint.yml | upstream gates on it too |
| `Race Detector` | race-detector.yml | cheap and catches concurrency bugs in local backends |
| `Compatibility Test` | compat-test.yml | catches repository-format regressions |

These run but are not required: code coverage, stress, htmlui, license,
make (build only), and VSS.

The exact check names come from the job `name:` plus the matrix value. Read
them from the first green run (`gh pr checks <n>`) before writing the rule.

### 4. Branch protection on `main`

```
gh api -X PUT repos/rjkroege/kopia/branches/main/protection --input - <<EOF
{
  "required_status_checks": { "strict": true, "contexts": [ <names from step 3> ] },
  "enforce_admins": true,
  "required_pull_request_reviews": { "required_approving_review_count": 0 },
  "restrictions": null,
  "allow_force_pushes": false,
  "allow_deletions": false,
  "required_linear_history": false
}
EOF
```

- `enforce_admins: true` matters: without it, you, as admin, can push
  straight to `main` and bypass the gate.
- `0` approvals: a solo owner can't approve their own PR. The PR plus green
  checks is the gate.
- `strict: true`: the PR branch must be up to date with `main`, so the tested
  commit is what lands.

### 5. Workflow changes that follow from this

- **Upstream sync becomes a merge-based PR.** Run
  `git switch -c sync/upstream-YYYYMMDD main && git merge upstream/master`,
  push, and open a PR. CI gates it. The rebase-onto-master flow from
  [[002-rebase-to-head]] no longer works on `main`, because it would need a
  force push. Rebasing a feature branch onto `main` is still fine.
- Feature branches (`migrate-skip-resume`, and so on) get pushed and PR'd
  into `main` instead of merged locally.
- Dependabot now targets `main`. You can keep it, since its PRs will be gated
  by CI, or turn it off with an empty `.github/dependabot.yml` override.

### 6. Rollout order

1. Make the workflow edits (step 2) in a PR into `main`. Protection is not
   yet strict, so you can merge it.
2. Get every candidate required check green on that PR, and fix `npm audit`
   if needed.
3. Record the exact check names, then apply the protection from step 4.
4. Prove it: open a throwaway PR that breaks a unit test and confirm that
   merge is blocked.
5. Push `migrate-skip-resume` as the first real gated PR.
6. In one consumer project, add the `replace` and run `go build`.

## Acceptance criteria

- [ ] `tests.yml`, `lint.yml`, `race-detector.yml`, and `compat-test.yml`
      run on PRs into `main` and on pushes to `main`.
- [ ] Fork-irrelevant workflows (auto-merge, stale, endurance, providers,
      release publish) do not run or fail on the fork.
- [ ] `main` protection requires those checks, a PR, an up-to-date branch,
      and enforces it for admins.
- [ ] A deliberately failing PR cannot be merged, and a direct `git push
      origin main` is rejected.
- [ ] One consumer project builds against `rjkroege/kopia@main` through
      `replace`.

## Notes

- Open question: is full `tests.yml` (4 OS × unit + integration, roughly
  30–60 min) the gate you want, or Linux-only for speed with the rest
  advisory? The proposal assumes all four.

> Decided: all four OSes are required.

- Open question: do you want semver prerelease tags, or are pseudo-versions
  from `@main` enough?

> Decided: pseudo-versions from `@main`; no tags.

- The Storj and cdnread unit tests must skip cleanly without credentials in
  `tests.yml`. Confirm this on the first run (related: [[004-storj-with-fulltest]]).

yes.
## Progress

- 2026-10-16: branch `ci/main-gate`, PR #483 into `main`. Added `main` to
  every `master` branch filter (13 workflows) and made `npm audit` non-fatal.
  Ran `gh workflow disable` on `auto-merge` and on `Manage stale issues and
  pull requests`. publish/endurance/stress/providers were already gated
  upstream on `github.repository == 'kopia/kopia'`, so they needed no change.
- Found an extra workflow: `Storj Full Test (fork-only, not for upstream)`
  is active on origin (from `better-storj-fulltest`). It is not on `main`.
- Next: wait for the checks on #483, record the exact check names, then
  apply protection (step 4).
- 2026-10-16: first CI run on #483 had two failures.
  - `Check PR Title`: upstream's regex requires `type(scope): ...`. I renamed
    the PR to `chore(ci): ...`. **Every future PR title must match**
    `^(feat|fix|breaking|build|chore|docs|style|refactor|test)\((ci|cli|deps-dev|deps|general|infra|kopiaui|lint|notifications|providers|repository|server|site|snapshots|testing|ui)\)!?: `.
  - `Lint`: these were `govulncheck` findings, not style. There were 16 vulns
    in Go 1.25.13 stdlib, x/net v0.58.0, x/crypto, and grpc. Upstream had
    already moved to Go 1.27.2 and x/net v0.60.0.
- Fix: merged upstream/master (76e4e8ea, 85 commits) as the second commit on
  #483 (abd5ff69). Conflicts were `cli/command_ls.go` (kept `--emit-json`)
  and go.mod/go.sum (took upstream, re-added `storj.io/uplink`, tidied).
  Locally, `go build ./...` passes, the storj/cdnread/cli tests pass, and
  govulncheck reports 0 reachable vulns.
- **Merge #483 with a merge commit, never squash.** Squashing would drop
  the upstream parent, and the next sync would re-conflict on all 85
  commits. Once protection is on, consider disabling squash for the repo or
  requiring merge commits.
- 2026-10-16: ran `gh repo edit --enable-merge-commit --enable-squash-merge=false
  --enable-rebase-merge=false`. Merge commits are now the only merge method.
  Keep `required_linear_history: false` in step 4, because a linear-history
  rule would block merge commits.
- 2026-10-16: second CI run. Everything was green except `Lint` (ubuntu
  and macos). All 57 findings were in fork-only code: storj, sharded,
  cdnread tests, `ls --emit-json`, and file-less config (open.go,
  maintenance_run.go). That code had never been through upstream's linter.
  I fixed it in 3c93bd2d with no behavior change, and the `--emit-json`
  keys are unchanged. Local `make lint` is clean on darwin and on
  GOOS=linux. Tests pass in the storj, cdnread, sharded, cli, and repo
  packages.
- **Divergence:** the storj lint fixes exist only on `main`. They are not
  on `better-storj`, the branch behind upstream PR #3450 (see
  [[001-address-review-comments]]). Port them there when revising #3450,
  since upstream CI will flag the same issues.
- 2026-10-16: third run on #483 (3c93bd2d) was all green. Merged #483 with a
  merge commit (cd385e83). Applied protection with these required contexts:
  `Tests (ubuntu-latest)`, `Tests (macos-latest)`, `Tests (windows-latest)`,
  `Tests (ubuntu-24.04-arm)`, `Lint (ubuntu-latest)`, `Lint (macos-latest)`,
  `Tests` (the Race Detector job, whose job name is just "Tests"), and
  `Compat Test`. Also set strict=true, enforce_admins=true, 0 approvals, no
  force-push or delete, and no linear-history requirement.
- Verified: a direct `git push origin main` is rejected with "Changes must be
  made through a pull request".
- In progress: throwaway PR #484 (`test/gate-must-fail`) adds a deliberately
  failing test in `internal/clock`. Expect merge to be blocked, then close
  the PR and delete the branch.
- 2026-10-16: verified the gate. On #484 all four `Tests (*)` jobs and the
  Race Detector `Tests` failed. `gh pr merge` was refused with "the base
  branch policy prohibits the merge", and the PR state was BLOCKED. I
  closed #484 and deleted `test/gate-must-fail` and `ci/main-gate`.
- Remaining: step 5 (push `migrate-skip-resume` as the first real gated PR;
  it needs updating onto the new `main`) and step 6 (a consumer project
  `replace` check). Both are waiting on the user.
- 2026-10-16: step 5 is done. #486 (`migrate-skip-resume`, [[012-migrate-skip-resume]])
  passed all 8 required checks and was merged through the gate. Remaining: step 6
  (a consumer `replace` check).
- 2026-10-16: the user is handling step 6. Claude's part is complete; this stays in-progress until the consumer check is done.
