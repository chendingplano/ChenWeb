## ADDED Requirements

### Requirement: Page image attribute
A page descriptor MAY set `page_image`, the file name of the page's image in
`DATA_HOME_DIR/page-images`. The validator SHALL require it to be a string matching
`^[A-Za-z0-9][A-Za-z0-9._-]{0,160}\.png$` and SHALL NOT check that the file exists.

#### Scenario: Valid image name
- **WHEN** a page sets `"page_image": "customers-20261105120000-a1b2.png"`
- **THEN** the page is stored

#### Scenario: Path in the image name
- **WHEN** a page sets `"page_image": "../x.png"`
- **THEN** writing the page fails with an error at `/page_image`
