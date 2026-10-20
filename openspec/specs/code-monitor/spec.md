# code-monitor Specification

## Purpose

Define daily code-size collection and the Code Monitor page for configured ChenWeb workspace projects.

## Requirements

### Requirement: Configured project collection

The system SHALL read project directory names from `frontend.projects-to-monitor`. Each name SHALL be resolved from the workspace root and SHALL identify the project group shown in the API and page.

#### Scenario: Local project configuration

- **WHEN** the local configuration lists `ChenWeb`
- **THEN** the system collects the project rooted at the workspace's `ChenWeb` directory and groups its history under `ChenWeb`

### Requirement: Project-wide source line metrics

For each configured project, the system SHALL count recognized Git-tracked source files throughout the project. The total line count SHALL include both frontend and backend source. Files under `web/src`, `svelte/src`, `frontend/src`, or `client/src` SHALL count as frontend; other recognized source files SHALL count as backend. Blank lines and a final line without a newline SHALL count; empty files SHALL contribute zero lines. Files with unrecognized extensions SHALL be ignored. A tracked file missing from the worktree SHALL be skipped.

#### Scenario: Project-wide totals include frontend and backend

- **WHEN** a recognized source file is tracked anywhere in a configured project
- **THEN** its physical lines contribute to the project total and its lines are assigned to the frontend or backend component

### Requirement: Daily snapshots

The system SHALL store one row per `(project_path, collection_date)` in `kb.code_monitor` and collect daily at 2:00 am in the configured workspace timezone. If the table has no rows at startup, it SHALL immediately collect a snapshot for the current local date with `collected_at` set to that date's 2:00 am local time. The system SHALL keep subsequent collections on the daily schedule.

#### Scenario: Empty history at startup

- **WHEN** the server starts and `kb.code_monitor` contains no rows
- **THEN** it stores a current-date snapshot with a synthetic 2:00 am local collection timestamp

### Requirement: Project history API and deltas

The authenticated `GET /api/v1/code-monitor?range=<range>` endpoint SHALL return daily rows grouped by project. Each row SHALL include the project's total source lines, frontend line delta, and backend line delta. Component deltas SHALL equal the current component line total minus the preceding recorded snapshot for the same project, even if that snapshot is outside the selected range. The first snapshot SHALL have zero component deltas. Total lines SHALL equal frontend plus backend line totals for that snapshot.

#### Scenario: Prior snapshot outside requested range

- **WHEN** the selected range contains a snapshot with an earlier recorded snapshot outside that range
- **THEN** the returned delta SHALL compare with that earlier snapshot

#### Scenario: First snapshot has zero component deltas

- **WHEN** a project has only one recorded snapshot
- **THEN** its frontend and backend line deltas SHALL be zero

### Requirement: Code Monitor page and charts

The system SHALL provide a bilingual Code Monitor page under Development → System Admin → System → Code Monitor. For each configured project, it SHALL show three charts on the same row: project total, frontend, and backend. Each chart SHALL show paired bars for the metric total and its daily delta over the selected collection dates. The total and delta bars SHALL use separate value axes. A project's first recorded delta SHALL be zero.

#### Scenario: Choose a chart range

- **WHEN** a user selects a supported range
- **THEN** the page displays the project's available daily rows for that range

Supported ranges are `today`, `yesterday`, `last_7_days` (today and six prior dates), `last_month` (30 local dates ending today), `last_6_months`, and `last_12_months`. Calendar ranges SHALL use inclusive local dates and clamp month boundaries to the last valid day.
