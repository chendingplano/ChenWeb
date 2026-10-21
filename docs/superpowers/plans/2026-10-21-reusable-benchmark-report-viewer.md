# Reusable Benchmark Report Viewer Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Render Summary benchmark reports in the same resizable report/PDF viewer structure as Metrics, using a shared report frame that future artifact benchmark pages can reuse.

**Architecture:** Add a typed shared Svelte report frame that owns report chrome, PDF pane, split resizing, evidence controls, and source selection. Migrate the current Metrics viewer into that frame while retaining Metrics-specific row rendering and source mapping, then adapt Summary to supply its run data and chunk-specific report body.

**Tech Stack:** Svelte 5, TypeScript, Paraglide messages, existing `PdfViewWindow` and `TableReference` utilities.

---

## Chunk 1: Extract the shared frame and migrate Metrics

### Files

- Create `web/src/lib/components/home3/benchmark-report-viewer.svelte` — shared run header, metadata, score cards, warnings/provenance, evidence controls, resizable report/PDF split, PDF remount/reset behavior, and typed report-body snippet.
- Modify `web/src/lib/components/home3/metric-score-view.svelte` — keep run fetching and metric-specific report rows/source mapping; replace common report and PDF markup with the shared frame.
- Reference `web/src/lib/components/home3/pdf-table-geometry.ts` — use its existing `TableReference` type.

### Task 1: Create the typed report frame

- [ ] Define typed props: `run: { id: string | number; title: string; status: string; documentTitle: string; createdAt: string; subtitle?: string }`; `scoreCards: { label: string; value: string; detail?: string }[]`; `warnings: string[]`; optional `provenance: Snippet`; `evidenceActions?: { id: string; label: string; view(): void; download?(): void }[]`; `inputId: number | null`; and `reportBody: Snippet<[{ selectSource(lines: number[], tableReferences?: TableReference[]): void; selectedSourceLines: number[]; selectedTableReferences: TableReference[] }]>`. Import `Snippet` from `svelte` and `TableReference` from `./pdf-table-geometry`.
- [ ] Render the shared run heading and status, document/timestamp metadata, optional subtitle, score cards, warnings/limitations, optional provenance snippet, and evidence controls in the report column. Move the Metrics report's common split-panel markup and resize behavior into the frame, retaining the same pointer and keyboard controls, responsive styles, and selectable text.
- [ ] Render the existing `PdfViewWindow` in the source pane using its numeric `inputId`, file URL, `renderHighlights`, and table-reference props. Wrap it in a Svelte `{#key}` keyed by `run.id` and `inputId` to remount on either identity change, and use a reactive effect that compares the previous `{ run.id, inputId }` pair and clears selected lines and table references only when that identity pair changes. The remounted viewer starts at page 1/default zoom.
- [ ] Implement `selectSource(lines, tableReferences?)` as replacement semantics: set lines and set table references to the supplied list or empty. Provide both selected values and the callback to the typed report body snippet.
- [ ] Keep optional subtitle, provenance, and evidence controls absent when not supplied. For `inputId === null`, show the existing localized no-source message (add a generic benchmark-viewer message in both Paraglide catalogs if there is no reusable key); for a non-null ID whose PDF request fails, rely on `PdfViewWindow`'s existing loading/error handling.

### Task 2: Migrate Metrics to the shared frame

- [ ] Keep Metrics API calls, run selection, source-entry-to-line/table mapping, filters, and metric-specific details in `metric-score-view.svelte`.
- [ ] Pass Metrics run metadata, score cards, warning/provenance content, evidence actions, input ID, and report-specific content to `BenchmarkReportViewer`. Preserve existing heading labels, metadata text/formatting, score-card labels, values, and fallback text; if the Metrics run lacks a title, document title, timestamp, or score value, use the current localized record/date/score fallback instead of rendering blank metadata or cards.
- [ ] Route current metric source selection through the frame callback; preserve selected-row behavior and table-reference highlighting.
- [ ] Remove only the common split/PDF/report-chrome code and styles made redundant by the migration.

### Task 3: Check the Metrics migration

- [ ] Run `bun run check` from `web/`; expected: Svelte and Paraglide checks complete without errors.
- [ ] Manually open a completed Metrics report and confirm the same report data, evidence controls, source-line and table-reference highlighting, PDF navigation, divider dragging, keyboard resizing, and responsive layout.

## Chunk 2: Adapt Summary and verify parity

### Files

- Modify `web/src/lib/components/home3/summary-score-view.svelte` — adapt the opened report to the shared frame, keep summary-specific score/chunk data and API behavior local, and add chunk source selection.
- Modify `web/messages/en.json` and `web/messages/zh-cn.json` only if the shared frame introduces user-visible labels without an existing reusable message key.

### Task 4: Map Summary run data into the shared frame

- [ ] Map the selected summary run's ID, title, status, document title, created timestamp, score cards, warnings, optional provenance, saved evidence actions, and input ID into the shared props.
- [ ] Move summary chunk judgments and summary-specific report content into the typed report-body snippet; retain Summary API models and score interpretation in the Summary page.
- [ ] For each selectable chunk, derive its source line span from the summary input snapshot and call `selectSource` on activation. If a chunk has no usable lines, keep it readable and clear the source selection.
- [ ] Preserve the current score details and evidence artifact links as actions/controls in the common layout.

### Task 5: Check Summary parity and localization

- [ ] Add English and Chinese Paraglide keys only for genuinely new shared labels, in the same change; reuse existing messages where suitable.
- [ ] Run `bun run check` from `web/`; expected: type/Svelte and i18n checks complete without errors.
- [ ] Manually open a completed Summary report and confirm the Metrics report/PDF split structure, resizing, PDF controls, run metadata, score cards, warnings, evidence, and chunk judgments.
- [ ] Click chunks with source spans and confirm the matching PDF lines highlight; click chunks without spans and confirm the report remains readable with no stale highlight.

## Completion notes

- Do not change scoring APIs, persistence formats, score calculations, document selection, or run-history querying.
- Do not add report schemas for future artifact types in this change; the shared frame's typed props/snippet provide the extension seam.
- Do not add or run automated tests for this task. Run the project-required `bun run check` and the manual UI checks above.
- Commit the plan separately from implementation. Commit implementation changes through `jj` after both chunks are complete.
