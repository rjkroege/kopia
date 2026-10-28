---
id: 005
title: split path repository backing
status: backlog
priority: medium
type: feature
created: 2026-08-12
updated: 2026-08-18
tags: []
---

## Goal

split path kopia storage provider: r/o reads go through modified urls. r/w and w traffic
use alternate path to the same file.

## Context

It is possible to setup `rclone` to have a separate read and write path for file
storage where the:

- write path is into a bucket in Backblaze
- the read path is from the cloudflare cdn

This likely provides latency and bandwidth advantages. Also: the egress
path from backblaze -> cloudflare doesn't charge backblaze egress fees
so is likely cheaper for read heavy workloads.

Read storj-to-b2-cloudflare-migration.md to understand the approach.

I want to create a kopia storage provider that provides this split path
architecture.

Important requirements:

- r/o files are read from the cdn. this needs no special handling. once we
have the path, it's a go http connection to get a block. presumably many such
fetches can be inflight at once so threading does not require particular attention
- these are publically visible files so no auth needed here
- r/w files must go through the backblaze (b2) path
- maintenance (when we remove r/o files needs 
- the b2 path still needs auth for writing. but the bucket must be public to
interop with cdn
- must scrub the code carefully for security consequences of this structure
- cdn system limits largest block to 512mb. make sure that kopia doesn't
exceed this limit
- need a url translation scheme:
	- there's a path to file in the bucket
	- a path to a file in the cdn
	- seems like a string manipulation op
- r/o files obviate concern that there is (possible) consistency lag between
backblaze write and file appearing on cloudflare. as I understand the operating
guarantees

`prompts/storj-to-b2-cloudflare-migration.md` (copied in from
`~/Documents/wiki/2026/08-Aug/18/storj-to-b2-cloudflare-migration.md`) is a
*migration* runbook (Storj → B2), but its Phase 1 (B2 account/bucket setup),
Phase 2.2 (rclone B2 remote), Phase 5 (Cloudflare CDN + Transform Rule +
Cache-Control), and Phase 6 (pointing rclone's `download_url` at the CDN) are
directly reusable as the from-scratch B2+Cloudflare setup steps for this
feature — read those sections, not the Storj-specific inventory/transfer
phases (0, 3, 4, 7), which don't apply here.

## Proposal

### Architecture: a decorator around the existing S3 provider, not a new backend

Kopia's own `repo/blob/b2` package (built on the native `go-backblaze` SDK)
logs a deprecation warning on every `New()` call telling callers to use the
S3-compatible provider instead (`repo/blob/b2/b2_storage.go:256`). B2 also
exposes an S3-compatible endpoint (the runbook's Phase 1.2 step 5:
`s3.<region>.backblazeb2.com`), so the write path for this feature should be
built on the actively-maintained `repo/blob/s3` package pointed at that
endpoint via `Options.Endpoint`, not a new/duplicated B2 client.

Kopia already has a decorator pattern for wrapping a `blob.Storage` and
overriding only a subset of its methods: `repo/blob/readonly` (blocks
mutations), plus `repo/blob/retrying`, `repo/blob/throttling`,
`repo/blob/logging`, `repo/blob/beforeop`. This feature is the same shape —
override `GetBlob`, delegate everything else — and should follow
`repo/blob/readonly/readonly_storage.go` as the closest structural template
(embed the base storage, override one or two methods, pass the rest through
verbatim) rather than being "derived from b2" as a from-scratch copy.

Sketch:

```go
// repo/blob/cdnread/cdnread.go
package cdnread

type Options struct {
    s3.Options        // BucketName, Endpoint (B2's S3 endpoint), AccessKeyID, SecretAccessKey, Prefix, ...
    CDNURL string `json:"cdnURL"` // e.g. https://assets.yourdomain.com — no trailing slash
}

type cdnReadStorage struct {
    blob.Storage // embedded base = *s3.Storage; PutBlob/DeleteBlob/ListBlobs/GetMetadata pass through untouched
    cdnURL     string
    httpClient *http.Client
}

func New(ctx context.Context, opt *Options, isCreate bool) (blob.Storage, error) {
    base, err := s3.New(ctx, &opt.Options, isCreate)
    if err != nil {
        return nil, err
    }
    return &cdnReadStorage{Storage: base, cdnURL: strings.TrimSuffix(opt.CDNURL, "/"), httpClient: &http.Client{}}, nil
}

func (s *cdnReadStorage) GetBlob(ctx context.Context, id blob.ID, offset, length int64, output blob.OutputBuffer) error {
    // GET s.cdnURL + "/" + <prefix+id>, with a Range header when length > 0
    // no auth; 404 -> blob.ErrBlobNotFound; copy body via internal/iocopy.JustCopy into output
    // finish with blob.EnsureLengthExactly, matching the pattern in repo/blob/b2 and repo/blob/s3
}
```

`net/http.Client` is safe for concurrent use, so "many fetches in flight"
(a stated requirement) needs no extra locking or pooling beyond Go's
default transport.

### URL translation is mostly free

The prompt calls out needing "a url translation scheme" between the bucket
path and the CDN path. Per the runbook's §5.3 Transform Rule, Cloudflare
itself rewrites `https://assets.yourdomain.com/{key}` to
`https://f004.backblazeb2.com/file/{bucket}/{key}` at the edge. Kopia's
`GetBlob` override therefore never needs to know the bucket name or do any
bucket-aware path construction — it's just `cdnURL + "/" + objectName`,
where `objectName` is the same `prefix + string(blobID)` key already used
for `PutObject` on the S3 side. The actual bucket-scoping translation lives
in the Cloudflare Transform Rule config, not in kopia code.

### Block size — already answered, no code needed

Task item "check what's the largest kopia block size": answered directly
from the repo, no experimentation required.

- `MaxPackSize` (the size of the blob objects kopia actually writes to
  storage) defaults to 20 MiB (`repo/format/format_provider.go:102`) and is
  clamped to `[minValidPackSize, maxValidPackSize]` =
  `[10<<20, 120<<20]` (`repo/format/format_provider.go:16-17`).
- 120 MiB is comfortably under Cloudflare's 512 MB per-file cache limit —
  **every valid kopia repository format already satisfies the CDN's limit**,
  so this requires no enforcement code, just a note in the docs/PR
  description.

### Maintenance / deletion — open question

The original requirement line ("maintenance (when we remove r/o files
needs") was left unfinished. `DeleteBlob` already goes through the write
(B2/S3) path for free in the proposed design, since it's not overridden by
the decorator. The open question is Cloudflare cache staleness: a deleted
blob may still be served from Cloudflare's edge cache until the
`Cache-Control: max-age=86400` entry (runbook §5.5) expires or is purged.
Because kopia blob IDs are content hashes, a stale cached copy is never
*wrong* content for that ID — only content that "shouldn't exist anymore"
per repository GC. Two options:
- accept the `max-age` window as a bounded staleness tolerance for
  maintenance/GC (simplest — do nothing extra)
- call Cloudflare's cache-purge API for a blob's URL from `DeleteBlob` if
  immediate invalidation is required

Needs a decision before `DeleteBlob`/maintenance behavior is finalized.
This sounds simple: just another decorator implementation. Create an issue
to add this feature.

### Security review notes

(per the explicit requirement to "scrub the code carefully for security
consequences")

- The bucket must be public for B2↔Cloudflare interop (runbook §1.2.3,
  §5.3), so anyone who can construct a valid blob ID could in principle
  fetch pack contents directly from B2's origin URL. Blob IDs are content
  hashes and effectively unguessable, so this is low practical risk, but
  it's still worth confirming and stating explicitly in the PR: kopia
  encrypts object content client-side before it ever reaches `PutBlob`, so
  blob storage (of any provider) never holds plaintext regardless of bucket
  visibility. Confirm this against `repo/content`/`repo/format` and cite it
  explicitly rather than asserting it from memory.
- The B2 write path must use a bucket-scoped application key, never the
  master key (runbook §1.3) — master keys are rejected by the S3-compatible
  API anyway, but scope it explicitly in the setup docs.
- CDN reads are intentionally unauthenticated, which is fine given the
  content-hash argument above, but means the CDN hostname itself is the
  only thing standing between "public internet" and "read access to every
  blob ID anyone can guess or observe" — keep that hostname out of
  anything that isn't already public.

The hostname (by necessity needs to be in DNS) so conceivably is "public". But
shouldn't be publicized.

### Two real bugs found and fixed during live end-to-end validation (2026-08-18)

Both found by actually running `kopia repository create cdnread ...` against
the real bucket/CDN set up per [[b2-cloudflare-cdn-setup]], not by review —
worth recording since neither was anticipated in the design above.

1. **Missing `blob.AddSupportedStorage` registration.** The initial
   implementation only registered `cdnread` with the CLI's own provider list
   (`cli.mustRegisterStorageProvider`, used for flag parsing), but never
   called `repo/blob.AddSupportedStorage("cdnread", ...)` — the registration
   every other provider does in its own package `init()` (see
   `repo/blob/s3/s3_storage.go:473-475`, and the same pattern in `b2`,
   `storj`, `gcs`, etc.). Symptom: `kopia repository create cdnread`
   appeared to succeed (wrote `kopia.blobcfg`/`kopia.repository` fine, since
   `Connect()` calls `cdnread.New()` directly), but then failed on the
   automatic post-create reconnect with `storage type 'cdnread' not
   registered` — because reconnecting deserializes the persisted
   `repository.config` JSON and looks the type up via `repo/blob`'s own
   registry, which never had an entry. Fixed by adding the missing
   `func init() { blob.AddSupportedStorage(storageType, Options{}, New) }` to
   `cdnread_storage.go`. Confirmed via `~/Library/Logs/kopia/cli-logs/` —
   kopia's own structured logs made the two-phase failure obvious once
   checked, rather than guessing from the CLI's one-line error.
2. **Mutable control blobs must never go through the CDN, in code, not just
   Cloudflare config.** The original design routed every `GetBlob` through
   the CDN unconditionally. In practice: Cloudflare cached a `kopia.repository`
   response, the bucket was later emptied (during testing) and
   re-initialized, and the CDN kept serving the stale cached copy
   (`cf-cache-status: HIT`, `age: 400+`) for the `max-age` window — causing
   a fresh `repository create` to fail with `repository already initialized`
   even though B2 origin was empty. A first attempt fixed this by excluding
   specific blob-name patterns in the Cloudflare Cache Rule itself — correctly
   rejected: that pushes correctness into external, unversioned
   infrastructure config that has to independently track kopia's internal
   blob-naming conventions. The real fix belongs in `GetBlob` itself: kopia's
   pack blobs (prefixes `p`/`q`, content-addressed and immutable — see
   `PackBlobIDPrefixRegular`/`PackBlobIDPrefixSpecial` in
   `repo/content/content_manager.go:34-35`) are the *only* blob category safe
   to ever route through the CDN. Everything else — `kopia.repository`,
   `kopia.blobcfg`, `kopia.maintenance`, index/manifest/session blobs — is
   mutable over the life of the repository and must always read straight
   from the write-path (Backblaze) storage. Implemented as
   `isImmutablePackBlob(id)` gating the CDN path in `GetBlob`; the Cloudflare
   Cache Rule was simplified back to unconditional caching for the host,
   since the code itself now guarantees only genuinely cacheable blobs ever
   reach it. Covered by `TestGetBlob_MutableControlBlobsBypassCDN` and
   `TestGetBlob_QPrefixServedFromCDN` in `cdnread_storage_test.go`.

Both fixes verified together with a full live round trip: `repository
create` → `repository connect` → `snapshot create` (real files, one random
512KB binary) → full maintenance → `restore` → `diff -r` against the
originals, byte-identical. Real pack blob confirmed CDN-cached
(`cf-cache-status: HIT`); `kopia.repository` confirmed to reflect live
origin state throughout, never served stale to kopia itself.

### Testing

- Reuse the existing generic conformance suite (`internal/blobtesting`,
  `blobtesting.VerifyStorage` and friends) that every other backend
  (including Storj on `better-storj`) is tested with — no new test harness
  design needed for the storage-interface conformance piece.
- No CDN test double exists yet. Two options, not mutually exclusive:
  - a real Cloudflare-fronted B2 bucket, gated as a fork-only live-backend
    CI job — directly reuse the pattern from `better-storj-fulltest`'s
    `.github/workflows/storj-fulltest.yml` (not yet on `better-storj`,
    still branch-only) and the `providers-extra.yml` wiring from
    `better-storj`
  - an `httptest.Server` faking CDN GET/Range semantics for fast unit tests
    of the `GetBlob` override, independent of any live backend

## Task

- [x] check what's the largest kopia block size — 120 MiB max
      (`maxValidPackSize`), well under the CDN's 512 MB cap; no enforcement
      code needed, just document it
- [x] trim `storj-to-b2-cloudflare-migration.md`'s Phases 1, 2.2, 5, 6 into a
      from-scratch B2+Cloudflare setup doc for this feature (drop the
      Storj-specific inventory/transfer/cutover phases) — see
      [[b2-cloudflare-cdn-setup]], ends with the exact
      `kopia repository create cdnread ...` invocation
- [x] reuse runbook §5.6 ("smoke test") as the local dev test for "write a
      test file to the b2 bucket and check its existence" — done, run
      repeatedly live throughout setup and validation (2026-08-18)
- [x] decide the deletion/cache-invalidation story for GC — punted to
      [[006-cdn-cache-purge-on-delete]] as a follow-on; 005 ships with the
      `max-age` bounded-staleness tradeoff and no purge-on-delete
- [x] confirm content is client-side encrypted before it reaches blob
      storage regardless of provider — `repo/content/content_manager_lock_free.go:78`
      calls `sm.format.Encryptor().Encrypt(...)` on (optionally compressed)
      data before it's ever handed to a `blob.Storage`, so this holds
      regardless of backend
- [x] implement `repo/blob/cdnread`: `Options{s3.Options, CDNURL string}`,
      `New()`, decorator overriding only `GetBlob`, modeled on
      `repo/blob/readonly/readonly_storage.go` — see
      `repo/blob/cdnread/cdnread_options.go`,
      `repo/blob/cdnread/cdnread_storage.go`, and unit tests in
      `repo/blob/cdnread/cdnread_storage_test.go` (httptest-based CDN fake +
      `blobtesting.NewMapStorage` base; `go build ./...`, `go vet ./...`,
      `go test ./repo/blob/cdnread/...` all pass)
- [x] implement CLI wiring — `cli/storage_cdnread.go`, registered as
      provider `cdnread` via `mustRegisterStorageProvider`; confirmed with
      `repository create cdnread --help` / `connect` / `set-client`
- [ ] wire fork-only live-backend CI following the `providers-extra.yml` /
      `storj-fulltest.yml` pattern — needs a real B2 bucket + Cloudflare CDN
      provisioned per the runbook first; not attempted (no credentials/infra
      access)
- [ ] `golangci-lint`/`gofumpt` not run locally (tools not installed in this
      environment); `gofmt -l` and `go vet ./...` are clean across the
      repo — run `make ci-setup && make lint` before merge
- [x] fix the two bugs found in live validation (missing storage-type
      registration; mutable control blobs bypassing the CDN in code) — see
      Proposal → "Two real bugs found and fixed" above

## Acceptance criteria

- [x] we can configure a new remote — `kopia repository create cdnread
      --bucket=... --endpoint=... --access-key=... --secret-access-key=...
      --cdn-url=...` against the real `test-for-kopia` B2 bucket +
      `XXX` CDN, 2026-08-18
- [x] we can snapshot to the remote — real snapshot of a small test
      directory (text file + 512KB random binary), full maintenance ran
      clean afterward
- [x] we can restore from the remote — restored to a separate directory,
      `diff -r` against the originals: byte-identical
- [x] all tests identified previously in our work on the storj provider have backend and
can run — done in [[007-cdnread-blobtesting-conformance]]:
`repo/blob/cdnread/cdnread_provider_test.go` adds `VerifyStorage`,
`AssertConnectionInfoRoundTrips`, `ValidateProvider`, `TestCleanupOldData`,
negative-path tests, and a supplementary CDN-routing proof
(`TestCDNReadStorage_ReadsGoThroughCDN`, verified via the `Cf-Ray` header);
`make provider-tests PROVIDER_TEST_TARGET=cdnread` passes against
`test-for-kopia`, 2026-08-19


## Notes

- Base this on the actively-maintained `repo/blob/s3` provider pointed at
  B2's S3-compatible endpoint, not the deprecated native `repo/blob/b2`
  package (deprecation warning logged in `repo/blob/b2/b2_storage.go:256`).
- Structural precedent for "wrap an existing Storage, override a method
  subset": `repo/blob/readonly/readonly_storage.go`.
- Precedent for gating a live/real-backend integration test behind a
  fork-only CI job: the `better-storj-fulltest` branch's
  `.github/workflows/storj-fulltest.yml` — not yet merged into
  `better-storj`, still only exists on that branch as of 2026-08-18.
- `repo/blob/registry.go`'s `AddSupportedStorage[T]` + `mustRegisterStorageProvider`
  in `cli/storage_b2.go`/`cli/storage_s3.go` are the concrete registration
  patterns to copy for CLI wiring.

512 MB per-file cache limit

- [x] what's the largest kopia block — 120 MiB (`maxValidPackSize`), see Proposal above
- [ ] encryption: are the blocks encrypted? (presumably yes — needs a citation, see Task)

Setting this up is... complicated.

- block fetches from CDN space
- private backblaze bucket? but it's much harder. so public bucket

- how secure is Kopia if the bucket is public? Probably good enough — see
  Proposal → Security review notes above for the reasoning, still needs the
  encryption citation to close out.
- details
