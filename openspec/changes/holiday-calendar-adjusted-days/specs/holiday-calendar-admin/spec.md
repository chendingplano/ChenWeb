## MODIFIED Requirements

### Requirement: Year calendar view and selection
The system SHALL let an admin select a year, a country (from a fixed country list), and a
calendar type (defaulting to "holidays"), and view a full 12-month grid for that year with any
bound holiday dates visually marked. Dates bound as holiday days (days off) and dates bound as
adjusted working days SHALL be shown in different colors. Pending holiday-day selections and
pending adjusted-day selections SHALL also be shown in different colors from each other and
from saved bindings, and the page SHALL show a legend for these colors.

#### Scenario: View empty calendar
- **WHEN** an admin selects a (year, country, calendar type) combination with no existing
  `calendars` row
- **THEN** the system renders the 12-month grid for that year with no dates marked, and no
  `calendars` row is created until the admin saves a holiday binding

#### Scenario: View existing calendar
- **WHEN** an admin selects a (year, country, calendar type) combination with an existing
  `calendars` row
- **THEN** the system renders the grid with each bound date marked and labeled with its holiday
  info name, holiday days and adjusted working days in different colors

### Requirement: Bind holiday dates to a calendar
The system SHALL let an admin select one or more day cells in the year grid, as holiday days
and optionally as adjusted working days, and attach an existing or newly-created holiday info
to all selected dates in one action. The system SHALL create the underlying `calendars` row on
first save if it does not already exist. Each date binding SHALL record its day kind, either
`holiday` or `adjusted`. Each date within a calendar SHALL map to at most one holiday info and
one day kind.

#### Scenario: Attach holiday to multiple selected dates
- **WHEN** an admin selects multiple day cells in "Set Holidays" mode and attaches a holiday
  info to the selection
- **THEN** the system creates or updates a `calendars` row for the active (year, country, type)
  and creates one date binding per selected date, all referencing that holiday info with day
  kind `holiday`

#### Scenario: Attach holiday with adjusted working days
- **WHEN** an admin selects Jan 1–3 in "Set Holidays" mode, then selects Jan 4 in "Set Adjusted
  Days" mode, and attaches a holiday info
- **THEN** the system binds Jan 1–3 to that holiday info with day kind `holiday` and binds Jan 4
  with day kind `adjusted`, in a single transaction

#### Scenario: Switching a pending date between modes
- **WHEN** an admin clicks a date that is pending in one mode while the other mode is active
- **THEN** the date moves to the active mode's pending selection and is no longer pending in the
  other mode

#### Scenario: Re-binding an already-bound date
- **WHEN** an admin attaches a holiday info to a date that already has a binding
- **THEN** the system replaces that date's holiday info and day kind with the new ones

#### Scenario: Contradictory request rejected
- **WHEN** a client submits the same date in both `dates` and `adjusted_dates`
- **THEN** the system rejects the request with 400 and changes no data

#### Scenario: Legacy request without adjusted dates
- **WHEN** a client submits `dates` without `adjusted_dates`
- **THEN** the system binds those dates with day kind `holiday`, as before this change
