# Past Review Card Details Design

## Goal

Improve the Product Review intake page's Past reviews cards so each card identifies the product in both Chinese and English, presents its useful fields as explicit name-value rows, shows the number of metrics, and includes the associated product 3D drawing on the left.

## Current behavior

`product-review-intake-view.svelte` renders profile summaries as compact selectable cards. A profile's `keywords` are rendered as rounded chips, which explains why only the Ventilator card in the supplied screenshot has a pill: only that profile has a keyword. The summary API already returns `drawing_id`, but the UI does not render the drawing. The profile schema currently stores only `name`, while the `kb.product_names` catalog stores Chinese and English names.

## Design

### Persist bilingual names

Add `name_cn` and `name_en` columns to `kb.product_profiles` through a goose migration. Existing rows are backfilled with `name_cn = name` and, where possible, `name_en` is resolved by matching `kb.product_names.product_name` to the profile's `name`. Keep the existing `name` column and its current behavior for compatibility with duplicate detection and review pipeline code.

The ProductNameField exposes the selected catalog entry to its parent. Intake submits the catalog's Chinese and English names when a catalog suggestion is selected. For manually typed names, the typed value becomes `name_cn` and `name_en` remains empty. The backend accepts and stores these fields while preserving `name` as the existing lookup value.

### Profile summary data

Extend `Profile`/`ProfileSummary` and the profile-list query to return bilingual names and a metric count. The metric count is the latest run's `result_count`; it is null when no run has produced results. The summary continues to include `drawing_id`, latest run IDs/status, and completion time.

### Card layout

Use a wider, framed-preview card based on visual option B. The card is a horizontal flex layout:

1. A fixed-width product drawing thumbnail on the left, loaded from the existing drawing content endpoint using `drawing_id`.
2. A flexible details area on the right.

The details title is `name_cn / name_en` when English is present, otherwise just `name_cn`/the legacy name. The attributes are rendered as explicit rows:

- `Keywords`: comma-separated keyword values, or an em dash
- `Description`: clipped preview, or an em dash
- `Metrics`: latest metric count, or an em dash

The existing review status line and `View results` action remain available. A profile without a drawing gets a neutral placeholder in the same framed area so card alignment remains stable.

### Long descriptions

Descriptions are limited to a two-line visual preview with CSS line clamping and ellipsis. The full description is placed in the native `title` tooltip only when its rendered text exceeds the preview limit. The implementation should avoid showing a misleading tooltip for descriptions that fit.

### Interaction and responsive behavior

The entire card remains keyboard-selectable and clicking the result button continues to stop propagation and open the run. At narrower widths, the card may stack the drawing above the details; the drawing remains first in reading order. Selecting a card continues to prefill the existing form and switch the action to Re-Run.

## Error handling

- Missing or invalid `drawing_id` results in the neutral placeholder, not a broken card.
- Missing English name falls back to Chinese/legacy name without a slash.
- Missing keywords, description, or metric count displays an em dash.
- Existing profile-list loading remains non-blocking for the form.

## Testing and verification

- Add migration/store tests for bilingual name persistence and profile-summary metric count.
- Add intake/type tests confirming catalog selection sends both names and manual entry remains compatible.
- Add frontend checks for summary rendering, fallback values, image URL construction, and description tooltip behavior where practical.
- Run the affected Go tests, frontend tests, ESLint, and `svelte-check` before completion.

## Out of scope

- Replacing the existing duplicate-name hero flow.
- Adding search, filtering, or pagination to Past reviews.
- Changing review metric computation or drawing generation.
- Removing the legacy `name` column.
