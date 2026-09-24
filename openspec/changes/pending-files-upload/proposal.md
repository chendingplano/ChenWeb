## Why

Internal developers and admins sometimes need to get files into the Knowledge
Store without going through the browser file picker (e.g. large files, or
files already sitting on the server). Today the only ingestion path is the
web upload UI, which is what attaches an authenticated user's `tenant_id` to
the resulting `kb.inputs` record — required by the doc-processing pipeline's
user/tenant attribution backstop (see
`KnowledgeStore/doc-repo/devdocs/202609/2026092403-devdoc-doc-processing-user-id-attribution.md`).
A file dropped directly into `DATA_STAGING_DIR` has no such identity and,
under the current backstop, doc-processing now correctly refuses to run on
it — so direct filesystem copies are a dead end as they stand.

This change adds a two-step flow: files can be copied directly into staging
(marked `.pending` so the existing staging watcher ignores them), and an
admin then explicitly claims and ingests them through the authenticated web
UI, which attributes them correctly just like a normal upload.

## What Changes

- The staging watcher (`server/cmd/pdf-parser`) ignores any file in
  `DATA_STAGING_DIR` whose name ends in `.pending`.
- A new admin-only **Pending Files** button appears next to **Upload Files**
  on `ChenWeb/home3/knowledge` → File Management. Visibility is enforced
  server-side (role check on the backing endpoints), not just hidden in the
  UI.
- Clicking **Pending Files** opens a dialog listing files in
  `DATA_STAGING_DIR` ending in `.pending` (excluding files whose size hasn't
  been stable for a few seconds, to skip in-flight copies), each with a
  checkbox and its age/mtime shown.
- The admin selects one or more files, optionally picks the same
  Auto / Upload Files Only / PDF Parsing processing mode the normal upload UI
  offers, and clicks **Upload Files** in the dialog.
- That action ingests the selected files server-side through the same
  ingestion path the normal authenticated upload uses (so `tenant_id` is set
  from the admin's active knowledge store), stripping the `.pending` suffix
  when writing to `DATA_BACKUP_DIR` and `DATA_HOME_DIR/Artifacts`, and
  removing the original file from `DATA_STAGING_DIR` on success — matching
  existing staging-cleanup and zip parent/child behavior.
- If a selected file was already claimed/removed by someone else between
  listing and upload, the dialog surfaces a clear error and refreshes the
  list rather than failing silently.

## Capabilities

### New Capabilities
- `pending-file-upload`: admin-only claiming and ingestion of files placed
  directly in the staging directory with a `.pending` marker, reusing the
  authenticated upload ingestion path for correct tenant attribution.

### Modified Capabilities
- (none — no existing spec covers staging/upload behavior yet)

## Impact

- `server/cmd/pdf-parser`: staging watcher must skip `.pending`-suffixed
  files.
- `server/api/kbhandler/` (or wherever `UploadInputs`'s underlying ingest
  function lives): needs a variant/entry point callable from a
  staging-directory file path rather than an HTTP multipart body, reusing the
  same backup/artifact-write/tenant-attribution logic.
- New backend endpoint(s): list pending files, ingest selected pending files
  — both gated by admin role server-side.
- `ChenWeb/home3/knowledge` frontend: new **Pending Files** button (admin
  role-gated) and selection dialog next to the existing Upload Files UI.
- No breaking changes to existing upload behavior or `kb.inputs` schema.
