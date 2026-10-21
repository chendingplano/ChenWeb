# Reusable Benchmark Report Viewer Design

## Goal

Make the summary benchmark report viewer use the same page structure and interaction pattern as the Metrics benchmark report viewer. Extract the shared report presentation into a reusable module so future artifact benchmarks can use it while supplying their own report content.

## User-facing structure

The report opened from benchmark history will use the Metrics viewer's split layout: a scrollable report panel on the left and the source PDF on the right, with a draggable/keyboard-adjustable divider. The summary page retains its existing document selection and run-history area; only the opened report viewer is in scope.

The shared report frame owns the common run heading and metadata, score summary cards, warnings/limitations, provenance and saved-evidence controls, split sizing, PDF controls, and the source-highlight state. The frame preserves text selection and supports the current Metrics viewer's responsive layout and keyboard resizing.

Benchmark-specific report sections remain supplied by each benchmark page. Metrics keeps its matched/missed/false-positive metric details and field-accuracy sections. Summaries supplies its chunk judgments and summary-specific score components. The shared frame must allow such content to render in the report column without assuming a metric or summary schema. Future provisions, entities, relations, and inventory item reports should be able to provide their own sections and score cards through the same boundary.

## Source interaction

Selecting a metric detail continues to highlight the associated source lines and table references in the PDF. Selecting a summary chunk highlights the chunk's source line span in the same PDF pane. The viewer accepts the input/document identity and source line selection from its benchmark-specific content; source selection is shared presentation state, while the data mapping stays with each benchmark.

The whole summary review heading supports pointer and keyboard selection. Selection opens the right context panel with production and gold summaries aligned by chunk sequence. Each field occupies one shared row with two columns; differing values have an amber background, matching the Metrics comparison panel. Fields include sequence, level, summary text, English summary, language, keywords, English keywords, source lines, and empty reason. Gold payloads have an implicit leaf level of zero. Missing aligned gold is shown explicitly. The PDF navigates to the first resolved source page and paints visible amber boxes for the union of production and gold source lines, including selections on the current page. The comparison keeps its two columns in a narrow context panel and supports both themes and text selection.

If a selected item has no usable source line numbers or the PDF cannot be loaded, the report content remains usable and the source pane shows the existing empty/error state. Changing the selected run clears the selected source lines/table references and resets the PDF to page 1 at its default zoom.

## Component boundary

Create a reusable benchmark report viewer component in the existing Svelte component area. It owns shared report chrome and source-selection state. Use a typed Svelte interface equivalent to:

- `run`: `{ id, title, status, documentTitle, createdAt, subtitle? }`.
- `scoreCards`: `{ label, value, detail? }[]`.
- `warnings`: `string[]`.
- `provenance`: optional rendered content supplied through a snippet.
- `evidenceActions`: `{ id, label, view(): void, download?(): void }[]`; omit the list when the run has no saved evidence.
- `inputId`: document identity used by the existing PDF component; nullable when no source PDF is available.
- `reportBody`: Svelte typed snippet, declared as `Snippet<[{ selectSource(lines: number[], tableReferences?: TableReference[]): void, selectedSourceLines: number[], selectedTableReferences: TableReference[] }]>`, rendered with `{@render reportBody({ ... })}`. Domain-specific rows/chunks call `selectSource` when activated. Import `Snippet` from `svelte`; import `TableReference` as a type from `./pdf-table-geometry`, the module already used by `shared-pdf-viewer.svelte`.

The frame renders `run.title` and `run.status` in the heading, `documentTitle` and `createdAt` in the metadata line, and `subtitle` when present. When either `run.id` or `inputId` changes, it clears selected source lines and table references and remounts the PDF component keyed by both values; the remounted viewer starts on page 1 at its default zoom. The benchmark-specific component owns how its records map to line spans and table references; the shared viewer owns displaying the current highlight. `selectSource(lines, tableReferences?)` replaces both selection values; omitted table references clear any previous table selection. The report snippet receives both `selectedSourceLines` and `selectedTableReferences`. The PDF pane renders the existing empty state when `inputId` is null or the PDF is unavailable. Optional subtitle, provenance, and evidence are omitted when absent. Avoid a generic schema for every artifact's detailed rows; keep those sections in their benchmark-specific components.

Metrics and Summary are the first consumers. Migrate the Metrics viewer without changing its displayed information or interactions, then make Summary use the same frame. Do not move benchmark selection, run-history querying, score creation, or benchmark-specific API models into the shared viewer.

## Out of scope

- Changing scoring APIs, algorithms, saved report formats, or score calculations.
- Redesigning document cards, filters, run history, or the benchmark setup form.
- Creating report schemas or implementations for provisions, entities, relations, or inventory items now.
- Adding a browser visual companion or changing the PDF rendering implementation.

## Acceptance criteria

1. An opened Summary benchmark report has the same high-level report/PDF split structure as an opened Metrics report, including resizing and PDF navigation.
2. Summary run metadata, score components, warnings, evidence, and chunk judgments remain available in the report column.
3. Clicking a summary chunk highlights its source line span in the PDF; selecting metric details continues to highlight their current source lines/table references.
4. Metrics and Summary render their shared report frame through one reusable component, while their domain-specific sections remain separate.
5. The component boundary is typed and documented well enough for another benchmark page to provide its own report sections without changing the shared viewer.
6. English and Chinese UI strings remain localized through Paraglide, and text remains selectable.

Browser regression coverage: `python3 web/scripts/test-summary-pdf-navigation.py --benchmark` (also run with `--locale zh-cn`) uses a real five-page PDF with mocked benchmark/source APIs. It checks full-row and keyboard selection, visible positioned highlights, same-page and multi-page navigation, aligned comparison columns, field differences, missing gold/source data, and clearing the details panel on close.
