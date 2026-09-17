## ADDED Requirements

### Requirement: Update video metadata endpoint
The system SHALL expose an authenticated `PATCH /api/v1/videos/:id` endpoint that updates the metadata columns of a `kb.videos` row (`name`, `description`, `source`, `url`, `image_id`, `keywords`, `category`, `subcategory`, `container`, `status`, `notes`, `video_type`). It SHALL NOT modify `uploaded_by` or `created_at`. A video file is OPTIONAL on this request.

The endpoint SHALL apply the same field validation as video upload: `source` MUST be `Recording` or `Web`; when `source` is `Web`, `url` MUST be non-empty; `status` MUST be one of `draft`, `published`, `archived`.

#### Scenario: Successful metadata update, file unchanged
- **WHEN** an authenticated admin sends `PATCH /api/v1/videos/:id` with valid form fields and no file, for an existing video
- **THEN** the system updates the corresponding `kb.videos` row's metadata columns and returns the updated video metadata as JSON, with `filename`/`stored_path`/`size_bytes`/`content_type` unchanged

#### Scenario: Unknown video id
- **WHEN** `PATCH /api/v1/videos/:id` is sent for an id with no matching row
- **THEN** the system returns a 404 with an error response and makes no changes

#### Scenario: Invalid source/url/status combination
- **WHEN** the request sets `source=Web` without a `url`, or an unrecognized `source` or `status` value
- **THEN** the system returns a 400 with an error response and makes no changes, and any file part in the request is not saved

### Requirement: Replace video file on update
When the `PATCH /api/v1/videos/:id` request includes a `file` part, the system SHALL validate it with the same content-type/extension allowlist and size cap as video upload, store it under a fresh unique path, update the row's `filename`, `stored_path`, `size_bytes`, and `content_type` together with any metadata fields in the same request, and remove the previously stored file from disk only after the database update succeeds.

#### Scenario: Successful file replacement
- **WHEN** an authenticated admin sends `PATCH /api/v1/videos/:id` with a valid replacement video file
- **THEN** the system stores the new file, updates the row's file-related columns to describe it, returns the updated metadata, and the previous file is removed from disk

#### Scenario: Invalid replacement file
- **WHEN** the `file` part fails the type or size checks
- **THEN** the system returns a 400 with an error response, does not modify the `kb.videos` row, and does not leave the rejected file on disk

#### Scenario: Database update fails after a new file was saved
- **WHEN** the row update fails after a valid replacement file has already been written to disk
- **THEN** the system removes the newly-saved file, leaves the existing row and its original file untouched, and returns an error response

### Requirement: Edit action in the video manager
The video manager table SHALL show an **Edit** action alongside View, Download, and Delete for each row. Selecting Edit SHALL open the upload dialog in edit mode, pre-filled with that video's current metadata (name, description, source, url, keywords, category, subcategory, container, status, notes, video_type, cover image), with an optional file picker to replace the stored video. Submitting SHALL call the update endpoint and, on success, refresh the table and close the dialog.

#### Scenario: Editing a video's metadata only
- **WHEN** an admin clicks Edit on a video row, changes the Name and Status fields without choosing a file, and submits
- **THEN** the dialog closes, the table reflects the new Name and Status for that video, and the video's file/size/uploaded-by/uploaded-at are unchanged

#### Scenario: Replacing a video's file while editing
- **WHEN** an admin clicks Edit on a video row, chooses a new video file, and submits
- **THEN** the dialog closes, the table reflects the new file's size, and viewing/downloading the video serves the new file

#### Scenario: Canceling an edit
- **WHEN** an admin opens Edit on a video row and closes the dialog without submitting
- **THEN** no request is sent and the video's metadata and file are unchanged
