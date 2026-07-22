## ADDED Requirements

### Requirement: Video metadata store
The system SHALL persist video metadata in a `kb.videos` table created via a goose migration, with at least: a primary key id, original filename, stored file path, byte size, content type, uploader identity, and creation timestamp.

#### Scenario: Table created on startup/migration
- **WHEN** migrations run
- **THEN** the `kb.videos` table exists with the columns above (reserved keywords protected)

### Requirement: Video upload
The Training page SHALL let an authenticated user upload a video file. On upload the system SHALL store the file bytes on the filesystem under the configured video storage directory and insert a corresponding `kb.videos` metadata row.

#### Scenario: Successful upload
- **WHEN** an authenticated user uploads a valid video file via the Training page
- **THEN** the bytes are written under the configured video directory, a `kb.videos` row is created capturing filename/size/content_type/uploader/created_at, and the new video appears in the list

#### Scenario: Rejected non-video / oversized upload
- **WHEN** an upload has a disallowed content type or exceeds the configured size limit
- **THEN** the server rejects it with a clear error and no metadata row or stored file is left behind

#### Scenario: Storage directory not configured
- **WHEN** the video storage directory env/config is unset
- **THEN** upload requests fail with a clear server error (fail closed), matching the existing `STAGING_DIR`-style handling

### Requirement: Video listing
The Training page SHALL display a list of stored videos with their metadata (filename, size, upload date, uploader).

#### Scenario: List reflects store
- **WHEN** the user opens the Training page
- **THEN** all videos currently in `kb.videos` are listed with their metadata, newest first

### Requirement: Video view (stream)
The system SHALL allow an authenticated user to view/play a stored video inline via a streaming endpoint that serves the file with its stored content type and supports HTTP range requests.

#### Scenario: Inline playback
- **WHEN** the user chooses to view a listed video
- **THEN** the video plays inline, served with the correct content type and range support for seeking

### Requirement: Video download
The system SHALL allow an authenticated user to download a stored video as a file attachment with its original filename.

#### Scenario: Download attachment
- **WHEN** the user chooses to download a listed video
- **THEN** the browser receives the file as an attachment named with the video's original filename

### Requirement: Video delete
The system SHALL allow an authenticated user to delete a stored video, removing both its `kb.videos` metadata row and its file bytes from the storage directory.

#### Scenario: Successful delete
- **WHEN** the user deletes a listed video and confirms
- **THEN** the metadata row is removed, the file is deleted from the storage directory, and the video no longer appears in the list

#### Scenario: Delete of missing file
- **WHEN** the metadata row exists but the underlying file is already gone
- **THEN** the delete still removes the metadata row and reports success (does not crash)

### Requirement: Authenticated access
All video endpoints (upload, list, view/stream, download, delete) SHALL require authentication, consistent with the auth-gated Resources page.

#### Scenario: Unauthenticated request rejected
- **WHEN** an unauthenticated request hits any `/api/v1/videos/...` endpoint
- **THEN** it is rejected with an unauthorized response and no video data is exposed or modified
