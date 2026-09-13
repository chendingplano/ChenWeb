## ADDED Requirements

### Requirement: Automatic drawing lookup or generation at intake
When a Product Review is started (or resumed) for a profile with no `drawing_id` set, the system SHALL
look up `kb.product_drawings` for an existing drawing whose name matches the product name (trimmed,
case-insensitive), and bind its id to `kb.product_profiles.drawing_id` if found — without generating a
new image.

#### Scenario: Existing drawing is reused
- **WHEN** a Product Review is started for a product name that already has a row in
  `kb.product_drawings` with a matching name
- **THEN** no new image is generated, and that drawing's id is written to
  `kb.product_profiles.drawing_id` for the profile

### Requirement: Automatic drawing generation when none exists
When no matching drawing is found for a profile with no `drawing_id` set, the system SHALL generate one
using the image generation model chosen on the intake form, save it to `kb.product_drawings`, and bind
its id to `kb.product_profiles.drawing_id` — as part of the same intake request that starts the review,
with no additional user action.

#### Scenario: New product gets a generated drawing automatically
- **WHEN** a user submits the intake form for a product name with no matching row in
  `kb.product_drawings`
- **THEN** the system generates a drawing with the selected model, inserts a new row into
  `kb.product_drawings`, and sets that row's id as the profile's `drawing_id`, all before the intake
  request completes

#### Scenario: Drawing generation failure does not block the review
- **WHEN** the image generation provider call fails during automatic drawing generation
- **THEN** the intake request still proceeds to start the review, the profile's `drawing_id` remains
  unset, and the failure is logged rather than returned as an error to the user

### Requirement: No duplicate generation for profiles that already have a drawing
The system SHALL NOT look up or generate a drawing for a profile that already has a non-null
`drawing_id`, whether the profile is newly created, resumed, or re-run.

#### Scenario: Re-running a reviewed product does not regenerate its drawing
- **WHEN** a user re-runs or resumes a review for a profile whose `drawing_id` is already set
- **THEN** the system does not query `kb.product_drawings` or call the image generation provider again
