## ADDED Requirements

### Requirement: Intake form captures product name, description, keywords, and notes
The Product Review intake page SHALL present a form with fields for product name (required),
short description (optional), keywords (optional), and notes (optional), and a "Start" action
that SHALL be disabled until a product name is entered.

#### Scenario: Start is disabled without a name
- **WHEN** the intake form's product name field is empty
- **THEN** the "Start" action is disabled

#### Scenario: Start is enabled with a name
- **WHEN** the user has entered a product name (description, keywords, and notes optional)
- **THEN** the "Start" action is enabled

### Requirement: Starting a review for a new product creates and runs it in one step
When the submitted product name has no existing profile, clicking "Start" SHALL create the
product profile, build it, mark it ready, and start a review run, then return a run the frontend
can open — without requiring any further manual step.

#### Scenario: First-time product name
- **WHEN** a user submits a product name that does not match any existing profile and clicks
  "Start"
- **THEN** the backend creates a new profile, builds its scope tree, marks it ready, executes a
  review run, persists the run's results and report, and returns that run's identifier
- **AND** the frontend navigates to the review's results page for that run

### Requirement: Starting a review for a previously reviewed product surfaces a choice instead of duplicating it
When the submitted product name matches an existing profile, clicking "Start" SHALL NOT create a
second profile for the same product. Instead the user SHALL be offered the choice to view the
existing results or trigger a re-run.

#### Scenario: Duplicate with a completed prior run
- **WHEN** a user submits a product name that matches an existing profile that has at least one
  completed run, and clicks "Start"
- **THEN** no new profile is created
- **AND** the user is offered "View results" and "Re-run" instead of a new run starting
  automatically

#### Scenario: Viewing existing results
- **WHEN** the user selects "View results" from the duplicate choice
- **THEN** the frontend navigates to the results page for the existing profile's latest
  completed run

#### Scenario: Re-running from the duplicate choice
- **WHEN** the user selects "Re-run" from the duplicate choice
- **THEN** the backend starts a fresh run against the existing profile (its current accepted
  scope tree and version), persists its results and report, and the frontend navigates to that
  new run's results page

#### Scenario: Duplicate with no completed run yet
- **WHEN** a user submits a product name that matches an existing profile that has no completed
  run (an earlier attempt did not finish)
- **THEN** the user is offered only "Re-run", with no "View results" option

### Requirement: Notes are passed through to the review run; keywords are stored but not sent to the AI
Notes entered on the intake form SHALL be recorded on the resulting review run's request in the
same way an operator-supplied note would be. Keywords entered on the intake form SHALL be stored
on the product profile. Neither notes nor keywords SHALL be sent to the AI model that proposes
the product's structure — that step continues to use only the product name and description.

#### Scenario: Notes appear on the run's request
- **WHEN** a user submits notes along with a new product and clicks "Start"
- **THEN** the resulting review run's request records those notes

#### Scenario: Keywords are stored on the profile
- **WHEN** a user submits keywords along with a new product and clicks "Start"
- **THEN** those keywords are stored on the created product profile
