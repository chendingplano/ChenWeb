## ADDED Requirements

### Requirement: Config-driven menu visibility source
The system SHALL read a `[knowledge-menus]` table from `config.toml`, overridable by `config.local.toml`, as a flat mapping of Wiki sidebar menu item id (string) to enabled state (boolean). The system SHALL treat any menu item id not present in the table as enabled (`true`).

#### Scenario: Section absent
- **WHEN** neither `config.toml` nor `config.local.toml` defines a `[knowledge-menus]` section
- **THEN** every Wiki sidebar menu item and sub-item is shown, identical to current behavior

#### Scenario: Local override takes precedence
- **WHEN** `config.toml` sets `kb-metrics = true` under `[knowledge-menus]` and `config.local.toml` sets `kb-metrics = false` under the same section
- **THEN** the resolved configuration reports `kb-metrics` as disabled

#### Scenario: Unlisted id defaults to enabled
- **WHEN** `[knowledge-menus]` contains at least one entry but does not mention `kb-object-manager`
- **THEN** the resolved configuration reports `kb-object-manager` as enabled

### Requirement: Menu configuration API endpoint
The system SHALL expose `GET /api/v1/kb/menu-config`, returning the resolved `[knowledge-menus]` mapping as JSON. The endpoint SHALL return a successful response with an empty mapping when no `[knowledge-menus]` section is configured.

#### Scenario: Fetch with no config
- **WHEN** a client calls `GET /api/v1/kb/menu-config` and no `[knowledge-menus]` section exists
- **THEN** the response is HTTP 200 with a status of success and an empty (or all-absent) menu mapping

#### Scenario: Fetch with overrides configured
- **WHEN** a client calls `GET /api/v1/kb/menu-config` and `[knowledge-menus]` sets `kb-doc-wiki = false`
- **THEN** the response includes `"kb-doc-wiki": false`

### Requirement: Sidebar honors parent/child visibility hierarchy
The Wiki sidebar on `/home3/knowledge` SHALL filter its menu items against the resolved `[knowledge-menus]` mapping before rendering. Disabling a top-level item SHALL hide that item and all of its children. Disabling a child item SHALL hide only that child, leaving its parent and sibling items visible. A top-level item that originally had one or more children SHALL be hidden if, after filtering, none of its children remain visible.

#### Scenario: Disabling a top-level section hides its subtree
- **WHEN** `[knowledge-menus]` sets `kb-doc-wiki = false`
- **THEN** the "Wiki" top-level menu item and all of its children (LLM Wiki, LLM Wiki v3, Document Metadata, etc.) are not rendered in the sidebar

#### Scenario: Disabling a single child leaves siblings visible
- **WHEN** `[knowledge-menus]` sets `kb-metrics = false` and does not disable any other Wiki child
- **THEN** the "Metrics" item is not rendered under "Wiki", but "Wiki" itself and its other children remain visible

#### Scenario: Disabling every child collapses the parent
- **WHEN** `[knowledge-menus]` disables every child id under "Document Processing" (`kb-chunks`, `kb-category-review`) but does not explicitly disable `kb-chunks` as the parent id
- **THEN** the "Document Processing" top-level menu item is not rendered, since it would otherwise expand to an empty subtree

#### Scenario: No config fetched yet or fetch fails
- **WHEN** the menu-config fetch has not yet resolved or fails
- **THEN** the sidebar renders the full, unfiltered menu (fail-open / default-enabled behavior)
