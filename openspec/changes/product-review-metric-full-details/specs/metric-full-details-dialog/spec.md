## ADDED Requirements

### Requirement: Full Details button on a selected Product Review metric
The Product Review METRIC DETAILS panel SHALL display a "Full Details" button after the
panel's field list whenever a result row is selected, and SHALL NOT display it when no
result is selected.

#### Scenario: Button appears after selecting a result
- **WHEN** a reviewer selects a result row in the Product Review results tab
- **THEN** the METRIC DETAILS panel shows the button labeled "Full Details" (localized) at the
  end of its field list

#### Scenario: Button absent with no selection
- **WHEN** no result row is selected in the Product Review results tab
- **THEN** the METRIC DETAILS panel shows its empty state and no "Full Details" button

### Requirement: Full Details opens an in-page dialog, not a new tab or page navigation
Clicking "Full Details" SHALL open a modal dialog within the current Product Review page.
It SHALL NOT open a new browser tab or window, and SHALL NOT navigate the current tab away
from Product Review.

#### Scenario: Opening full details for a selected result
- **WHEN** a reviewer clicks "Full Details" for a selected result
- **THEN** a modal dialog opens over the current page
- **AND** no new browser tab or window is opened
- **AND** the reviewer's Product Review results, selection, and scroll position are unaffected
  once the dialog is closed

#### Scenario: Closing the dialog
- **WHEN** the dialog is open and the reviewer clicks its Close button, clicks outside the
  dialog, or presses Escape
- **THEN** the dialog closes and Product Review is shown exactly as it was before opening it

### Requirement: Dialog shows the metric's full curated detail, grouped
The dialog SHALL show the selected result's metric using the same Metric / Metadata / Context
/ Grounding / Reasoning field grouping the Metrics workspace (Knowledge Base → Metrics) shows
for the same metric, sourced from the same underlying `kb.metrics` record.

#### Scenario: Metric found
- **WHEN** the dialog opens for a result whose metric exists in `kb.metrics`
- **THEN** the dialog shows five sections — Metric, Metadata, Context, Grounding, Reasoning —
  each listing that metric's fields for the section, matching what the Metrics workspace shows
  for the same `metric_id`

#### Scenario: Metric not found
- **WHEN** the dialog opens for a result whose `metric_id` no longer matches any metric on its
  source record (e.g. stale data)
- **THEN** the dialog shows a "not found" message instead of a blank or broken card, with no
  error thrown

#### Scenario: Loading state
- **WHEN** the dialog opens and the metric's full record has not yet finished loading
- **THEN** the dialog shows a loading indicator in place of the field groups
