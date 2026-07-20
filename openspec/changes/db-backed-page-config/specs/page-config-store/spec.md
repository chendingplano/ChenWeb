## ADDED Requirements

### Requirement: Page definition storage
The system SHALL store one `kb.page_def` row per configurable frontend page, keyed by a stable, unique `page_key`, alongside its `route` and admin-facing metadata.

#### Scenario: Page identified by stable key
- **WHEN** a page is registered for configurable content
- **THEN** it has exactly one `kb.page_def` row with a unique `page_key`
- **AND** the `page_key` is stable across route, title, and metadata edits

#### Scenario: Duplicate page key rejected
- **WHEN** a second `kb.page_def` row is inserted with an existing `page_key`
- **THEN** the unique constraint on `page_key` rejects it

### Requirement: Per-language configurable entry storage
The system SHALL store configurable page entries in `kb.page_config`, one row per entry per language, uniquely identified by `(page_key, entry_key, language)`, carrying a `content` payload, an `access_role` array, an `accessible` flag, and an `enabled` flag.

#### Scenario: Entry identity is the page/entry pair
- **WHEN** an entry's label, description, translated content, or accessibility changes
- **THEN** its `page_key + entry_key` identity is unchanged
- **AND** frontend, backend, and admin tooling reference it by `page_key + entry_key`, never by display text

#### Scenario: One row per language
- **WHEN** an entry has both an `en` and a `zh-cn` translation
- **THEN** there are two `kb.page_config` rows sharing `page_key + entry_key`, differing by `language`
- **AND** the unique `(page_key, entry_key, language)` constraint forbids a duplicate row for the same triple

#### Scenario: Every active entry has a default-language row
- **WHEN** an entry is active
- **THEN** a row exists whose `language` equals `[languages].default`
- **AND** that default-language row is authoritative for `accessible`, `enabled`, and `access_role`

### Requirement: Explicit accessibility and enable columns
The system SHALL provide, on `kb.page_config`, an explicit `accessible` boolean (default true), an explicit `enabled` boolean (default true), and an `access_role` JSON array, so an entry can be scoped by role, temporarily suspended, or disabled independently.

#### Scenario: Defaults preserve visibility
- **WHEN** a row is inserted without specifying `accessible` or `enabled`
- **THEN** both default to `true`

### Requirement: Seed of existing file-based domains
The migration SHALL seed `kb.page_def` and `kb.page_config` so the existing `/home3/knowledge` and `/semos/workspace` content renders identically after rewiring, inserting one entry per current menu id and workspace key in both `en` and `zh-cn`, marked `accessible = true`, `enabled = true`, and `access_role` set to the current `[system].access_roles` key list, with labels/descriptions carried over from today's config.

#### Scenario: Seed reproduces current rendering
- **WHEN** the seed migration has run and no operator changes are made
- **THEN** every menu id and workspace key currently shown is present in `kb.page_config` for both locales with its current label/description
- **AND** each seeded entry is authorized for any user holding at least one `[system].access_roles` role

#### Scenario: Seed is idempotent
- **WHEN** the seed migration runs more than once
- **THEN** it inserts each row at most once (`ON CONFLICT (page_key, entry_key, language) DO NOTHING`)
