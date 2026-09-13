## ADDED Requirements

### Requirement: Card-styled, full-width intake layout
The Product Review intake page SHALL present the intake form and the past-reviews list as separate
full-width, rounded-rectangle card blocks inside the middle content panel, matching the visual
convention (bordered surface, 12px corner radius) used by the Document Review page, instead of a
720px-wide centered single column.

#### Scenario: Intake page renders as full-width cards
- **WHEN** a user opens Applications → Product Review
- **THEN** the intake form and the past-reviews list each render inside their own bordered,
  rounded-rectangle card that stretches to the middle panel's width (minus the panel's standard side
  padding), rather than a centered column capped at 720px

### Requirement: Two-column intake form with model selection
The intake form SHALL arrange the Product name, Keywords, and Image Generation Model fields in a
two-column grid wide enough to fill the card, with the Short description and Notes fields spanning
both columns, and SHALL let the user choose an image generation model (Qwen or OpenAI) before
starting a review.

#### Scenario: Fields render in two columns
- **WHEN** the intake form is displayed
- **THEN** Product name, Keywords, and Image Generation Model each occupy one column of a two-column
  grid, and Short description and Notes each span the full width of that grid

#### Scenario: Model selection defaults and is submitted
- **WHEN** the user has not changed the Image Generation Model field
- **THEN** it defaults to "Qwen · Aliyun", offers "OpenAI · ChatGPT Image 2.5" as the other option, and
  the chosen value is included in the request the Start button sends
