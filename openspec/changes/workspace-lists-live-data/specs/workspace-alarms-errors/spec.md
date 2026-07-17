## ADDED Requirements

### Requirement: Alarm/error storage has no i18n and tracks status/notes
The system SHALL store alarms/errors in `public.alarms_errors`, one row per event, with `occurred_at`, `severity`, `message`, `status` (`unsolved` or `solved`, defaulting to `unsolved`), and `notes` (a JSON array of `{time, user, note}` objects, defaulting to empty). No locale/translation fields exist on this table.

#### Scenario: New alarm/error row defaults to unsolved with no notes
- **WHEN** a row is inserted into `public.alarms_errors` without an explicit `status` or `notes`
- **THEN** the row has `status='unsolved'` and `notes='[]'`

### Requirement: Alarm/error read API supports an unsolved-only filter
The system SHALL expose a read endpoint returning alarms/errors ordered by `occurred_at` descending, limited to a fixed internal cap (20 rows for the workspace display), and SHALL support a query parameter that filters to `status='unsolved'` only versus all statuses.

#### Scenario: Workspace page requests alarms/errors
- **WHEN** `/semos/workspace` requests the alarms/errors list
- **THEN** the API returns up to 20 rows ordered by most recent `occurred_at` first, all statuses included

#### Scenario: Admin toggles "unsolved only"
- **WHEN** the admin alarms page requests the list with the unsolved-only filter enabled
- **THEN** the API returns only rows with `status='unsolved'`

### Requirement: Workspace page renders alarms/errors in three columns
The system SHALL render each returned alarm/error as a row with three columns: Time, Severity, and Alarms/Errors (message), with no i18n applied to the message text, and SHALL show an empty-state message when no rows are returned.

#### Scenario: No alarms or errors present
- **WHEN** the alarms/errors API returns zero rows
- **THEN** the workspace page shows the existing localized empty-state message instead of an empty table

### Requirement: Admin alarms page allows status and note updates only
The system SHALL provide `/semos/admin/alarms`, gated by the existing authenticated-session check, listing all alarm/error fields read-only except `status` and `notes`, with a toggle to show unsolved-only versus all rows. All other fields (`occurred_at`, `severity`, `message`) SHALL NOT be editable through this page or its API.

#### Scenario: Admin marks an alarm solved
- **WHEN** an admin sets an alarm/error's status to `solved`
- **THEN** the system updates only that row's `status` field, leaving `occurred_at`, `severity`, and `message` unchanged

#### Scenario: Admin appends a note
- **WHEN** an admin submits a note on an alarm/error
- **THEN** the system appends a new `{time, user, note}` object to the existing `notes` array, setting `time` to the current server time and `user` to the authenticated user's identity, without letting the client set either field directly

#### Scenario: Admin cannot edit read-only fields
- **WHEN** a request to the alarms admin API includes changes to `occurred_at`, `severity`, or `message`
- **THEN** the system ignores those fields and only applies `status`/`notes` changes
