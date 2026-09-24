## Context

Corrected after reading the actual code (the initial version of this design
was based on the `KnowledgeStore` process docs, which describe an idealized
pipeline — including per-entry zip extraction and a single unified "ingest
function" — that does not exist in this codebase today):

- `kbhandler.UploadInputs` (`server/api/kbhandler/upload_handler.go`) writes
  the uploaded file into a staging directory (env var `STAGING_DIR`) and, in
  the same DB transaction, inserts a `kb.inputs` row via
  `insertUploadedInputRecord` with `tenant_id`/`ks_store_id`/`type`/
  `processing_mode`/etc. and `file_name` set to the staged path,
  `backup_filename` left empty. It does **not** copy the file to backup/home
  itself, and there is **no zip child-record extraction** anywhere in the Go
  code — a `.zip` upload today is one opaque `type='zip'` row, nothing more.
- Independently, the `service-pdf-parser` binary
  (`server/cmd/service-pdf-parser/main.go`) polls its own staging directory
  (env var `DATA_STAGING_DIR`) every 10s. For each file it finds, it copies
  it to `DATA_BACKUP_DIR` and `DATA_HOME_DIR`, deletes it from staging, and
  either **updates** the existing `kb.inputs` row where
  `file_name = <staged path> AND backup_filename = ''` (the row `UploadInputs`
  already inserted) or, if no such row exists, **inserts** a brand-new one
  with no `tenant_id` and raises `AlarmMissingTenantIDAtInsert`.
- In this repo's `mise.local.toml`, `STAGING_DIR` and `DATA_STAGING_DIR` are
  set to the same path — two env var names for what is meant to be one
  directory. This was confirmed to be an accidental duplication, not an
  intentional two-directory design; part of this change consolidates them
  into a single `UPLOAD_FILE_STAGING_DIR` var (see Decision 3).

As of the 2026-09-24 doc-processing attribution fix
(`KnowledgeStore/doc-repo/devdocs/202609/2026092403-devdoc-doc-processing-user-id-attribution.md`),
a `kb.inputs` row with an empty/default `tenant_id` now causes doc-processing
to refuse to run. A file copied directly into the staging directory outside
`UploadInputs` never gets a matching pre-existing row, so `service-pdf-parser`
inserts it unattributed and it hits that refusal. This capability closes that
gap: keep such files out of the pipeline (`.pending` suffix) until an
authenticated admin claims them, and have the claim itself do exactly what
`UploadInputs` does — insert the same kind of pre-existing row with a real
`tenant_id` — before the file becomes visible under its real name. The
existing `service-pdf-parser` polling loop then completes it exactly as it
would a normal upload, unmodified.

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
- Zip child-record extraction. Not implemented anywhere today; a pending
  `.zip` file is claimed the same way any other pending file is — one
  `kb.inputs` row, matching current upload behavior. Out of scope to add.
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

**2. `service-pdf-parser`'s polling loop ignores `.pending` files outright**
(skipped before `entry.Info()`/copy/insert). One filter check at the top of
the per-entry loop in `processStagingOnce`. Guarantees a `.pending` file is
never picked up automatically, however long it sits there.

**3. Consolidate `STAGING_DIR` and `DATA_STAGING_DIR` into one
`UPLOAD_FILE_STAGING_DIR` env var**, read by both `kbhandler.UploadInputs`
and `service-pdf-parser`. Update `mise.local.toml` to define only the new
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
  race where `service-pdf-parser`'s next poll could see the renamed file
  before a matching row exists — with the row inserted first, the file only
  becomes visible under its final name once the row already exists to match
  against, exactly mirroring how `UploadInputs` behaves within the same
  transaction. If the rename fails, the transaction is rolled back, so no
  orphan row is left behind. Chosen over a bare rename with no DB insert
  (would recreate the unattributed-row bug this change exists to close, per
  Context above) and over a full parallel backup/home-copy implementation
  (unnecessary — `service-pdf-parser` already does that for any row it can
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

## Migration Plan

No data migration. Deploy order:
1. Env var consolidation (`UPLOAD_FILE_STAGING_DIR`) across
   `upload_handler.go`, `service-pdf-parser/main.go`, `mise.local.toml`, and
   the `python/pdf-parser` fallback chain — safe on its own since it's a
   rename of what was already the same directory.
2. `.pending` filter in `service-pdf-parser`'s polling loop (safe no-op
   until `.pending` names are actually used).
3. Backend list/claim endpoints and admin-role gate.
4. Frontend Pending Files button and dialog.

Rollback is a straightforward revert at any stage; no persisted state format
changes.

## Open Questions

None blocking.
