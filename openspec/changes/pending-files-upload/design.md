## Context

Corrected twice after reading the actual code — first against the
`KnowledgeStore` process docs (which turned out to describe a mostly-accurate
pipeline, just under the wrong binary name), then again after this change's
first implementation pass patched the wrong Go service entirely. Worth
recording both mistakes so the second one isn't repeated a third time:

- **First mistake:** the process docs referred to a staging service as
  `server/cmd/pdf-parser`. No such binary exists; the closest name in the
  tree, `server/cmd/service-pdf-parser`, does exist and was (wrongly)
  assumed to be it.
- **Second mistake:** `server/cmd/service-pdf-parser` is dead code — it is
  not referenced by any `mise` task and does not run in this environment.
  The real staging service is `server/cmd/doc-service` (task names
  `build-doc-service`, `doc-service-start`, `doc-service-run`, etc. in
  `mise.toml`), run as its own process independent of the `air`-managed main
  server. The first implementation pass added the `.pending` filter and the
  env var rename to `service-pdf-parser` only, which compiled and had
  passing tests but had **zero effect at runtime** — confirmed when a real
  `.pending` file was ingested unattributed by the live `doc-service`
  process during manual testing. `doc-service` also already contains real
  per-entry zip extraction (`ingestInputFile`/`ingestZipChildren`) — the
  earlier version of this doc, and of this design, claimed no such thing
  existed; that was wrong, an artifact of investigating the wrong file.

The real architecture (`server/cmd/doc-service/main.go`):

- `kbhandler.UploadInputs` (`server/api/kbhandler/upload_handler.go`) writes
  the uploaded file into a staging directory (env var `STAGING_DIR`, now
  `UPLOAD_FILE_STAGING_DIR`) and, in the same DB transaction, inserts a
  `kb.inputs` row via `insertUploadedInputRecord` with
  `tenant_id`/`ks_store_id`/`type`/`processing_mode`/etc. and `file_name` set
  to the staged path, `backup_filename` left empty. It does not copy the
  file to backup/home itself.
- Independently, `doc-service` watches the staging directory via `fsnotify`
  (debounced, plus a periodic fallback rescan). For each file found, it
  copies it to `DATA_BACKUP_DIR` and a record-sharded path under
  `DATA_HOME_DIR/Artifacts/{id/1000}/{id}/`, removes it from staging, and
  either **updates** the existing `kb.inputs` row where
  `file_name = <staged path> AND backup_filename = ''` (the row
  `UploadInputs` already inserted) or, if no such row exists, **inserts** a
  brand-new one with no `tenant_id` and raises `AlarmMissingTenantIDAtInsert`.
  For a `.zip`, it then opens the archive and calls `ingestInputFile` again
  per entry (`ingestZipChildren`), each child inheriting the parent zip
  record's `tenant_id`/`ks_store_id`/`ks_desc`/processing mode.
- In this repo's `mise.local.toml`, `STAGING_DIR` and `DATA_STAGING_DIR` were
  set to the same path — two env var names for what is meant to be one
  directory. Confirmed to be an accidental duplication, not an intentional
  two-directory design; consolidated into a single `UPLOAD_FILE_STAGING_DIR`
  var (see Decision 3) — which, unrelated to this change, `doc-service`
  already happened to be reading under that exact name before this change
  started, a naming collision that would have been another warning sign had
  it been noticed sooner.

As of the 2026-09-24 doc-processing attribution fix
(`KnowledgeStore/doc-repo/devdocs/202609/2026092403-devdoc-doc-processing-user-id-attribution.md`),
a `kb.inputs` row with an empty/default `tenant_id` now causes doc-processing
to refuse to run. A file copied directly into the staging directory outside
`UploadInputs` never gets a matching pre-existing row, so `doc-service`
inserts it unattributed and it hits that refusal. This capability closes that
gap: keep such files out of the pipeline (`.pending` suffix) until an
authenticated admin claims them, and have the claim itself do exactly what
`UploadInputs` does — insert the same kind of pre-existing row with a real
`tenant_id` — before the file becomes visible under its real name. The
existing `doc-service` staging loop then completes it exactly as it
would a normal upload, unmodified — zip child extraction included, since
that logic triggers on the file's real extension on disk, not on anything
the claim inserts.

