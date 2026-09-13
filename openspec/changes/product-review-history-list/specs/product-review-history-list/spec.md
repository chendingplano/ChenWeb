## ADDED Requirements

### Requirement: Past reviews are listed as cards on the intake page
The Product Review intake page SHALL display a list of previously created product profiles as
cards, ordered by most recently updated first, each showing the product's name, description, its
keywords, and its latest review run's status.

#### Scenario: Profiles exist
- **WHEN** the intake page loads and at least one product profile exists for the tenant
- **THEN** a card is shown for each returned profile, most recently updated first, with its name,
  description, keywords, and latest-run status visible

#### Scenario: No profiles exist yet
- **WHEN** the intake page loads and no product profile exists for the tenant
- **THEN** the page shows an empty state instead of any cards

### Requirement: Selecting a card prefills the form without leaving it
Clicking a card SHALL populate the intake form's product name, description, keywords, and notes
fields with that profile's stored values, SHALL leave the form visible and editable, and SHALL
change the submit action's label from "Start" to "Re-Run". Selecting a card SHALL NOT trigger the
existing duplicate-name hero screen.

#### Scenario: Card selected
- **WHEN** a user clicks a past-review card
- **THEN** the name, description, keywords, and notes fields are populated from that profile
- **AND** the submit button reads "Re-Run"
- **AND** the plain form remains visible (no hero/takeover screen appears)

### Requirement: Submitting a selected card resumes that profile's review
When the form has a card selected, submitting SHALL resume that profile's review — reusing the
existing re-run behavior (re-running the profile's latest request if one exists, or resuming a
never-run draft profile if not) — rather than creating a new profile.

#### Scenario: Re-running a profile with a prior request
- **WHEN** a user selects a card for a profile that already has at least one review request and
  clicks "Re-Run"
- **THEN** the backend starts a fresh run against that profile's latest request
- **AND** the frontend navigates to that new run's results page

#### Scenario: Resuming a profile that was never run
- **WHEN** a user selects a card for a profile that has no review request yet and clicks "Re-Run"
- **THEN** the backend builds and starts a review against that existing profile instead of
  creating a second profile for the same product
- **AND** the frontend navigates to the resulting run's results page

### Requirement: Editing the name away from the selected card reverts to Start
The selection SHALL be cleared and the submit button SHALL revert to "Start" if a card is
selected and the user then edits the product name field to a value other than that card's name;
submitting then creates/looks up a profile for the new name as a plain review, not a re-run.

#### Scenario: Name edited after selecting a card
- **WHEN** a user selects a card and then changes the product name field to a different value
- **THEN** the card selection is cleared
- **AND** the submit button reads "Start" again
- **AND** submitting creates/looks up a profile for the newly entered name as if no card had been
  selected

### Requirement: Profile list endpoint returns summaries with latest-run status
The backend SHALL expose a way to list product profiles for a tenant, ordered by most recently
updated first, where each entry includes the profile's name, description, keywords, notes,
status, and — if any review request exists for it — the id and status of its latest run.

#### Scenario: Listing profiles with mixed run history
- **WHEN** the profile list is requested for a tenant that has profiles with completed runs,
  profiles with no runs yet, and profiles with a running/failed latest run
- **THEN** every matching profile is returned, most recently updated first
- **AND** each entry reflects its own latest run's id and status, or indicates no run exists for
  profiles that have none
