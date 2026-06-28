## Context

`doc-review-results-view.svelte` renders the completed-review screen. Lines 292–310 display four plain stat cards (total, high, medium, low counts). The component already has:
- `findings: FindingItem[]` (each has `.pass`, `.aspect`, `.severity`)
- `highCount`, `mediumCount`, `lowCount` derived values
- `aspectStatuses: AspectStatus[]` (full list of all reviewers, including empty ones)
- `packages` is returned by `getRequest` but is currently discarded in the polling function

Data shapes relevant to the new panels:
- **Package** = `finding.pass` (e.g. "P1", "P2")
- **Reviewer** = `finding.aspect` (e.g. "grammar", "structure_check")
- **Full package list** = `ReviewPackageInfo[]` from `getRequest` (key + label)
- **Full reviewer list** = `AspectStatus[]` already stored in `aspectStatuses`
- **Timing** = `request.start_time` and `request.end_time` (ISO strings)

## Goals / Non-Goals

**Goals:**
- Replace the four count cards with a 2×2 grid of: severity pie, package pie, reviewer pie, metadata block
- Use pure inline SVG for charts — no external chart library
- Show all packages/reviewers (including zero-finding ones) in the legend beside each pie
- Compute elapsed time from `start_time` / `end_time`

**Non-Goals:**
- Interactive chart tooltips or animations
- Changing the report tab or findings list behavior
- Altering any API endpoints

## Decisions

### D1: Inline SVG donut charts, no library
Rationale: no new dependency, consistent with codebase inline-style pattern, full control over colors and sizing. A donut chart is rendered as SVG `<circle>` arcs using `stroke-dasharray` / `stroke-dashoffset`. Each slice is one `<circle>` element positioned using cumulative offsets.

**Alternative considered**: Canvas 2D API — rejected because SVG is declarative and fits Svelte's reactive model without imperative draw calls.

### D2: Package data stored in component state
The `pollStatus` function currently discards `result.packages`. It will be stored in `let packages = $state<ReviewPackageInfo[]>([])`. Zero-finding packages are the entries in `packages` whose `.key` does not appear in the set of unique `f.pass` values in `findings`.

**Alternative considered**: Derive package labels from `AspectInfo` — not available in this component; `ReviewPackageInfo` from `getRequest` is the correct source.

### D3: Reviewer "empty" list derived from `aspectStatuses`
`aspectStatuses` already holds the full set of reviewers. Zero-finding reviewers are those whose `finding_count === 0` (or whose `.aspect` is not present in the findings set). Use `aspectStatuses` entries with `finding_count === 0` for the "no findings" list.

### D4: Elapsed time display
`elapsedSeconds = Math.round((new Date(request.end_time).getTime() - new Date(request.start_time).getTime()) / 1000)`. Format: `Xs` (e.g. "142s"). If `end_time` is absent (still running), show "—".

### D5: 2×2 grid layout matches original four-panel footprint
The outer container keeps `display: grid; grid-template-columns: repeat(4, 1fr)` changed to `repeat(2, 1fr)` with each cell taller, preserving visual weight in the UI. Each cell has `background: cardBg; border: 1px solid borderColor; border-radius: 12px`.

The pie+legend layout inside each chart card uses `display: flex; gap: 0.75rem` with the SVG on the left and the legend list on the right.

## Risks / Trade-offs

- [SVG arc math for donut slices] → Use the `stroke-dasharray` technique on a single `<circle>` per slice; circumference = 2π×r is fixed, so offset per slice is straightforward.
- [Large number of reviewers/packages] → Legend list is scrollable (`overflow-y: auto; max-height: ...`) so it does not overflow the card.
- [Derived reactive computation] → All chart data (slice arrays, metadata values) is computed with `$derived.by` to stay reactive with polling updates.

## Migration Plan

Single-file change. No migration needed. Rollback = revert the Svelte file.
