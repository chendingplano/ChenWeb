## ADDED Requirements

### Requirement: Sort the video list
The system SHALL let an admin sort the Resources > Videos table by name,
upload time, or file size, in ascending or descending order, applied to the
entire `kb.videos` table (not just currently-rendered rows).

#### Scenario: Default order unchanged
- **WHEN** an admin opens the Videos page without choosing a sort option
- **THEN** the table lists videos ordered by upload time, most recent first
  (unchanged from current behavior)

#### Scenario: Sort by name ascending
- **WHEN** an admin selects "By Name ASC" from the Sort dropdown
- **THEN** the request to `GET /api/v1/videos` includes `sort_by=name` and
  `sort_dir=asc`, and the table re-renders with all matching videos ordered
  alphabetically A→Z by name

#### Scenario: Sort by size descending
- **WHEN** an admin selects "By Size DESC" from the Sort dropdown
- **THEN** the table re-renders with all matching videos ordered by file size
  largest first

### Requirement: Filter the video list by name
The system SHALL let an admin filter the Resources > Videos table to only
videos whose name contains a given substring (case-insensitive), applied to
the entire `kb.videos` table.

#### Scenario: Name filter narrows results
- **WHEN** an admin types "训练" into the Filter by Name field and applies it
- **THEN** the table shows only videos whose name contains "训练"
  (case-insensitive), regardless of how many total videos exist

#### Scenario: Empty name filter shows all
- **WHEN** the Filter by Name field is empty
- **THEN** no name filter is applied and videos are not excluded by name

### Requirement: Filter the video list by upload time range
The system SHALL let an admin filter the Resources > Videos table to only
videos uploaded within a given date range, applied to the entire `kb.videos`
table.

#### Scenario: Time range narrows results
- **WHEN** an admin sets a Filter by Time start date and end date and applies
  it
- **THEN** the table shows only videos whose upload time falls within the
  selected range, inclusive of both the start of the start date and the end
  of the end date

#### Scenario: Only start date provided
- **WHEN** an admin sets only a start date (no end date) and applies it
- **THEN** the table shows only videos uploaded on or after that date

#### Scenario: Only end date provided
- **WHEN** an admin sets only an end date (no start date) and applies it
- **THEN** the table shows only videos uploaded on or before the end of that
  date

#### Scenario: Invalid date range rejected
- **WHEN** an admin submits a start date that is after the end date, or a
  malformed date value
- **THEN** the system rejects the request with a 400 error instead of
  silently ignoring the filter or returning an unfiltered/incorrect result

### Requirement: Sort and filter compose
The system SHALL apply the selected sort order and any active name/time
filters together against the full `kb.videos` table in a single request.

#### Scenario: Combined sort and filter
- **WHEN** an admin has a Filter by Name value, a Filter by Time range, and a
  Sort option all set at once
- **THEN** the table shows only videos matching both filters, ordered
  according to the selected sort option
