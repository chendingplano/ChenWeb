## ADDED Requirements

### Requirement: Recent activity storage is row-per-locale
The system SHALL store recent activities in `kb.recent_activities`, with one row per `(group_id, lang)` pair, where `group_id` identifies a single logical activity entry and `lang` is a supported language code from `[languages].languages`. Each row SHALL carry `occurred_at`, `activity_type` (free-text category), and the activity text for that locale.

#### Scenario: Creating an activity writes one row per configured language
- **WHEN** an admin creates a new recent activity entry with text for each configured language
- **THEN** the system inserts one `kb.recent_activities` row per language, all sharing a newly generated `group_id`

#### Scenario: Editing an activity updates all locale rows together
- **WHEN** an admin edits the `occurred_at` or `activity_type` of an existing activity entry
- **THEN** the system updates every row sharing that entry's `group_id` in a single transaction, keeping non-text fields identical across locales

### Requirement: Recent activity read API is locale-filtered and bounded
The system SHALL expose a read endpoint returning recent activities for the requesting locale, ordered by `occurred_at` descending, limited to a fixed internal cap (20 rows).

#### Scenario: Workspace page requests recent activities in the active locale
- **WHEN** `/semos/workspace` requests recent activities for locale `en`
- **THEN** the API returns only `lang='en'` rows, most recent `occurred_at` first, at most 20 rows

### Requirement: Workspace page renders recent activities in three columns
The system SHALL render each returned activity as a row with three columns: time, type, and activity text, and SHALL show an empty-state message when no activities are returned.

#### Scenario: No recent activities available
- **WHEN** the recent activities API returns zero rows
- **THEN** the workspace page shows the existing localized empty-state message instead of an empty table

### Requirement: Admin CRUD page for recent activities
The system SHALL provide `/semos/admin/recent-activities`, gated by the existing authenticated-session check, allowing any logged-in user to create, edit, and delete recent activity entries across all configured locales in one action.

#### Scenario: Admin deletes a recent activity entry
- **WHEN** an admin deletes a recent activity entry
- **THEN** the system removes every `kb.recent_activities` row sharing that entry's `group_id`

#### Scenario: Unauthenticated request to admin API
- **WHEN** a request without a valid session calls the recent-activities admin API
- **THEN** the system rejects it the same way any other `/api/v1` endpoint rejects unauthenticated requests
