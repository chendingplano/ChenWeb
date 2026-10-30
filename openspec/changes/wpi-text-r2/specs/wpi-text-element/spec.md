## ADDED Requirements

### Requirement: Localized read-only text sources

The text module SHALL implement the human-approved R2 attribute contract in `KnowledgeStore/doc-repo/code_modules/interpreter-text.md`. Exactly one of merged text and data_binding SHALL be required at runtime; incomplete save and fixed-value rules SHALL remain unchanged. Static localized maps SHALL follow current locale then English; bound values SHALL retain null-to-empty and JavaScript String conversion. Text SHALL never write the input doc.

#### Scenario: Existing bound copy
- **WHEN** an old class copy binds a boolean false or number zero
- **THEN** the paragraph shows false or 0, remains selectable and never changes the doc

#### Scenario: Locale changes
- **WHEN** the current locale changes while a localized paragraph, caption and help popup are mounted
- **THEN** their text updates without losing page state or focus

### Requirement: Caption icon and accessible tooltip

The module SHALL support optional caption, decorative local icon and supplementary tooltip with R2 defaults and layout. Caption SHALL wrap without truncation. Narrow horizontal captions SHALL stack above content below 250px. Tooltip SHALL support hover, focus, click, tap, Escape, outside activation, hover transfer and cleanup on hide/unmount.

#### Scenario: Tooltip dismissed from keyboard
- **WHEN** a focused help button has an open tooltip and Escape is pressed
- **THEN** the popup closes and focus remains on the help button

#### Scenario: Cached page hidden
- **WHEN** a page containing an open tooltip is hidden or unmounted
- **THEN** the popup closes and no portal content or timers remain

### Requirement: Validated defaults and preserved copies

The module SHALL support all 30 R2 attributes and store the 26 concrete defaults with the four documented exceptions. Client/server SHALL reject invalid supported values with matching grammar. Unknown class attributes SHALL remain visible warnings. New descriptor versions SHALL preserve old versions and page copies.

#### Scenario: Invalid supported value
- **WHEN** a text instance sets zero icon size, an unknown icon or out-of-range RGB channels
- **THEN** validation reports the offending attribute rather than silently ignoring it

### Requirement: Text authoring and layout

The Properties panel SHALL expose the six bilingual groups, localized controls, defaults/reset/fixed-value display and conditional fields without deleting hidden values. Text SHALL use border-box dimensions and keyboard/touch-accessible scrolling when explicit dimensions overflow.

#### Scenario: Reset an override
- **WHEN** an author resets an editable text attribute
- **THEN** its override is removed and its saved-copy default or module fallback is shown

#### Scenario: Fixed height paragraph
- **WHEN** the complete text cannot fit an explicitly assigned height
- **THEN** it is reachable by local keyboard/touch scrolling without covering the next element
