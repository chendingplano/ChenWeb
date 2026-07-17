## ADDED Requirements

### Requirement: Announcement storage is row-per-locale
The system SHALL store announcements in `kb.site_announcements`, with one row per `(group_id, lang)` pair, where `group_id` identifies a single logical announcement and `lang` is a supported language code from `[languages].languages`. Each row SHALL carry `occurred_at` (timestamp shown in the "time" column), `importance` (free-text category), and the announcement text for that locale.

#### Scenario: Creating an announcement writes one row per configured language
- **WHEN** an admin creates a new announcement with text for each configured language
- **THEN** the system inserts one `kb.site_announcements` row per language, all sharing a newly generated `group_id`

#### Scenario: Editing an announcement updates all locale rows together
- **WHEN** an admin edits the `occurred_at` or `importance` of an existing announcement
- **THEN** the system updates every row sharing that announcement's `group_id` in a single transaction, keeping non-text fields identical across locales

### Requirement: Announcement read API is locale-filtered and capped
The system SHALL expose a read endpoint returning announcements for the requesting locale, ordered by `occurred_at` descending, limited to at most `[frontend].announcements_max` rows (default 5 when unset), with an internal hard cap regardless of configuration.

#### Scenario: Workspace page requests announcements in the active locale
- **WHEN** `/semos/workspace` requests announcements for locale `zh`
- **THEN** the API returns only `lang='zh'` rows, at most `announcements_max` of them, most recent `occurred_at` first

#### Scenario: Missing translation for the active locale
- **WHEN** an announcement group has no row for the requesting locale
- **THEN** that announcement is omitted from the response for that locale (no fallback row is fabricated)

### Requirement: Workspace page renders announcements in three columns
The system SHALL render each returned announcement as a row with three columns: time, importance, and announcement text, and SHALL show an empty-state message when no announcements are returned.

#### Scenario: No announcements configured
- **WHEN** the announcements API returns zero rows
- **THEN** the workspace page shows the existing localized empty-state message instead of an empty table

### Requirement: Admin CRUD page for announcements
The system SHALL provide `/semos/admin/announcements`, gated by the existing authenticated-session check, allowing any logged-in user to create, edit, and delete announcements across all configured locales in one action.

#### Scenario: Admin deletes an announcement
- **WHEN** an admin deletes an announcement
- **THEN** the system removes every `kb.site_announcements` row sharing that announcement's `group_id`

#### Scenario: Unauthenticated request to admin API
- **WHEN** a request without a valid session calls the announcements admin API
- **THEN** the system rejects it the same way any other `/api/v1` endpoint rejects unauthenticated requests

## REMOVED Requirements

### Requirement: TOML-configured workspace announcements
**Reason**: Replaced by the database-backed `kb.site_announcements` table and admin CRUD page; announcements are no longer edited by changing config files.
**Migration**: Existing entries in `config/site/site-default*.toml` `[workspace].announcements` are carried over into `kb.site_announcements` as part of this change's migration, then the TOML array is deleted. Future announcement changes go through `/semos/admin/announcements`.
