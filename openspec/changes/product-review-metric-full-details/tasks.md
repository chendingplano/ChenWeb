## 1. Shared field-grouping module

- [x] 1.1 Create `web/src/lib/components/home3/metric-detail-groups.ts`, moving
      `buildMetricGroupAttrs`, `normalizeMetricSpans` (now taking an explicit
      `lineNumToPage: Map<number, number>` parameter instead of closing over component state),
      `confidencePct`, and the `AttrDef`/`LineEntry`/`NormalizedSpan`/`AttrKind` types out of
      `metric-mgmt-view.svelte`. Add `buildLineNumToPage(rawLines)` as a small new export.
- [x] 1.2 Update `metric-mgmt-view.svelte` to import from the new module instead of defining
      these locally; update the (now four) `normalizeMetricSpans(...)` call sites to pass
      `lineNumToPage`; remove the now-duplicate local `confidencePct`. Keep `SatelliteNode` /
      `GroupNode` / `MetricsCanvas` types and all other behavior unchanged.
- [x] 1.3 `bun run check` in `web/` — confirm no new type errors from the move (pre-existing,
      unrelated `bun:test`/`.ts`-extension errors in test files are expected and unaffected).

## 2. i18n strings

- [x] 2.1 Add to `web/messages/en.json` / `web/messages/zh-cn.json`: `pmr_full_details_button`
      ("Full Details" / "完整详情"), `pmr_full_details_loading`, `pmr_full_details_not_found`,
      and `pmr_group_metric` / `pmr_group_metadata` / `pmr_group_context` /
      `pmr_group_grounding` / `pmr_group_reasoning` (the five dialog section headers).

## 3. Product Review "Full Details" button + dialog

- [x] 3.1 In `product-metric-review-view.svelte`, add the "Full Details" button after the last
      `<dd>` in `.drawer-fields`, inside the existing `{#if selectedResult}` block.
- [x] 3.2 Add dialog state (`showFullDetailsDialog`, `fullDetailsLoading`, `fullDetailsError`,
      `fullDetailsMetric`) and an `openFullDetails(result)` function that fetches
      `listKbMetrics(result.input_record_id)` and matches by `metric_id`, guarded against
      out-of-order responses (`fullDetailsRequestId`).
- [x] 3.3 Add `fullDetailsRawLines` (reusing `detailRawLines` when it matches the metric's
      record) and `fullDetailsGroups` (via the shared `buildMetricGroupAttrs`/
      `normalizeMetricSpans`/`buildLineNumToPage`), plus `attrRows()` to flatten each group into
      `{key, value, depth}` rows for display.
- [x] 3.4 Render the dialog reusing the existing `.doc-dialog-overlay`/`.doc-dialog` markup
      pattern (same as the "document record" dialog on the same page): header with the metric
      name and a Close button, body with one section per group (Metric, Metadata, Context,
      Grounding, Reasoning) showing filled/total counts and its rows.
- [x] 3.5 Style the "Full Details" button consistently with the panel (`.ghost` + a small
      `.full-details-btn` modifier for full width / centering).

## 4. Superseded work reverted

- [x] 4.1 Revert `web/src/routes/home3/metrics/+page.svelte` to its original form (no
      `record_id`/`metric_id` query-param reading) — the new-tab deep-link this fed is no longer
      used.
- [x] 4.2 Remove the `initialRecordId`/`initialMetricId` props and mount-effect from
      `metric-mgmt-view.svelte` that were added for that deep-link.

## 5. Verification

- [x] 5.1 `bun run check` after all changes — same 3 pre-existing, unrelated errors as baseline;
      no new errors.
- [x] 5.2 Playwright smoke-load of `/home3/product-review` and `/home3/metrics` against the
      local dev server — both compile and render (only expected 401s from unauthenticated API
      calls; no console/page JS errors).
- [ ] 5.3 Manual re-verification of the in-page dialog in the browser (the user verified the
      earlier new-tab version and asked for it to become a dialog instead; that dialog behavior
      itself hasn't been re-confirmed against a real login/data yet). Same auth/data blockers as
      before apply to agent-driven verification — see design.md's Risks section.
