## ADDED Requirements

### Requirement: Page image upload
The server SHALL accept `PUT /api/v1/wpi/page-images/:pageId` from admins with an `image/png`
body of at most 4 MiB, write it under `DATA_HOME_DIR/page-images/` with a server-generated name
`<pageId>-<timestamp>-<suffix>.png`, and return `{"page_image": "<name>"}`. A body that is not a
PNG SHALL be refused with 400, a non-admin with 403, an oversize body with 413, and an unset
`DATA_HOME_DIR` with 503.

#### Scenario: Valid upload
- **WHEN** an admin PUTs a PNG for page `customers`
- **THEN** a file named `customers-…png` exists in `DATA_HOME_DIR/page-images` and its name is returned

#### Scenario: Not a PNG
- **WHEN** an admin PUTs a body that does not start with the PNG signature
- **THEN** the response is 400 and no file is written

#### Scenario: Not an admin
- **WHEN** a non-admin PUTs an image
- **THEN** the response is 403

### Requirement: Page image read
The server SHALL serve a stored image at `GET /api/v1/wpi/page-images/:name` to any logged-in
user, as `image/png`. A name that does not match the image name pattern or names no file SHALL
give 404; a name SHALL never resolve outside the page-images directory.

#### Scenario: Path traversal
- **WHEN** a client requests `..%2F..%2Fetc%2Fpasswd`
- **THEN** the response is 404

### Requirement: Image captured on save
When the WPI editor creates or saves a page, it SHALL capture a PNG of the page surface, upload
it, and store the returned name in the page root as `page_image` in the same save. If capture or
upload fails, the editor SHALL still save the page and SHALL tell the author the image was not made.

#### Scenario: Save with image
- **WHEN** an author saves a page in the editor
- **THEN** the stored page has a `page_image` naming an existing file

#### Scenario: Capture fails
- **WHEN** the capture throws during a save
- **THEN** the page is saved without a new image and a warning is shown
