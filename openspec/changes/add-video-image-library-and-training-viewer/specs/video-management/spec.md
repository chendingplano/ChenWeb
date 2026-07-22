## ADDED Requirements

### Requirement: Manager location under System Admin
The video manager (upload/edit/delete) SHALL be reachable from the workspace nav tree under SYSTEM ADMIN → Resources → Videos, and SHALL NOT be hosted on the Resources page's Training entry.

#### Scenario: Manager under System Admin
- **WHEN** an authenticated user opens SYSTEM ADMIN → Resources → Videos (on `/home3` or `/development`)
- **THEN** the video manager renders with its upload and delete controls

#### Scenario: Training is no longer the manager
- **WHEN** the user opens Resources → Videos → Training
- **THEN** they see the read-only viewer, not the manager

### Requirement: Enriched video metadata on upload
Uploading a video SHALL capture `name`, `description`, `source` (one of `Recording`, `Web`), an optional `url`, and a cover `image_id`, in addition to the uploaded file. When `source = Web`, the dialog SHALL require a `url`. These fields SHALL be persisted on `kb.videos`.

#### Scenario: Upload with metadata
- **WHEN** the user submits the upload dialog with a file, name, description, source, and a selected cover image
- **THEN** the video is stored with those fields and the chosen `image_id`

#### Scenario: Web source requires url
- **WHEN** `source = Web` and no `url` is provided
- **THEN** the upload is rejected with a clear validation error

### Requirement: Cover image selection in the dialog
The upload dialog SHALL let the user set the cover image either by picking one from the image library (Pick an Image) or by generating one (Auto-Generate), where each Auto-Generate click selects a newly generated image.

#### Scenario: Pick from library
- **WHEN** the user clicks Pick an Image and chooses one
- **THEN** that image becomes the selected cover (its id is submitted as `image_id`)

#### Scenario: Auto-generate a cover
- **WHEN** the user clicks Auto-Generate
- **THEN** a new image is generated, saved to the library, and selected as the cover; clicking again replaces the selection with another new image

### Requirement: List/detail expose new fields
Video list and detail responses SHALL expose `name`, `description`, `source`, `url`, and a cover-image URL (resolved from `image_id`) so the viewer and manager can render them.

#### Scenario: Responses include cover + metadata
- **WHEN** the viewer or manager loads videos
- **THEN** each item includes its name, description, source, url, size, upload time, and a usable cover-image URL (or empty when none)
