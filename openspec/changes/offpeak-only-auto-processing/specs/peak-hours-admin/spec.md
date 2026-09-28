## MODIFIED Requirements

### Requirement: Peak hours evaluation
The system SHALL provide a read endpoint that resolves whether a named peak-hours definition is
active at a given instant (default: the current server time), applying `applicable_days` first,
then `exclude_days`, then checking the instant's time-of-day against `hours`, all evaluated in the
record's `timezone`. A date bound as an adjusted working day (`calendar_holidays.day_kind =
'adjusted'`) for the record's `country` SHALL be treated as a workday.

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
  is `workdays` and the instant falls on a Saturday that is not an adjusted working day)
- **THEN** the system returns `{"active": false}` without evaluating `hours`

#### Scenario: Inactive due to weekend exclusion
- **WHEN** `exclude_days` includes `"weekends"` and the evaluation instant falls on a Saturday or
  Sunday (in the record's timezone) that is not an adjusted working day
- **THEN** the system returns `{"active": false}` regardless of `applicable_days` or `hours`

#### Scenario: Adjusted working day
- **WHEN** the record has a non-empty `country` and the evaluation instant's date is bound with
  `day_kind = 'adjusted'` in that country's `holidays` calendar for the instant's year
- **THEN** the date matches `applicable_days` mode `workdays`, is not excluded by `"weekends"` or
  `"holidays"`, and the system evaluates `hours` as on any workday

#### Scenario: Inactive due to holiday exclusion
- **WHEN** `exclude_days` includes `"holidays"`, the record has a non-empty `country`, and the
  evaluation instant's date (in the record's timezone) matches a date bound with
  `day_kind = 'holiday'` in `public.calendar_holidays` for that `country` and the instant's year
  with `calendar_type = 'holidays'`
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
