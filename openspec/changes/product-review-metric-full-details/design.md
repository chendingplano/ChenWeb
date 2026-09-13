## Context

Two views already show overlapping data for the same underlying `kb.metrics` row:

- **Product Review** (`product-metric-review-view.svelte`) — a review-scoped result row
  (`ResultRow`) plus a fetched `MetricDetail` (via `getMetricDetail(artifact_id)` →
  `/product-reviews/artifacts/:artifactId/metric`, ~10 fields). Both `ResultRow.artifact_id` and
  `MetricDetail.artifact_id` are metric-id strings in the `<input_record_id>_mtc_<n>` format
  (e.g. `386_mtc_130`) — same convention `metric-mgmt-view.svelte` and `metricWikiService.ts`'s
  `metricIdFromArtifactId` already rely on. `ResultRow.input_record_id` gives the owning
  `kb.inputs` record id directly; `detailRawLines`/`detailInputId` (already loaded to jump/
  highlight the PDF for the selected result) hold that record's raw lines.
- **Metrics workspace** (`metric-mgmt-view.svelte`) — the authoritative, much richer record
  (`KbMetricRecord`, ~30 fields), loaded per `kb.inputs` record via `listKbMetrics(id)`. Its
  right-hand "group info panel" (GIP) renders a `metric` selection as five curated groups —
  Metric / Metadata / Context / Grounding / Reasoning — built by a private function
  `buildMetricGroupAttrs(metric, spans, lineByKey)` plus `normalizeMetricSpans` (line-span → page
  resolution) and `confidencePct`. This is the exact view the second reference screenshot shows;
  it is a plain grouped field list (`.attr-view`/`.attr-group`/`.gip-row`), not the radial
  "satellite" canvas the surrounding types (`GroupNode`, `SatelliteNode`) suggest — that geometry
  is computed but not what's rendered on screen today.

**Revision note:** the first version of this change linked out to the standalone `/home3/metrics`
route in a new tab (mirroring `nav-rail.svelte`'s existing "open in a new tab" pattern for that
workspace) and added `initialRecordId`/`initialMetricId` deep-link props to `MetricMgmtView` for
it. After implementing and testing that version, explicit feedback was: a new tab is the wrong
behavior — the reviewer wants the metric's full details **in place**, as a dialog they can close,
without leaving or interrupting their Product Review session. The deep-link props/route params
were reverted (dead code once nothing used them); this document now describes the dialog design
that replaced them. The one part of the original work worth keeping was the observation that
`buildMetricGroupAttrs`/`normalizeMetricSpans`/`confidencePct` are pure functions of
`(KbMetricRecord, raw lines)` with no other dependency on `metric-mgmt-view.svelte`'s state —
which is what makes reusing them from Product Review's dialog possible.

## Goals / Non-Goals

**Goals:**
- One click from a selected Product Review result to that metric's full curated detail, shown
  in place (modal dialog), closable, without navigating away or opening a new tab.
- The dialog shows the same fields, in the same five groups, as the Metrics workspace — one
  source of truth for "what a full metric detail view contains."
- Reuse existing data access (`listKbMetrics`, and the raw lines already loaded for the selected
  result) — no new backend endpoint.

**Non-Goals:**
- Navigating to or reusing the standalone `/home3/metrics` page or the embedded
  `/home3/knowledge?section=kb-metrics` section — both are full workspaces (record browser,
  PDF pane, search) which is more than "show this one metric's details" calls for, and both
  leave/replace the reviewer's current page.
- Replicating the Metrics workspace's specific visual theme (serif headings, brass accents,
  icons). The dialog reuses Product Review's own existing dialog chrome (`.doc-dialog-*`, already
  used for the "document record" dialog) for visual consistency with the page it lives on.
- Changing `MetricDetail` / the product-review backend API, or `metric-mgmt-view.svelte`'s
  rendering/behavior.

## Decisions

1. **Presentation: an in-page modal dialog, not a new tab or route navigation.**
   Reuses the existing `.doc-dialog-overlay`/`.doc-dialog` pattern already in
   `product-metric-review-view.svelte` (used today for the "document record" dialog opened from
   the same panel) — same overlay/escape/click-outside-to-close behavior, so this is consistent
   with an existing convention on the same page rather than a new one. Rejected alternatives:
   (a) new tab to `/home3/metrics` — the initial implementation; explicitly rejected as
   interrupting the reviewer's viewing. (b) navigating the embedded `/home3/knowledge` section in
   the same tab — discards the reviewer's in-progress run/results state.

