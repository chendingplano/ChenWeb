# code-monitor Specification

## Purpose

Define daily code-size collection and the Code Monitor page for configured ChenWeb workspace projects.

## Requirements

### Requirement: Configured project collection

The system SHALL read project directory names from `frontend.projects-to-monitor`. Each name SHALL be resolved from the workspace root and SHALL identify the project group shown in the API and page.

#### Scenario: Local project configuration

- **WHEN** the local configuration lists `ChenWeb`
- **THEN** the system collects the project rooted at the workspace's `ChenWeb` directory and groups its history under `ChenWeb`

### Requirement: Frontend and backend source metrics

For each project, the system SHALL count recognized Git-tracked source files under `web/src` as frontend files and under `server` as backend files. The total line count SHALL sum physical lines from both directories. Blank lines and a final line without a newline SHALL count; empty files SHALL contribute zero lines. Files outside these directories and files with unrecognized extensions SHALL be ignored. A tracked file missing from the worktree SHALL be skipped.

#### Scenario: Project directories determine category

- **WHEN** a recognized source file is under `web/src` or `server`
- **THEN** it contributes to the total lines and its file count is determined by its directory

### Requirement: Daily snapshots

The system SHALL store one row per `(project_path, collection_date)` in `kb.code_monitor` and collect daily at 2:00 am in the configured workspace timezone. If the table has no rows at startup, it SHALL immediately collect a snapshot for the current local date with `collected_at` set to that date's 2:00 am local time. The system SHALL keep subsequent collections on the daily schedule.

#### Scenario: Empty history at startup

- **WHEN** the server starts and `kb.code_monitor` contains no rows
- **THEN** it stores a current-date snapshot with a synthetic 2:00 am local collection timestamp

### Requirement: Project history API and deltas

The authenticated `GET /api/v1/code-monitor?range=<range>` endpoint SHALL return daily rows grouped by project. Each row SHALL include total and delta values for total lines, frontend files, and backend files. A delta SHALL equal the current value minus the preceding recorded snapshot for the same project, even if that snapshot is outside the selected range. The first snapshot SHALL have a zero delta.

#### Scenario: Prior snapshot outside requested range

- **WHEN** the selected range contains a snapshot with an earlier recorded snapshot outside that range
- **THEN** the returned delta SHALL compare with that earlier snapshot

#### Scenario: First snapshot

- **WHEN** a project has only one recorded snapshot
- **THEN** its three delta values SHALL be zero

### Requirement: Code Monitor page and charts

The system SHALL provide a bilingual Code Monitor page under Development → System Admin → System → Code Monitor. It SHALL group three charts under each configured project: total lines, frontend files, and backend files. Each chart SHALL show Total and Delta as bar series, with Total on the left y-axis and Delta on the right y-axis.

#### Scenario: Choose a chart range

- **WHEN** a user selects a supported range
- **THEN** the page displays the project's available daily rows for that range

Supported ranges are `today`, `yesterday`, `last_7_days` (today and six prior dates), `last_month` (30 local dates ending today), `last_6_months`, and `last_12_months`. Calendar ranges SHALL use inclusive local dates and clamp month boundaries to the last valid day.
