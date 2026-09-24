## ADDED Requirements

### Requirement: Admin-only access
The Calendar admin page and its backend endpoints SHALL be accessible only to users who are
owner, admin, or have role `admin` or `root`. All other users SHALL receive 401 (unauthenticated)
or 403 (authenticated but unauthorized).

#### Scenario: Non-admin is denied
- **WHEN** an authenticated user without admin/root/owner privileges calls any
  `/api/v1/calendars/*` endpoint
- **THEN** the system returns 403 with an error envelope and performs no data change

#### Scenario: Admin nav entry
- **WHEN** an admin/root user opens Development → System Admin → System
- **THEN** a "Calendar" leaf item is visible and, when selected, renders the calendar admin view

### Requirement: Holiday info management
The system SHALL let an admin create, list, update, and delete year-independent holiday
definitions ("holiday info"), each with a country, name, optional description, and optional note.
A holiday info SHALL be uniquely identified by (country, name).

#### Scenario: Create holiday info
- **WHEN** an admin submits a new holiday info with country, name, and optional description/note
- **THEN** the system creates the record and returns it

#### Scenario: Duplicate holiday info rejected
- **WHEN** an admin submits a holiday info whose (country, name) already exists
- **THEN** the system rejects the request with an error and creates no duplicate

#### Scenario: Delete holiday info still bound to a date
- **WHEN** an admin deletes a holiday info that is still bound to at least one calendar date
- **THEN** the system rejects the deletion with 409 and an error message naming the conflict

#### Scenario: Delete unused holiday info
- **WHEN** an admin deletes a holiday info with no calendar date bindings
- **THEN** the system deletes the record

### Requirement: Year calendar view and selection
The system SHALL let an admin select a year, a country (from a fixed country list), and a
calendar type (defaulting to "holidays"), and view a full 12-month grid for that year with any
bound holiday dates visually marked.

#### Scenario: View empty calendar
- **WHEN** an admin selects a (year, country, calendar type) combination with no existing
  `calendars` row
- **THEN** the system renders the 12-month grid for that year with no dates marked, and no
  `calendars` row is created until the admin saves a holiday binding

#### Scenario: View existing calendar
- **WHEN** an admin selects a (year, country, calendar type) combination with an existing
  `calendars` row
- **THEN** the system renders the grid with each bound date marked and labeled with its holiday
  info name

### Requirement: Bind holiday dates to a calendar
The system SHALL let an admin select one or more day cells in the year grid and attach an
existing or newly-created holiday info to all selected dates in one action, creating the
underlying `calendars` row on first save if it does not already exist. Each date within a
calendar SHALL map to at most one holiday info.

#### Scenario: Attach holiday to multiple selected dates
- **WHEN** an admin selects multiple day cells and attaches a holiday info to the selection
- **THEN** the system creates or updates a `calendars` row for the active (year, country, type)
  and creates one date binding per selected date, all referencing that holiday info

#### Scenario: Re-binding an already-bound date
- **WHEN** an admin attaches a different holiday info to a date that already has one bound
- **THEN** the system replaces the existing binding for that date with the new one

### Requirement: Per-deployment default country
The system SHALL let an admin configure at most one country as the default selection shown when
the Calendar admin page loads. When no default is configured, the page SHALL default to "US".
Setting a new default country SHALL replace any previously configured default, so at most one
country is ever marked as default.

#### Scenario: No default configured
- **WHEN** the Calendar admin page loads and no default country has been configured
- **THEN** the page selects "US" as the initial country

#### Scenario: Set a default country
- **WHEN** an admin checks "Set as default country" while a country is selected
- **THEN** the system records that country as the default, and the Calendar admin page
  subsequently loads with that country pre-selected

#### Scenario: Changing the default country
- **WHEN** an admin sets country B as the default while country A was previously the default
- **THEN** the system records only country B as the default; country A is no longer default

#### Scenario: Clearing the default country
- **WHEN** an admin unchecks "Set as default country" for the currently configured default
- **THEN** the system clears the configured default, and the page reverts to defaulting to "US"

### Requirement: Edit and delete calendar date bindings
The system SHALL let an admin remove a single date's holiday binding, or delete an entire
calendar (all date bindings for a given year, country, and calendar type) in one action.

#### Scenario: Remove one date binding
- **WHEN** an admin removes the holiday binding on a single date
- **THEN** the system deletes that binding and the date shows as unmarked in the grid

#### Scenario: Delete entire calendar
- **WHEN** an admin deletes the calendar for the active (year, country, calendar type)
- **THEN** the system deletes the `calendars` row and all its date bindings
