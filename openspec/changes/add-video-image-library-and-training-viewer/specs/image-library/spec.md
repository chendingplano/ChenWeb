## ADDED Requirements

### Requirement: Image metadata store
The system SHALL persist image metadata in a `kb.images` table created via a goose migration, with at least: primary key id, original filename, stored file path, byte size, content type, an origin marker (uploaded vs generated), an optional generation prompt, uploader identity, and creation timestamp.

#### Scenario: Table created on migration
- **WHEN** migrations run
- **THEN** `kb.images` exists with the columns above (reserved keywords protected)

### Requirement: Image upload to library
An authenticated user SHALL be able to upload an image into the library. The bytes are stored on the filesystem under the configured image directory and a `kb.images` row is inserted.

#### Scenario: Successful image upload
- **WHEN** an authenticated user uploads a valid image
- **THEN** the bytes are written under the image directory, a `kb.images` row is created (origin = uploaded), and the image appears in the library listing

#### Scenario: Rejected non-image
- **WHEN** the uploaded file is not an accepted image type
- **THEN** the request is rejected with a clear error and no row or file remains

### Requirement: Image listing and serving
The system SHALL list library images (newest first) with their metadata and SHALL serve an image's bytes by id with its stored content type.

#### Scenario: List and view
- **WHEN** the picker opens
- **THEN** it shows the library images with previews served from the by-id content endpoint

### Requirement: Authenticated access
All image endpoints (upload, list, serve, generate) SHALL require authentication.

#### Scenario: Unauthenticated request rejected
- **WHEN** an unauthenticated request hits any `/api/v1/images/...` endpoint
- **THEN** it is rejected with an unauthorized response
