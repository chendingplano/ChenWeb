## Why

The current four summary stat cards in the Document Review results view display raw numbers (total, high, medium, low) but provide no visual distribution insight. Replacing them with two SVG pie charts, one package distribution chart, and a metadata block gives reviewers an at-a-glance picture of where findings are concentrated and how long the review took.

## What Changes

- Remove the four static count cards from the completed-state summary area (lines 292–310 of `doc-review-results-view.svelte`)
- Add a 2×2 grid of four new panels:
  - **Severity pie chart** — donut chart of High / Medium / Low counts with inline legend
  - **Package pie chart** — donut chart of findings per pass/package; zero-finding packages listed beside the chart
  - **Reviewer pie chart** — donut chart of findings per aspect/reviewer; zero-finding reviewers listed beside the chart
  - **Metadata block** — compact card showing Start Time, Time Used (seconds), Total Findings, Total Non-Empty Packages, Total Non-Empty Reviewers
- Store `packages` (from `getRequest` response) in component state so empty packages can be shown in the legend
- Compute `elapsedSeconds` from `request.start_time` / `request.end_time`

## Capabilities

### New Capabilities

- `doc-review-summary-charts`: SVG pie/donut charts and metadata panel replacing the static summary count cards in the Document Review results view

### Modified Capabilities

<!-- none -->

## Impact

- **Modified file**: `ChenWeb/web/src/lib/components/home3/doc-review-results-view.svelte`
- No new dependencies (charts use inline SVG)
- No API changes; `packages` field is already returned by `getRequest` but was unused in this component
- No breaking changes
