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
The system SHALL store the current page in `kb.frontend_pages`. A new page starts at version 1.
On a subsequent save the system SHALL copy the current record to `archive.frontend_pages`,
update the current record, and increment its version, in one transaction. The page in use SHALL
be the highest `version` whose `status` is `active`. `PUT /api/v1/wpi/descriptors/page/:descriptorId` SHALL write pages to this table.
`kb.wpi_descriptors` SHALL NOT hold pages.

#### Scenario: Page written through the descriptor endpoint
- **WHEN** an admin PUTs a valid page `customers` whose latest version in `kb.frontend_pages` is 2
- **THEN** version 2 is archived, version 3 is stored in `kb.frontend_pages` and nothing is written to `kb.wpi_descriptors`

### Requirement: Editor auto-saving
The editor SHALL save each dirty page as a recovery record in `archive.frontend_pages` every N
seconds since its last save or auto-save, defaulting to 10 seconds, when the page descriptor
has changed since its last successful recovery save. An unchanged recovery SHALL NOT be
rewritten; edits made during a recovery write SHALL remain eligible for a later write.
Recovery records SHALL be
private to the author, SHALL NOT create a page version, and SHALL NOT mark the page clean.
When loading a page with a recovery record the editor SHALL load that record and mark the page
dirty. A successful ordinary save SHALL remove that author's recovery record atomically with
the page save. Unnamed pages SHALL receive temporary recovery IDs in the form
`unnamed-yyyymmdd-hhmmss` using local time and be restored as dirty tabs
when the editor opens. A recovery write based on an obsolete page version SHALL be rejected.
When unnamed recoveries are restored at startup, the editor SHALL omit the default empty
Unnamed tab; an existing tab containing a page or edits SHALL be preserved.

#### Scenario: Idle after recovery
- **WHEN** a recovery save succeeds and the page descriptor does not change
- **THEN** subsequent timer checks do not write another recovery
- **AND** the page remains dirty until an ordinary save

#### Scenario: Recover edits
- **WHEN** an author loads a page with their recovery record
- **THEN** the editor displays the recovery copy and marks the tab dirty

#### Scenario: Saving removes recovery
- **WHEN** an author successfully saves a page with a recovery record
- **THEN** the recovery record is removed and the previous page version is archived

#### Scenario: Existing pages are moved
- **WHEN** the migration runs on a database whose `kb.wpi_descriptors` holds page rows
- **THEN** every version of every page is in `kb.frontend_pages` with its status and audit columns, and `kb.wpi_descriptors` has no page rows

### Requirement: Page attributes
A page descriptor SHALL require `descriptor_id`, `schema_version`, `layouter` and `elements`,
and MAY set `page_name`, `description`, `access_controls`, `keywords`, `page_tags`,
`page_policy`, `page_style`, `input`, `save_action`, `refresh`, `binding_mode`,
`layouter_config`, `hori_align`, `vert_align`, `bkgd_color`, `frgd_color`, the sizes `width`,
`height`, `min_width`, `min_height`, `max_width` and `max_height`, and the margins
`left_margin`, `right_margin`, `top_margin` and `bottom_margin`. The validator SHALL check:

- `page_name` (at most 100 characters) and `description` (at most 1000) are text attributes
  (a non-empty string, or non-empty strings keyed by language code including `en`);
- `access_controls`, `keywords` and `page_tags` are arrays of non-empty strings; `keywords`
  and `page_tags` hold at most 200 items;
- `page_policy` and `page_style` are strings and are otherwise ignored;
- `hori_align` is `left`, `center` or `right`; `vert_align` is `top`, `center` or `bottom`;
- a size or margin is a non-negative number with an optional unit `px`, `rem`, `em` or `%`
  (`px` when omitted; spaces may precede the unit), a `%` value is within 0 to 100, and
  `width` and `height` also accept `auto`;
- `bkgd_color` and `frgd_color` are `auto` or a CSS color;
- `binding_mode` is `read-only` or `read-write`.

The page's status is the `status` column of `kb.frontend_pages`; a `status` attribute, the
former `page_alignment` and any other attribute SHALL be an error.

#### Scenario: Named and described page
- **WHEN** an admin PUTs a page with `"page_name": {"en": "Customers", "zh-cn": "客户"}` and `"description": "All customers"`
- **THEN** the page is stored

#### Scenario: Elastic page width
- **WHEN** a page sets `"width": "elastic"`
- **THEN** writing the page fails with an error at `/width`

#### Scenario: Unknown alignment
- **WHEN** a page sets `"hori_align": "justify"`
- **THEN** writing the page fails with an error at `/hori_align`

#### Scenario: Former attribute name
- **WHEN** a page sets `"page_alignment": "center"`
- **THEN** writing the page fails with an error at `/page_alignment`

#### Scenario: Length without a unit
- **WHEN** a page sets `"left_margin": "30"` and `"width": "50 %"`
- **THEN** the page is stored and the margin is 30px

### Requirement: Page access
A page whose `access_controls` is absent or empty SHALL be public to every logged-in user.
Otherwise only a user holding one of the listed roles (an admin holds `admin`) SHALL read the
page, receive it in an app bundle, or run an action written in it.

