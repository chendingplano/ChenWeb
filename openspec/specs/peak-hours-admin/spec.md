# peak-hours-admin Specification

## Purpose
TBD - created by archiving change peak-hours-admin. Update Purpose after archive.

## Requirements

### Requirement: Admin-only access
The Peak Hours admin page and its backend endpoints SHALL be accessible only to users who are
owner, admin, or have role `admin` or `root`. All other users SHALL receive 401 (unauthenticated)
or 403 (authenticated but unauthorized). The evaluation endpoint
(`GET /api/v1/peak-hours/{name}/is-active`) is exempt from this restriction, since other backend
code needs to query it.

#### Scenario: Non-admin is denied CRUD access
- **WHEN** an authenticated user without admin/root/owner privileges calls
  `POST`, `PUT`, or `DELETE` on any `/api/v1/peak-hours` endpoint
- **THEN** the system returns 403 with an error envelope and performs no data change

#### Scenario: Admin nav entry
- **WHEN** an admin/root user opens Development → System Admin → System
- **THEN** a "Peak Hours" leaf item is visible alongside "Calendar" and, when selected, renders
  the peak hours admin view

### Requirement: Peak hours record management
The system SHALL let an admin create, list, update, and delete named peak-hours definitions.
Each definition SHALL have: a `name` (required, unique, used as its lookup key), `hours` (one or
more time-of-day ranges), `timezone` (a valid IANA zone), `applicable_days` (one of: all
workdays, a specific set of weekdays, or a specific set of days-of-month), `exclude_days` (zero or
more of: weekends, holidays, or specific dates/date ranges — ORed together), and an optional
`country` used to resolve holiday exclusions.

#### Scenario: Create peak hours definition
- **WHEN** an admin submits a new peak hours definition with a unique name, at least one hour
  range, a valid timezone, and an applicable-days rule
- **THEN** the system creates the record and returns it

#### Scenario: Duplicate name rejected
- **WHEN** an admin submits a peak hours definition whose `name` already exists
- **THEN** the system rejects the request with an error and creates no duplicate

#### Scenario: Invalid hour range rejected
- **WHEN** an admin submits an `hours` entry that is not a well-formed `HH:MM-HH:MM` range, or
  whose start is not before its end
- **THEN** the system rejects the request with an error identifying the invalid entry

#### Scenario: Invalid timezone rejected
- **WHEN** an admin submits a `timezone` value that is not a valid IANA zone name
- **THEN** the system rejects the request with an error and creates no record

#### Scenario: Invalid exclude_days entry rejected
- **WHEN** an admin submits an `exclude_days` entry that is not `"weekends"`, `"holidays"`, an
  ISO date (`YYYY-MM-DD`), or an ISO date range (`YYYY-MM-DD..YYYY-MM-DD`)
- **THEN** the system rejects the request with an error identifying the invalid entry

#### Scenario: Update peak hours definition
- **WHEN** an admin submits changes to an existing peak hours definition's fields (other than
  `name`, which is immutable after creation)
- **THEN** the system validates and persists the changes and returns the updated record

#### Scenario: Delete peak hours definition
- **WHEN** an admin deletes an existing peak hours definition by name
- **THEN** the system removes the record; later evaluation requests for that name return 404

### Requirement: Peak hours evaluation
The system SHALL provide a read endpoint that resolves whether a named peak-hours definition is
active at a given instant (default: the current server time), applying `applicable_days` first,
then `exclude_days`, then checking the instant's time-of-day against `hours`, all evaluated in the
record's `timezone`.

#### Scenario: Active within an hour range on an applicable day
- **WHEN** the evaluation instant, converted to the record's timezone, falls on a day matched by
  `applicable_days`, is not matched by any `exclude_days` entry, and falls within one of the
  record's `hours` ranges
- **THEN** the system returns `{"active": true}`

#### Scenario: Inactive outside all hour ranges
- **WHEN** the evaluation instant falls on an applicable, non-excluded day but outside every
  `hours` range
- **THEN** the system returns `{"active": false}`

#### Scenario: Inactive on a non-applicable day
- **WHEN** the evaluation instant's day does not match `applicable_days` (e.g. `applicable_days`
  is `workdays` and the instant falls on a Saturday)
- **THEN** the system returns `{"active": false}` without evaluating `hours`

#### Scenario: Inactive due to weekend exclusion
- **WHEN** `exclude_days` includes `"weekends"` and the evaluation instant falls on a Saturday or
  Sunday (in the record's timezone)
- **THEN** the system returns `{"active": false}` regardless of `applicable_days` or `hours`

#### Scenario: Inactive due to holiday exclusion
- **WHEN** `exclude_days` includes `"holidays"`, the record has a non-empty `country`, and the
  evaluation instant's date (in the record's timezone) matches a bound date in
  `public.calendar_holidays` for that `country` and the instant's year with
  `calendar_type = 'holidays'`
- **THEN** the system returns `{"active": false}` regardless of `applicable_days` or `hours`

#### Scenario: Holiday exclusion with no country configured
- **WHEN** `exclude_days` includes `"holidays"` but the record's `country` is empty, or no
  matching calendar row exists for that country/year
- **THEN** the system treats the `"holidays"` entry as matching no dates (it does not exclude
  anything) rather than returning an error

#### Scenario: Inactive due to specific excluded date or range
- **WHEN** `exclude_days` includes an ISO date or ISO date range that contains the evaluation
  instant's date (in the record's timezone)
- **THEN** the system returns `{"active": false}` regardless of `applicable_days` or `hours`

#### Scenario: Unknown name
- **WHEN** the evaluation endpoint is called with a `name` that has no matching peak hours record
- **THEN** the system returns 404 with an error envelope

#### Scenario: Unparseable instant
- **WHEN** the evaluation endpoint is called with an `at` query parameter that is not a valid
  RFC3339 timestamp
- **THEN** the system returns 400 with an error envelope

#### Scenario: Default instant
- **WHEN** the evaluation endpoint is called without an `at` query parameter
- **THEN** the system evaluates against the current server time
