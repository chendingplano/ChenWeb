## ADDED Requirements

### Requirement: Resources page menu tree
The Resources page (`pageKey="resources"`) SHALL present a left-rail menu tree with two top-level folders: `Documents` and `Videos`. `Documents` SHALL contain the items `User's Manual` and `Development`. `Videos` SHALL contain the item `Training`.

#### Scenario: Menu structure on document page
- **WHEN** a logged-in user opens `/resources`
- **THEN** the left rail shows a `Documents` folder containing `User's Manual` and `Development`, and a `Videos` folder containing `Training`

#### Scenario: Folders expand to reveal items
- **WHEN** the user expands the `Videos` folder
- **THEN** the `Training` item is revealed and selectable

#### Scenario: Localized menu labels
- **WHEN** the site language is Simplified Chinese
- **THEN** the folder and item labels render in their seeded `zh-cn` text
- **WHEN** the site language is English
- **THEN** they render in their seeded English text

### Requirement: Menu scoped to the document page
The `Documents` and `Videos` folders and their items SHALL be visible only on the `resources` page. They SHALL NOT appear in the nav rail on `/home3`, `/development`, or any other page. Visibility SHALL be driven by seeded `kb.page_def`/`kb.page_config` rows for the `resources` page key (for both `en` and `zh-cn`), consistent with the existing DB-backed page-config overlay mechanism.

#### Scenario: Hidden elsewhere
- **WHEN** a user opens `/home3` or `/development`
- **THEN** the `Documents`/`Videos` document-page folders do not appear in the left rail

#### Scenario: Config-driven visibility
- **WHEN** the page-config rows for the document menu are disabled
- **THEN** the corresponding menu items are omitted from the document page's rail

### Requirement: Placeholder document items
`User's Manual` and `Development` SHALL be selectable menu items that render a non-functional placeholder view in the content panel for this change. No document content, upload, or management behavior is delivered for them here.

#### Scenario: Placeholder content
- **WHEN** the user selects `User's Manual` or `Development`
- **THEN** the content panel shows a placeholder view (e.g. a "coming soon"/empty state) rather than an error or blank crash
