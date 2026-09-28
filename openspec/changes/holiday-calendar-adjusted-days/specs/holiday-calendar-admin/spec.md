## MODIFIED Requirements

### Requirement: Year calendar view and selection
The system SHALL let an admin select a year, a country (from a fixed country list), and a
calendar type (defaulting to "holidays"), and view a full 12-month grid for that year with any
bound holiday dates visually marked. Dates bound as holiday days (days off) and dates bound as
adjusted working days SHALL be shown in different colors. Pending holiday-day selections and
pending adjusted-day selections SHALL also be shown in different colors from each other and
from saved bindings, and the page SHALL show a legend for these colors.

#### Scenario: View calendar that does not exist
- **WHEN** an admin selects a (year, country, calendar type) combination with no existing
  `calendars` row
- **THEN** the system renders the 12-month grid for that year with no dates marked and day
  selection disabled, and shows a Create button in the upper panel if the (country, calendar
  type) holiday info exists, or asks the admin to create the holiday info first

#### Scenario: Create a calendar
- **WHEN** an admin clicks the upper panel's Create for a (year, country, calendar type) with
  no `calendars` row
- **THEN** the system creates the empty `calendars` row and enables day selection

#### Scenario: Holiday info shown independent of year
- **WHEN** an admin selects a country and calendar type, whatever the year and whether that
  year's calendar exists
- **THEN** the lower panel shows that (country, calendar type)'s holiday info, or, if it has
  no holidays yet, a Create button that opens the New Holiday form

#### Scenario: Holiday definitions scoped to country and calendar type
- **WHEN** an admin selects a country and a calendar type
- **THEN** the holiday definitions list shows only the definitions of that (country, calendar
  type), in their display order, whatever the selected year

#### Scenario: Holiday from another calendar type rejected
- **WHEN** a client binds dates of a (country, calendar type) calendar to a holiday info of a
  different country or calendar type
- **THEN** the system rejects the request with 400 and changes no data

#### Scenario: Incomplete calendar key
- **WHEN** the year is empty or invalid
- **THEN** the system loads no calendar and hides the day grid, while the lower panel still
  shows the holiday info for the country and calendar type

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

### Requirement: Edit and delete calendar date bindings
The system SHALL let an admin stage changes to saved date bindings (change a date's day kind, or
remove the binding), select new holiday or adjusted days, and save all of these changes to the
active calendar with a "Modify" action, which binds each new day to the holiday of its nearest
saved date without prompting, or delete an entire
calendar (all date bindings for a given year, country, and calendar type) in one action. The
Modify action SHALL be disabled while there are no staged changes.

#### Scenario: Stage and save changes to saved dates
- **WHEN** an admin, in "Set Holidays" mode, clicks a saved holiday day and a saved adjusted day,
  then clicks Modify
- **THEN** the system removes the first date's binding, rebinds the second date to the same
  holiday info with day kind `holiday`, and the grid shows the saved result

#### Scenario: Modify adds a new adjusted day to the nearest holiday
- **WHEN** Jan 1–3 are saved holiday days of 元旦, and an admin selects Jan 4 in "Set Adjusted
  Days" mode and clicks Modify
- **THEN** the system binds Jan 4 to 元旦 with day kind `adjusted`, with no dialog

#### Scenario: Modify disabled without changes
- **WHEN** there are no staged changes, including after an admin clicks a saved date twice so
  that it returns to its saved kind
- **THEN** the Modify button is disabled

#### Scenario: Discard staged changes
- **WHEN** an admin has staged changes and clicks Clear Selection
- **THEN** the system discards them and saved bindings are unchanged

#### Scenario: Delete entire calendar
- **WHEN** an admin deletes the calendar for the active (year, country, calendar type)
- **THEN** the system deletes the `calendars` row and all its date bindings
