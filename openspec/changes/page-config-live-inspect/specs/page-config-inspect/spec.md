## ADDED Requirements

### Requirement: Embedded preview of the selected page

The Page Content admin view SHALL display a same-origin `<iframe>` preview of the page currently selected in the `Page` dropdown, alongside the entry table. The preview's source SHALL be the selected page's `route`. The preview pane SHALL be collapsible so the entry table can occupy the full width.

#### Scenario: Selecting a page shows it in the preview
- **WHEN** the admin selects a page in the `Page` dropdown
- **THEN** the preview iframe loads that page's `route` and displays the live page

#### Scenario: Collapsing the preview
- **WHEN** the admin toggles the preview pane closed
- **THEN** the preview is hidden and the entry table expands to full width, and toggling it open restores the preview

### Requirement: Reusable page inspection contract

A page SHALL be made inspectable solely by adding a `data-entry-key="<entry_key>"` attribute to each element whose content is resolved from `getPageConfig`, where `<entry_key>` matches the `entry_key` used in `kb.page_config`. The admin inspector SHALL locate configurable elements generically via this attribute and SHALL require no admin-side code change to support an additional page.

#### Scenario: A new page becomes inspectable with only the attribute
- **WHEN** a page that uses `getPageConfig` stamps `data-entry-key` on its configurable elements and no admin code is changed
- **THEN** hovering that page in the preview and hovering its rows in the admin both produce correct two-way highlighting

#### Scenario: The knowledge page is instrumented
- **WHEN** the knowledge page (`home3-knowledge`) is previewed
- **THEN** each sidebar menu element carries `data-entry-key` equal to its menu item `id` (the `entry_key`)

### Requirement: Hovering a config row highlights the matching preview element

WHEN the admin hovers a row in the entry table, the corresponding element in the preview (the element whose `data-entry-key` equals the row's `entry_key`) SHALL be visually highlighted and scrolled into view; the highlight SHALL be removed when the hover ends.

#### Scenario: Row hover highlights element
- **WHEN** the admin hovers a config row whose `entry_key` is present in the preview
- **THEN** the matching element is highlighted and scrolled into view, and the highlight clears when the pointer leaves the row

#### Scenario: Row has no matching element in the preview
- **WHEN** the admin hovers a config row whose `entry_key` is not rendered in the preview (hidden, unauthorized, or stale)
- **THEN** no highlight is shown and no error occurs

### Requirement: Hovering a configurable preview element highlights the matching config row

WHEN the admin hovers an element in the preview that carries a `data-entry-key`, the config row whose `entry_key` equals that value SHALL be visually highlighted; the highlight SHALL be removed when the hover ends. This behavior SHALL continue to work after the previewed page re-renders its content internally.

#### Scenario: Element hover highlights row
- **WHEN** the admin hovers a configurable element in the preview
- **THEN** the config row with the matching `entry_key` is highlighted, and the highlight clears when the pointer leaves the element

#### Scenario: Highlighting survives internal preview re-render
- **WHEN** the previewed page re-renders configurable content (for example expanding a menu section) without a full navigation
- **THEN** hovering the re-rendered elements still highlights the matching config rows

### Requirement: Cross-origin and unloaded previews degrade safely

The inspector SHALL access the preview document only when it is same-origin and loaded. If the preview document is unavailable (still loading) or not same-origin, the inspector SHALL disable highlighting for that preview without throwing an error; the preview itself SHALL still render.

#### Scenario: Preview not yet loaded
- **WHEN** the preview iframe has not finished loading its document
- **THEN** hover interactions produce no highlight and no error

#### Scenario: Cross-origin preview
- **WHEN** the previewed page is served from a different origin
- **THEN** the page still renders in the preview, highlighting is silently disabled, and no error is thrown