## Goals / Non-Goals

**Goals:**
- Let internal developers/admins get files into the Knowledge Store by
  copying them directly onto the server, without producing unattributed
  `kb.inputs` records.
- Reuse the existing upload insert path (`insertUploadedInputRecord`) as-is
  rather than inventing new ingestion logic — a pending claim should produce
  a `kb.inputs` row indistinguishable from one `UploadInputs` would have
  created for the same file.
- Keep the feature admin-only; this is a trusted-operator tool, not a
  customer-facing upload alternative.
- Fix the accidental `STAGING_DIR`/`DATA_STAGING_DIR` duplication as part of
  this change, since the pending-file flow depends on both the upload
  handler and the staging watcher agreeing on one directory.

**Non-Goals:**
- Per-user staging isolation. The staging directory remains a single shared
  directory; only people with machine access can place files there at all
  (internal developers/admins), and this release accepts that any admin can
  see and claim any pending file. Not a concern for external customers, who
  have no filesystem access.
- Changing zip child-record extraction. `doc-service` already extracts each
  zip entry into its own `kb.inputs` row (`ingestZipChildren`) for a
  normally staged `.zip`; a claimed pending `.zip` goes through the exact
  same extraction unmodified, since it's triggered by the file's real
  extension on disk. Nothing to build here.
- Any change to the normal browser Upload Files flow or its processing
  modes, beyond the env var rename.
- Automatic/scheduled ingestion of pending files — claiming is always an
  explicit, manual admin action.
- Retention/cleanup policy for abandoned `.pending` files — out of scope for
  this release; may revisit later if stale files accumulate.

## Decisions

**1. Marker convention: append `.pending` to the full original filename.**
`report.pdf` → `report.pdf.pending`. Claiming is then a pure
prefix-preserving transform (strip the trailing `.pending`), so the existing
extension-based `type` handling needs no change.

**2. `doc-service`'s staging loop ignores `.pending` files outright**
(skipped before `entry.Info()`/copy/insert, in `processStagingOnce` — which
runs both on `fsnotify` events and the periodic fallback rescan, so both
paths are covered by one filter check). Guarantees a `.pending` file is
never picked up automatically, however long it sits there.

**3. Consolidate `STAGING_DIR` and `DATA_STAGING_DIR` into one
`UPLOAD_FILE_STAGING_DIR` env var**, read by both `kbhandler.UploadInputs`
and `doc-service`. Update `mise.local.toml` to define only the new
name. `python/pdf-parser`'s existing fallback chain (`STAGING_DIR` /
`PDF_STAGING_DIR` / `DATA_STAGING_DIR`) gets `UPLOAD_FILE_STAGING_DIR` added
as its first-checked name, old names left as fallback for any other
deployment still setting them — that service wasn't part of this proposal's
scope and its fallback chain already tolerates multiple names, so this is
additive, not a rewrite.

**4. Claiming reuses `insertUploadedInputRecord` directly, then renames the
file on disk — no new ingestion function.** For each selected pending file,
the claim endpoint, per file:
  1. re-derives `type` from the stripped filename's extension (same
     extension→type mapping the frontend already uses for multi-file
     uploads, ported to Go),
  2. opens a DB transaction and calls the existing
     `insertUploadedInputRecord` with `StagingAbsPath` set to the
     **post-strip** path (i.e. the row references a file that does not yet
     exist under that name),
  3. attempts `os.Rename(pendingPath, strippedPath)` — if it fails (file
     already claimed by someone else, or gone), rolls back the transaction
     and reports a per-file error; if it succeeds, commits.

  Doing the DB insert *before* the rename (rather than after) closes the
  race where `doc-service`'s next staging cycle could see the renamed file
  before a matching row exists — with the row inserted first, the file only
  becomes visible under its final name once the row already exists to match
  against, exactly mirroring how `UploadInputs` behaves within the same
  transaction. If the rename fails, the transaction is rolled back, so no
  orphan row is left behind. Chosen over a bare rename with no DB insert
  (would recreate the unattributed-row bug this change exists to close, per
  Context above) and over a full parallel backup/home-copy implementation
  (unnecessary — `doc-service` already does that for any row it can
  match, which this claim intentionally sets up).