#### Scenario: Restricted page
- **WHEN** a page sets `"access_controls": ["admin"]` and a user without that role loads an app that opens it
- **THEN** the app bundle omits the page and reading or running it is refused with 403

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
`GET /api/v1/wpi/pages?q=<text>&limit=<n>&offset=<n>` SHALL return, for the latest active version of
each page, `descriptor_id`, `version`, `page_name`, `description`, `created_at` and `updated_at`, keeping
pages whose ID, name or description (in any language) contains `q`, ignoring case, ordered by
ID; `limit` SHALL default to 50 and be at most 200. `offset` SHALL default to zero
and be a non-negative integer. The response SHALL contain `pages` and `has_more`.
Optional `descriptor_id` and `description` SHALL filter by case-insensitive substring;
all filters SHALL be combined with AND and applied after selecting the latest active
version. `created_from` and `updated_from` SHALL be inclusive RFC3339 timestamps;
`created_to` and `updated_to` SHALL be exclusive RFC3339 timestamps. Invalid bounds
or pagination parameters SHALL return 400. Filtering and pagination SHALL run in the
database. `GET /api/v1/wpi/pages/:pageId` SHALL
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


### Requirement: Load a page into the editor
The WPI editor's Load Page button SHALL open a modal listing the latest active version
of each page, 20 per page, with descriptor ID, localized name and description, created
and updated timestamps. Authors SHALL be able to filter by descriptor ID, description,
and inclusive local calendar date ranges for creation and update, and navigate pages.
Loading SHALL fetch the full descriptor before replacing the canvas with one editable
node per element instance, preserving order, repeated classes and all attributes.
The descriptor SHALL stay in memory and View JSON SHALL retain its page metadata.
For an element, View JSON SHALL display the current editor instance ID; that ID is
session-local and SHALL NOT be written into the page descriptor.
A nonempty canvas SHALL require confirmation before replacement; undo SHALL restore
the prior canvas and page metadata. Failed requests SHALL leave the canvas unchanged
and display a translated error. The dialog SHALL support English and Chinese,
keyboard focus containment, Escape dismissal and text selection.

### Requirement: Canvas page preview
When a page is loaded, the editor SHALL render its elements with the interpreter's
layout and element components inside a resizable page surface on the canvas, initially centered and scaled to fit. Each
rendered element SHALL have a transparent selection overlay that selects its editor
node and opens its properties without triggering its page action. The preview SHALL
use the page's stored data sources and preserve element instance attributes when
properties change. When flow storage supports persistence, the flow data carries the
loaded page alongside its editable elements.

### Requirement: Interactive page preview
The editor toolbar SHALL offer Preview for the selected tab, switching the canvas to
an interactive WPI runtime and providing an Edit control to return. Preview SHALL use
the current page, whether named or unnamed, and load the pages it opens. A draft
preview SHALL NOT save the page; input and save actions tied to the draft page SHALL
report that saving is required. Validation failures SHALL be reported to the author.
An element issue SHALL identify its pane, its one-based position and zero-based index,
and its current editor instance ID when the element can be resolved. The editor SHALL
convert a bare nonnegative numeric value to pixels for element length attributes and
flow-layout length controls, including existing element values sent in a draft preview.
Switching tabs SHALL return to editing.

### Requirement: Page and layouter controls
When a page is loaded, the editor SHALL show a movable control frame at the upper
right of the canvas. Its page configuration and layouter buttons SHALL display the
corresponding descriptor properties in the Properties panel. Selecting an element
SHALL return the panel to that element. Edits SHALL update the in-memory descriptor,
refresh the preview and be undoable. The frame SHALL be movable by pointer and keyboard and initially aligned immediately left of the canvas settings button. The preview SHALL be resizable by pointer and keyboard; resizing changes the editor surface rather than page descriptor dimensions, and SHALL be undoable.

### Requirement: First save of a new page

When an unnamed page is saved in the WPI editor, the editor SHALL ask for a page name,
description, keywords, tags and a descriptor ID. It SHALL require a non-blank name and a valid
descriptor ID, accept comma-separated keywords and tags, and create a page with the current
canvas contents. If the ID already exists, it SHALL ask for confirmation before saving a new
version under that ID. Declining SHALL keep the entered information. Subsequent saves SHALL
create versions of that page. Cancelling or a failed save
SHALL leave the page unsaved. Save errors SHALL remain visible until the author dismisses
them, and their text SHALL be selectable.

#### Scenario: Create a page on first save
- **WHEN** an author saves an unnamed page and submits its information
- **THEN** the editor stores a new page with that information and its current elements
- **AND** subsequent saves update that page

### Pane and panel colors

Panes and panels MAY specify `background_color` and `foreground_color` as a CSS color or an object containing `light` and `dark` CSS colors. Missing or null means unset. The active display mode selects the corresponding color. An unset pane color inherits the closest ancestor pane's corresponding color independently. A pane's effective background, including an inherited one, overrides its own panel background; a panel may override the inherited foreground. A host pane replaces the root pane of a page opened inside it.
