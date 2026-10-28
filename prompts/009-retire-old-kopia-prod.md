---
id: 009
title: retire the old kopia-prod repository after the passphrase/key rotation (step 19)
status: blocked
priority: high
type: ops
created: 2026-10-06
updated: 2026-10-06
tags: [security, b2, cloudflare, rotation]
---

## Goal

Finish the credential rotation by destroying everything that the leaked
`kopia-prod` connection token can still unlock or that only existed to run the
migration. **Not before 2026-10-13** (one-week fallback period), and only after
the "Prerequisites" below are done.

No secrets are in this file. Secrets live in the macOS keychain under the
item names listed below; never print them (see memory
`feedback-never-print-kopia-tokens`).

## Context

- 2026-09-16: a Claude session ran `KopiaServer get demo` on `macvm`, which
  printed the full production connection token (B2 key + secret for bucket
  `kopia-prod`, repo password, CDN URL) into transcript
  `~/.claude/projects/-Users-rjkroege-tools-kopiaserver/c7be61e9-d243-40d8-a7ef-8f1446465413.jsonl`.
- `kopia repository change-password` would not help: it re-wraps the format
  blob but keeps the same master key. So the repo was re-keyed by migrating
  into a new repository.
- Done 2026-10-01 … 2026-10-06 (steps 1–18.1 of the rotation plan):
  - Old bucket `kopia-prod` set **private**; leaked B2 key (ID ending `000004`)
    deleted. Read-only "Key A" (`kopia-migrate-ro`) created for the migration.
  - New bucket `kopia-prod-2610`, Key B (read/write, restricted to it).
  - Cloudflare zone `XXX': rewrite rule for
    `bucketyprodo` now targets `/file/kopia-prod-2610`; guards on all three
    rules (`XXX`, `XXX`, `XXX`) fixed to end in `/`;
    cache purged for `bucketyprodo` after the repoint. **No further purge is
    needed**: the old bucket is private and no hostname routes to it.
  - New repo `kopia-prod-2610` (format v3, unique ID `b20f2d93…`, new master
    key) created and populated with `snapshot migrate` (patched binary, see
    "Code" below).
  - 219 complete snapshots migrated; sizes and file counts match the old repo
    exactly. 13 `adaskin:/a` Aug/Sep 2026 snapshots deliberately skipped
    (incl. the 52 TB recursive-copy error `de4d81ee…`); 28 incomplete
    (checkpoint/canceled) source snapshots are not migrated by design.
  - `snapshot verify --verify-files-percent=10`: 0 errors. 144 leftover
    migration checkpoints deleted; new repo = 219 snapshots, 0 incomplete.
  - Default kopia config on `adaskin` reconnected to `kopia-prod-2610`.
  - Migration caches (`472218b52c8a0177`, `822570fe378c2d4c`) deleted.

### Data that is lost when the old bucket is deleted

Only data referenced exclusively by:
- the 13 skipped `adaskin:/a` snapshots (2026-09-07, 09-18 … 09-25 12:51), and
- the 28 incomplete snapshots, notably all of
  `static:/Users/rjkroege/archival-restores/*` (checkpoint-only, ~2.7 TB
  logical) and the 2022-01-16 `schubert:/Volumes/MediaStore` checkpoints
  (~370 GB beyond the one complete 14 GB snapshot).

Accepted 2026-10-05. Re-confirm before step 2 below.

## Prerequisites (before 2026-10-13)

- [ ] Every backup host reconnected to `kopia-prod-2610`: schubert,
      villalobos, helios4, ween, rachmaninoff, shostakovich, static,
      macbook-pro. Template (secrets via env, never on the command line):

      kopia repo disconnect
      CDNREAD_ACCESS_KEY_ID=… CDNREAD_SECRET_ACCESS_KEY=… KOPIA_PASSWORD=… \
        kopia repository connect cdnread --bucket kopia-prod-2610 \
        --endpoint s3.ca-east-006.backblazeb2.com --region ca-east-006 \
        --cdn-url https://XXX

      Note the env names have **no** `KOPIA_` prefix (the old help text was
      wrong; fixed in `76ba85f2`).
- [ ] At least one successful new backup from each active host into
      `kopia-prod-2610` (`kopia snapshot list --all`).
- [ ] KopiaServer / kopiabrowser (Mac + `macvm`) moved to the new repo — **the
      user is doing this personally; agents must not touch them**.

## Step 19 tasks

1. [ ] **Final fallback check** — if anything is still needed from the old
       repo, restore it now via `~/.config/kopia/repo-old.config` (read-only,
       Key A).
2. [ ] **Empty and delete bucket `kopia-prod`** (B2 console; Key A cannot
       delete):
       - Bucket Settings → Lifecycle → custom rule for prefix `` (all files):
         hide after 1 day, delete hidden after 1 day. This also removes old
         file versions.
       - When the bucket shows 0 files, delete it.
3. [ ] **Delete Key A** (`kopia-migrate-ro`) in B2 → Application Keys.
4. [ ] **Disconnect and remove migration configs** (removes kopia's own keychain
       password entry for the old config):

       kopia repo disconnect --config-file ~/.config/kopia/repo-old.config
       kopia repo disconnect --config-file ~/.config/kopia/repo-new.config
       rm -f ~/.config/kopia/repo-old.config* ~/.config/kopia/repo-new.config*

5. [ ] **Remove migration tooling and logs:**

       rm -rf ~/.config/kopia/{migrate-2610.sh,verify-2610.sh,MigrateProgress,MigrateProgress.app,MigrateProgress.swift,VerifyProgress.app,VerifyProgress.swift}
       rm -f ~/Library/Logs/kopia-migrate-2610*.log ~/Library/Logs/kopia-verify-2610*.log
       rm -f ~/Library/Logs/kopia/cli-logs/*-snapshot-migrate.* ~/Library/Logs/kopia/cli-logs/*-snapshot-verify.*

       Keep `~/.config/kopia/kopia-migrate` only if it is going to replace
       `/usr/local/bin/kopia` (task 8).
6. [ ] **Delete keychain items** no longer needed:

       security delete-generic-password -s kopia-old-ro-keyid
       security delete-generic-password -s kopia-old-ro-secret
       security delete-generic-password -s kopia-old-pw
       security delete-generic-password -s cf-api-token

       Keep `kopia-new-keyid`, `kopia-new-secret`, `kopia-new-pw` while hosts
       still need reconnecting; afterwards they are redundant with kopia's own
       per-config keychain entry and the password manager.
7. [ ] **Cloudflare API token** `kopia-roll-over`: delete it in the dashboard
       (My Profile → API Tokens). It expires 2026-10-10 anyway.
8. [ ] **Install the patched kopia** (optional but recommended): the stock
       `/usr/local/bin/kopia` lacks the cdnread 429/5xx retry (`a771f2d4`), so
       heavy parallel restores through the CDN can fail. Build from
       `migrate-skip-resume` (or wherever it lands) and install.
9. [ ] **Scrub the leaked token from the transcript** `c7be61e9-…jsonl`
       (replace every `eyJ2ZXJzaW9uIjoi…` token string with `<REDACTED>`).
       Do it with a script that never prints the token. Also check
       `macvm` for any shell history or logs from 2026-09-16 that captured the
       `KopiaServer get demo` output.
10. [ ] **Update memory / docs**: mark this prompt done; note the final state
        in [[b2-cloudflare-cdn-setup]] (bucket `kopia-prod-2610`).

## Code

Branch `migrate-skip-resume` (not pushed, not merged):
- `035f12ba` feat(cli): `snapshot migrate --skip` and resume from checkpoints
- `76ba85f2` fix(cdnread): correct env var names in flag help
- `0bf958be` fix(cli): pick migrate resume checkpoint by EndTime
- `a771f2d4` fix(cdnread): retry throttled and transient CDN reads

Open issues worth a follow-up prompt:
- `snapshot migrate` never deletes its own checkpoints, and the uploader
  applies retention with deletion after every checkpoint
  (`snapshot/upload/upload.go:545`); during the migration this deleted
  nothing, but it is a hazard.
- `snapshot migrate` silently ignores incomplete source snapshots
  (debug log only); an `--include-incomplete` option would have preserved the
  checkpoint-only `static:` restores.
