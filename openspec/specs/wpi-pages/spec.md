# wpi-pages Specification

## Purpose
WPI pages, which have no classes: a page is complete in itself and reused by copying. Covers
the page attributes, storage in `kb.frontend_pages`, the page's size and alignment in its
pane, the page-wide `binding_mode`, static data as input, and the endpoints authors use to
search pages and read one to copy.
## Requirements
### Requirement: Pages have no classes
A page SHALL be a complete descriptor with an ID chosen by its author. There SHALL be no page
classes: pages are reused by copying a stored page and saving the copy under a new ID. Apps
and actions SHALL refer to a page by its `descriptor_id`.

#### Scenario: Copy a page under a new ID
- **WHEN** an admin reads page `customer-form`, changes it and PUTs it as `vip-customer-form`
- **THEN** `vip-customer-form` version 1 is stored and `customer-form` is unchanged

### Requirement: Page storage
The system SHALL store pages in `kb.frontend_pages`, one row per version, identified by
`(descriptor_id, version)`. The page in use SHALL be the highest `version` whose `status` is
`active`. `PUT /api/v1/wpi/descriptors/page/:descriptorId` SHALL write pages to this table.
`kb.wpi_descriptors` SHALL NOT hold pages.

#### Scenario: Page written through the descriptor endpoint
- **WHEN** an admin PUTs a valid page `customers` whose latest version in `kb.frontend_pages` is 2
- **THEN** version 3 is stored in `kb.frontend_pages` and nothing is written to `kb.wpi_descriptors`

#### Scenario: Existing pages are moved
- **WHEN** the migration runs on a database whose `kb.wpi_descriptors` holds page rows
- **THEN** every version of every page is in `kb.frontend_pages` with its status and audit columns, and `kb.wpi_descriptors` has no page rows

### Requirement: Page attributes
A page descriptor SHALL require `descriptor_id`, `schema_version`, `layouter` and `elements`,
and MAY set `page_name`, `description`, `input`, `save_action`, `refresh`,
`page_alignment`, `width`, `height`, `min_width`, `max_width` and `binding_mode`. The
validator SHALL check: `page_name` and `description` are text attributes (a non-empty string,
or non-empty strings keyed by language code including `en`); `page_alignment` is `left`,
`center` or `right`; the four sizes are `auto` or a length in `px`, `rem`, `em` or `%`;
`binding_mode` is `read-only` or `read-write`. Any other attribute SHALL be an error.

#### Scenario: Named and described page
- **WHEN** an admin PUTs a page with `"page_name": {"en": "Customers", "zh-cn": "客户"}` and `"description": "All customers"`
- **THEN** the page is stored

#### Scenario: Elastic page width
- **WHEN** a page sets `"width": "elastic"`
- **THEN** writing the page fails with an error at `/width`

#### Scenario: Unknown alignment
- **WHEN** a page sets `"page_alignment": "justify"`
- **THEN** writing the page fails with an error at `/page_alignment`

### Requirement: Page size and alignment
The interpreter SHALL draw a page inside its pane with the page's `width`, `height`,
`min_width` and `max_width`. Without `width` the page SHALL fill the pane's width; without
`height` it SHALL be as tall as its content. When the page is narrower than the pane,
`page_alignment` `center` SHALL centre it and `right` SHALL align it to the right; `left` is
the default.

#### Scenario: Centred form
- **WHEN** a page sets `"max_width": "640px"` and `"page_alignment": "center"` in a pane 1200px wide
- **THEN** the page is 640px wide with equal space on both sides

### Requirement: Page-wide binding mode
A page's `binding_mode` SHALL be the default for every element on it whose class defines
`binding_mode`. An element instance's own `binding_mode` SHALL come first, then the page's,
then the element class's default. A class row whose `binding_mode` is `null` SHALL be
satisfied by the page's value.

#### Scenario: Read-only page
- **WHEN** a page with `"binding_mode": "read-only"` has an `input` instance that does not set `binding_mode`, and the `input` class defaults it to `read-write`
- **THEN** the input is read-only

#### Scenario: Instance overrides the page
- **WHEN** the same page has an `input` instance with `"binding_mode": "read-write"`
- **THEN** that input is editable

### Requirement: Static input
An object without an `action` attribute SHALL be the input doc itself when it is a page's
`input` or an `open-page`'s `input`; `{}` is the empty doc. A panel opened with static data SHALL
start with a copy of it as its input doc and as its clean copy, SHALL be ready without a
server call, and SHALL NOT reload it on refresh.

#### Scenario: Header with static data
- **WHEN** page `app-header` has `"input": {"title": "Customer admin"}` and a `text` element bound to `$input_doc.title`
- **THEN** the header shows "Customer admin" without running an action

#### Scenario: Editing static data
- **WHEN** a read-write element changes a value of a static input doc
- **THEN** the panel is dirty, and the page's stored descriptor is unchanged

### Requirement: Page search and read
`GET /api/v1/wpi/pages?q=<text>&limit=<n>` SHALL return, for the latest active version of
each page, `descriptor_id`, `version`, `page_name`, `description` and `updated_at`, keeping
pages whose ID, name or description (in any language) contains `q`, ignoring case, ordered by
ID; `limit` SHALL default to 50 and be at most 200. `GET /api/v1/wpi/pages/:pageId` SHALL
return `{"version": n, "page": <descriptor>}` for the latest active version. Both SHALL
require a logged-in user.

#### Scenario: Search by a Chinese name
- **WHEN** a logged-in user searches `q=客户` and page `customers` has `"page_name": {"en": "Customers", "zh-cn": "客户"}`
- **THEN** `customers` is in the result

#### Scenario: Read a page to copy it
- **WHEN** a logged-in user requests `/api/v1/wpi/pages/customer-form`
- **THEN** the response holds the latest active version number and its descriptor

#### Scenario: Unknown page
- **WHEN** a logged-in user requests a page ID with no active version
- **THEN** the response is 404

#### Scenario: Not logged in
- **WHEN** the request has no valid session
- **THEN** the response is 401