**5. Admin-only, enforced server-side.** Both new endpoints check the
caller's role via the same pattern already used elsewhere in `kbhandler`
(`keyword_handlers.go`'s `requireKeywordRewriteAdmin`: `IsOwner`/`Admin`/
`roles contains "admin"|"root"`) and return 403 for non-admins, independent
of frontend visibility. The frontend shows the "Pending Files" button only
after a successful call to the list endpoint on mount (no separate
"am I admin" endpoint invented for this — reuses the real endpoint, fails
closed to hidden on 401/403).

**6. Staleness filter: one two-sample size check per list request, not per
file.** The list endpoint reads all candidate sizes once, sleeps a short
fixed interval, re-reads once, and keeps only files whose size didn't
change — a single fixed delay per request regardless of candidate count,
rather than per-file waits.

## Risks / Trade-offs

- **[Risk]** Shared staging directory means any admin can see and claim any
  other admin's pending file, misattributing its `tenant_id` to whichever
  admin claims it first. → **Mitigation**: accepted for this release per the
  trust model (machine access is already restricted to internal
  developers/admins); documented explicitly in the proposal.
- **[Risk]** Two admins claim the same file concurrently. → **Mitigation**:
  DB-insert-then-rename means only one claim's rename can succeed; the loser
  rolls back cleanly and reports "no longer available" for that file without
  affecting the rest of its batch.
- **[Risk]** Size-stability check adds latency to listing and could still
  race a very slow/resumed copy. → **Mitigation**: acceptable for an
  admin-only, low-frequency workflow; a truncated file that slips through is
  caught downstream by existing parse/error handling, same as today.
- **[Risk]** Renaming `python/pdf-parser`'s env var fallback list touches a
  service outside this change's original scope. → **Mitigation**: purely
  additive (new name added with top priority, nothing removed), so existing
  deployments setting the old names keep working unchanged.
- **[Realized]** The `.pending` filter was first added to
  `server/cmd/service-pdf-parser` — a plausible-sounding name found via
  `ls server/cmd/` while hunting for the docs' `pdf-parser` — without
  checking whether it was actually wired into any `mise` task. It wasn't;
  `server/cmd/doc-service` is the real staging service (also visible in the
  same `ls` listing, and not specifically ruled out — just not the name
  being searched for) and got no fix at all, so real `.pending` files kept
  being ingested unattributed until this was caught by manual testing and
  fixed in a follow-up pass. → **Lesson, not a mitigation**: before trusting
  an investigation into "which service does X," confirm the candidate is
  actually reachable from a `mise dev*`/`mise *-start` task, not just that
  its name and code plausibly fit.

## Migration Plan

No data migration. Deploy order:
1. Env var consolidation (`UPLOAD_FILE_STAGING_DIR`) across
   `upload_handler.go`, `mise.local.toml`, and the `python/pdf-parser`
   fallback chain — safe on its own since it's a rename of what was already
   the same directory (`doc-service` needed no change here; it already read
   `UPLOAD_FILE_STAGING_DIR`).
2. `.pending` filter in `doc-service`'s staging loop (safe no-op
   until `.pending` names are actually used).
3. Backend list/claim endpoints and admin-role gate.
4. Frontend Pending Files button and dialog.

Rollback is a straightforward revert at any stage; no persisted state format
changes.

## Open Questions

None blocking.
