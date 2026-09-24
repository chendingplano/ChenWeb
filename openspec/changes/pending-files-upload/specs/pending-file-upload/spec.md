## ADDED Requirements

### Requirement: Staging watcher ignores `.pending` files
The staging directory poller (`server/cmd/service-pdf-parser`) SHALL NOT
detect, move, back up, or otherwise process any file in the staging
directory (`UPLOAD_FILE_STAGING_DIR`) whose name ends in `.pending`,
regardless of how long the file remains there.

#### Scenario: File with .pending suffix is left untouched
- **WHEN** a file named `report.pdf.pending` is placed in
  `UPLOAD_FILE_STAGING_DIR`
- **THEN** the staging poller does not copy it to the backup or home
  directory, does not remove it from staging, and does not create or update
  any `kb.inputs` record for it

### Requirement: Pending Files access is admin-only
The endpoints that list pending files and ingest selected pending files
SHALL verify the authenticated caller has the admin role before performing
any action, independent of what the frontend displays.

#### Scenario: Non-admin calls the pending-files endpoints directly
- **WHEN** an authenticated non-admin user calls the list-pending-files or
  claim-pending-files endpoint
- **THEN** the backend returns an HTTP 403 response and performs no listing
  or ingestion

#### Scenario: Non-admin views the File Management page
- **WHEN** a non-admin user opens Knowledge → File Management
- **THEN** the "Pending Files" button is not shown

### Requirement: Pending Files listing excludes in-flight copies
Listing pending files SHALL only include `.pending`-suffixed files in
`UPLOAD_FILE_STAGING_DIR` whose size has remained unchanged across two reads
separated by a short interval; files still being written SHALL be excluded
from the list.

#### Scenario: Listing while a large file is still being copied in
- **WHEN** an admin opens the Pending Files dialog while `bigfile.pdf.pending`
  is still being written to `UPLOAD_FILE_STAGING_DIR` and its size is changing
- **THEN** `bigfile.pdf.pending` does not appear in the pending files list

#### Scenario: Listing after a copy has completed
- **WHEN** an admin opens the Pending Files dialog and `report.pdf.pending`
  has had a stable size for at least the required interval
- **THEN** `report.pdf.pending` appears in the list with its filename
  (`.pending` suffix shown or stripped for display) and age/mtime

#### Scenario: No pending files present
- **WHEN** an admin clicks "Pending Files" and `UPLOAD_FILE_STAGING_DIR` contains no
  eligible `.pending` files
- **THEN** the UI informs the admin that there are no pending files instead
  of opening an empty selection dialog

### Requirement: Claiming a pending file ingests it via the authenticated upload path
Claiming one or more selected pending files SHALL create a `kb.inputs` row
for each, via the same insert logic the authenticated browser upload uses
(`insertUploadedInputRecord`), using the claiming admin's current session /
active knowledge store to set `tenant_id`, and using the selected processing
mode (Auto / Upload Files Only / PDF Parsing) exactly as the normal upload UI
does — then rename the file on disk to strip `.pending`, so it becomes a
normal staged file that the existing staging poller
(`server/cmd/service-pdf-parser`) picks up and completes exactly as it would
a normal upload.

#### Scenario: Admin claims a single PDF pending file
- **WHEN** an admin selects `report.pdf.pending` in the Pending Files dialog,
  leaves mode as Auto, and clicks "Upload Files"
- **THEN** the backend creates a `kb.inputs` row for `report.pdf` with
  `tenant_id` set from the admin's active knowledge store (the same row
  shape a browser upload of `report.pdf` would produce), renames
  `report.pdf.pending` to `report.pdf` in the staging directory, and the
  existing staging poller subsequently backs it up, copies it to the home
  directory, and — since mode is Auto — proceeds through PDF parsing

#### Scenario: Admin claims a pending zip file
- **WHEN** an admin selects `archive.zip.pending` and clicks "Upload Files"
- **THEN** the backend creates a single `type='zip'` `kb.inputs` row for
  `archive.zip` and renames the file, identical in shape to what a browser
  upload of a `.zip` file produces today (no per-entry child records — this
  codebase does not extract zip contents at ingestion time)

#### Scenario: Successful claim renames the pending file for the poller to pick up
- **WHEN** a pending file's `kb.inputs` row is successfully inserted
- **THEN** the original `<name>.pending` file is renamed to `<name>` in the
  staging directory within the same transaction as the insert (rolled back
  together if the rename fails), so the existing staging poller finds and
  completes it without any pending-specific handling

### Requirement: Claim failure on a file already claimed by another admin
If a selected pending file no longer exists in `UPLOAD_FILE_STAGING_DIR` at the
moment the backend attempts to ingest it (e.g. another admin already
claimed it), the system SHALL report a clear per-file error for that file
and SHALL NOT fail the entire batch or crash; the dialog SHALL refresh its
listing to reflect current state.

#### Scenario: Two admins select the same pending file concurrently
- **WHEN** admin A and admin B both select `report.pdf.pending` and admin A's
  claim completes first
- **THEN** admin B's claim for `report.pdf.pending` fails with a clear
  "no longer available" error for that file, any other files in admin B's
  selection are still processed normally, and the Pending Files list
  refreshes to no longer show `report.pdf.pending`
