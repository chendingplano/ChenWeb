## ADDED Requirements

### Requirement: Severity pie chart
The results view SHALL display a donut pie chart showing the distribution of findings by severity (High, Medium, Low). High slices SHALL use #ef4444, Medium #f59e0b, Low #22c55e. The chart SHALL include a legend listing each severity label and count.

#### Scenario: Chart renders with findings
- **WHEN** the review completes with at least one finding
- **THEN** a donut chart appears with colored slices proportional to the High / Medium / Low counts

#### Scenario: Chart renders with zero findings for a severity
- **WHEN** a severity level has zero findings
- **THEN** that severity is omitted from the pie slices but still listed in the legend with count "0"

### Requirement: Package pie chart
The results view SHALL display a donut pie chart showing the finding count per package (pass code). Packages with zero findings SHALL NOT appear as pie slices but SHALL be listed in a "No findings" section beside the chart.

#### Scenario: Non-empty packages shown as slices
- **WHEN** one or more packages have findings
- **THEN** each non-empty package appears as a proportional slice in the donut chart

#### Scenario: Empty packages listed beside chart
- **WHEN** one or more packages have zero findings
- **THEN** those packages appear in a labeled list beside the chart and are not represented as slices

### Requirement: Reviewer pie chart
The results view SHALL display a donut pie chart showing the finding count per reviewer (aspect). Reviewers with zero findings SHALL NOT appear as pie slices but SHALL be listed in a "No findings" section beside the chart.

#### Scenario: Non-empty reviewers shown as slices
- **WHEN** one or more reviewers have findings
- **THEN** each non-empty reviewer appears as a proportional slice in the donut chart

#### Scenario: Empty reviewers listed beside chart
- **WHEN** one or more reviewers have zero findings
- **THEN** those reviewers appear in a labeled list beside the chart and are not represented as slices

### Requirement: Metadata block
The results view SHALL display a metadata card showing: Start Time (formatted from request.start_time), Time Used in seconds (request.end_time − request.start_time), Total Findings, Total Non-Empty Packages (packages with at least one finding), and Total Non-Empty Reviewers (aspects with at least one finding).

#### Scenario: All timing fields present
- **WHEN** the review has both start_time and end_time
- **THEN** the metadata card shows Start Time and Time Used in seconds

#### Scenario: End time absent
- **WHEN** end_time is not yet set (review still running)
- **THEN** Time Used shows "—"

### Requirement: 2×2 grid layout
The four new panels (severity chart, package chart, reviewer chart, metadata) SHALL be arranged in a 2-column grid, preserving the same horizontal footprint as the previous four single-column cards.

#### Scenario: Grid renders on completed review
- **WHEN** the review status is completed
- **THEN** the 2×2 grid of panels is shown in place of the old four stat cards
