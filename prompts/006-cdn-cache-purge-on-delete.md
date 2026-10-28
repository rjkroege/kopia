---
id: 006
title: purge Cloudflare cache entry on DeleteBlob for split-path provider
status: backlog
priority: low
type: feature
created: 2026-08-18
updated: 2026-08-18
tags: []
---

## Goal

When the split-path storage provider (see [[005-split-path-repository-backing]])
deletes a blob via the B2/write path, also purge the corresponding Cloudflare
edge-cache entry so a stale cached copy can't be served after the underlying
object is gone.

## Context

Blob IDs are content hashes, so a stale cached response is never *wrong*
content for that ID — only content that maintenance/GC has decided
shouldn't exist anymore. Without a purge, a deleted blob can still be served
from Cloudflare's edge for up to `Cache-Control: max-age` (86400s per the
runbook) after `DeleteBlob` returns. That's a bounded staleness window, not
a correctness bug, but it means GC space reclamation and "deleted" isn't
immediately reflected at the edge.

005's proposal punted on this — ship split-path with the bounded-staleness
behavior first, track the purge-on-delete as a separate, optional
enhancement here.

## Task

- [ ] confirm this only needs to run on the `DeleteBlob` path, not on
      overwritten `PutBlob` calls (blob IDs are content-addressed, so a
      given ID is only ever written once in practice)
- [ ] implement as another small decorator (matching the pattern used for
      `repo/blob/cdnread` itself, and `repo/blob/readonly`): wrap the
      split-path storage, override `DeleteBlob` to call the underlying
      delete, then fire a Cloudflare purge-by-URL API call for the blob's
      CDN URL
- [ ] Cloudflare API token/zone-ID configuration needed for the purge call —
      decide how these are supplied (new CLI flags vs. reusing existing env
      var conventions)
- [ ] decide on failure semantics: does a purge failure fail `DeleteBlob`,
      or just log and proceed (data is already deleted from B2 either way)

## Acceptance criteria

- deleting a blob through the split-path provider results in the blob no
  longer being served from the CDN URL within a short, bounded time
  (independent of the `max-age` window)
- a Cloudflare purge failure does not prevent the underlying B2 delete from
  succeeding

## Notes

- Not required for 005's initial ship — the `max-age` window is an
  acceptable bounded-staleness tradeoff for a first version.