2. **Shared field-grouping logic, extracted rather than duplicated.**
   `buildMetricGroupAttrs`, `normalizeMetricSpans`, and `confidencePct` moved from
   `metric-mgmt-view.svelte` into a new pure module,
   `web/src/lib/components/home3/metric-detail-groups.ts`; `metric-mgmt-view.svelte` now imports
   them (mechanical change, same behavior — `normalizeMetricSpans` gained an explicit
   `lineNumToPage` parameter since it can no longer close over the component's local `$derived`).
   The Product Review dialog imports the same functions. Alternative considered: copy the ~150
   lines into a second, dialog-local copy instead of touching the existing, working
   `metric-mgmt-view.svelte` — rejected because the two views would silently drift on which
   fields a "full metric detail" shows as the backend's `kb.metrics` schema evolves; the shared
   module keeps that field list in exactly one place, and the extraction is a pure, low-risk
   relocation (verified via `bun run check` and a smoke-load of both views).

3. **Data fetch: `listKbMetrics(result.input_record_id)`, matched by `metric_id`; raw lines reused
   from the already-loaded PDF view rather than re-fetched.**
   Same lookup `handleMetricSearchResultClick` already does in `metric-mgmt-view.svelte` (load a
   record's metrics, find by `metric_id` string — `ResultRow`/`MetricDetail` never carry the
   internal numeric `KbMetricRecord.id`). For the Grounding section's line text, the dialog reuses
   `detailRawLines`/`detailInputId` — already fetched by the existing PDF-highlighting code for
   the same `input_record_id` — guarded so it's only used when it actually matches the metric's
   record (avoids a stale-data window if the dialog is opened before that fetch settles).
   Alternative considered: a new backend endpoint returning one `KbMetricRecord` by `metric_id`
   directly — unnecessary; `listKbMetrics` per-record is what every existing caller already does,
   and Product Review only ever has one record's worth of metrics to search at a time.

4. **Row rendering: flatten each attribute group into `{key, value, depth}` rows and reuse
   `.doc-dialog-rows`/`.doc-dialog-row`/`.doc-dialog-key`/`.doc-dialog-value`, the same primitives
   `flattenForDisplay` already renders for the document dialog.** A `lines`-kind attribute (the
   Grounding section) renders as a depth-0 header row plus depth-1 rows per source line, mirroring
   how the document dialog already nests object fields. This avoids introducing the Metrics
   workspace's separate `.gip-*`/`.attr-*` CSS (chips, line-cards, icons) into Product Review, at
   the cost of a plainer look for the Grounding section (no per-line "type" chip styling, just
   inline text) — acceptable since the goal is showing the data, not the graphic treatment.

5. **Button placement and label unchanged from the original version**: a single "Full Details"
   button appended after the last `<dd>` inside `.drawer-fields`, inside the existing
   `{#if selectedResult}` block. Paraglide keys added: `pmr_full_details_button`,
   `pmr_full_details_loading`, `pmr_full_details_not_found`, and `pmr_group_metric` /
   `_metadata` / `_context` / `_grounding` / `_reasoning` for the five section headers (EN + ZH).

## Risks / Trade-offs

- **`metric_id` format coupling**: unchanged from the original design — `ResultRow.artifact_id`
  and `KbMetricRecord.metric_id` must stay the same string convention. Already relied on
  elsewhere (`metricWikiService.ts`); not new.
- **Metric not found**: if `listKbMetrics` succeeds but no row's `metric_id` matches (stale data),
  the dialog shows `pmr_full_details_not_found` instead of a blank/broken card.
- **Refactor risk in `metric-mgmt-view.svelte`**: moving `buildMetricGroupAttrs`/
  `normalizeMetricSpans`/`confidencePct` out touches a large (~4200-line), previously untouched-
  by-this-change file. Mitigated by keeping the moved functions byte-for-behavior identical (only
  `normalizeMetricSpans`'s signature gained an explicit parameter) and verifying with
  `bun run check` plus a manual load of the Metrics workspace after the change.
- **No automated end-to-end verification of the dialog's actual rendered content**: this
  workspace's dev environment has no bootstrapped login (`CHENWEB_ROOT_ACCOUNT_PASSWORD` unset)
  and no existing product-review run with results, so the click-through was verified by the user
  directly rather than by an agent-driven browser test.

## Migration Plan

Purely additive/refactoring frontend change; no data migration. The now-unused `/home3/metrics`
deep-link query params and `MetricMgmtView` props from the first version of this change were
reverted rather than left in place, since nothing calls them anymore.
