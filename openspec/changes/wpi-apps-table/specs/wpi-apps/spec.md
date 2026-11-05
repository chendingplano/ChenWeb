## ADDED Requirements

### Requirement: Apps are stored apart from pages

App descriptors SHALL be stored, versioned, in `kb.wpi_apps`, and an app SHALL name the page it
opens by that page's `descriptor_id` in `initial_page`. A page SHALL NOT carry an `app` attribute.

#### Scenario: One page serves two apps
- **WHEN** two apps name the same `initial_page`
- **THEN** both load and neither changes the page

#### Scenario: Missing page
- **WHEN** an admin saves an app whose `initial_page` has no active page
- **THEN** the write is refused with 400 and nothing is stored

### Requirement: App attributes

An app descriptor SHALL have `descriptor_id`, `schema_version` and `initial_page`, and MAY have
`app_name`, `app_description` (text or text per language), `keywords` and `tags` (non-empty
strings). Any other attribute SHALL be rejected.

#### Scenario: Unknown attribute
- **WHEN** an app descriptor has an attribute such as `pane_layouter`
- **THEN** validation fails with `unknown attribute`
