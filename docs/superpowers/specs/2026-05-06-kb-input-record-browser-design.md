# KB Input Record Browser Design

**Date:** 2026-05-06  
**Project:** `ChenWeb`  
**Route Family:** `web/src/routes/home3/knowledge/+page.svelte`  
**Scope:** Reusable left-side `kb.inputs` record browser for `home3/knowledge`

## Goal

Extract the repeated left-side `kb.inputs` browsing UI into a reusable module that can be used by most `ChenWeb/home3/knowledge` sections, including:

- `Document Details`
- `Metrics`
- `Chunks`
- `Document Structure`
- `Document Summaries`
- `Semantic Web`
- `Compliance Provisions`

The reusable module should cover only the left-side record browser. Each section keeps full ownership of its right-side detail, editor, graph, or PDF-inspector behavior.

## Problem

Multiple knowledge pages currently reimplement the same left-panel concerns:

- `Record ID` input and `Retrieve`
- `KbInputSearchDialog` launch and selection handling
- paginated `listKbInputs` browsing
- active-store scoping rules
- first-record auto-selection
- record-card list rendering
- loading, empty, and error states
- pagination controls

This duplication makes the workspace harder to evolve consistently. A bug fix or UX improvement to the browser has to be repeated across many screens, and the current “tree” naming hides that the real shared abstraction is a record browser over `kb.inputs`.

## Recommendation

Create a new self-fetching component:

- `web/src/lib/components/home3/kb-input-record-browser.svelte`

This component owns the shared `kb.inputs` browsing workflow and emits selection/results events to its parent. Each consumer screen reacts to the selected record and loads its own domain-specific data.

This is preferable to a presentational-only component because the data-loading and selection rules are exactly what are duplicated today. Keeping those rules centralized yields the biggest reduction in complexity.

## Non-Goals

- Do not extract the right-side content panes in this change.
- Do not unify topic/summary/metric/chunk/document-structure data loaders.
- Do not redesign the visual language of each section beyond what is needed to make the shared record browser fit.
- Do not force every section into the same record-card layout if a section needs different card copy.

## Component Boundary

### Owned by `kb-input-record-browser.svelte`

- `Record ID` input
- `Retrieve` action
- opening and handling `KbInputSearchDialog`
- `listKbInputs` pagination
- optional active-store scoping
- browser-local loading, empty, and error states
- record selection state
- first-record auto-selection on initial load and page changes
- record-list rendering
- compact pager UI

### Owned by Parent Screens

- what happens after a record is selected
- any additional data fetches for the selected record
- right-panel PDF viewer / editor / graph / metadata UI
- domain-specific wording outside the left browser
- whether the browser should scope to the active knowledge store

## Public API

The reusable browser should expose a small, neutral API centered on records rather than “topics” or “trees”.

### Core Props

- `darkMode?: boolean`
- `title?: string`
  - Default: `kb.inputs`
- `subtitle?: string`
  - Optional helper copy shown near the header or empty state
- `pageSize?: number`
  - Default should match current knowledge patterns
- `scopeToActiveStore?: boolean`
  - Enables `knowledgeStoreState.activeStore?.id` scoping when desired
- `selectedRecordId?: number | null`
  - Allows parent-driven selection when needed
- `autoSelectFirstRecord?: boolean`
  - Default: `true`

### Rendering Props

- `renderMode?: 'compact' | 'cards'`
  - Use only if consumers still need both list styles
- `mapRecord?: (record: KbInputRecord) => BrowserRecordCard`
  - Optional mapper for card display fields so each section can customize title/subtitle/metadata without reimplementing the browser

Suggested `BrowserRecordCard` shape:

- `id: number`
- `title: string`
- `subtitle?: string`
- `meta?: string[]`
- `status?: string`
- `description?: string`
- `badges?: string[]`

### Events / Callbacks

- `onSelect(record: KbInputRecord): void`
- `onResultsChange(payload: { results: KbInputRecord[]; total: number; page: number }): void`
- `onError?(error: Error): void`

If Svelte event dispatch is preferred over callback props, keep the event names equally neutral:

- `select`
- `resultschange`
- `error`

## Internal Behavior

### Load Rules

When no explicit `Record ID` is entered:

- call `listKbInputs`
- apply pagination
- apply active-store scoping only when `scopeToActiveStore` is true
- populate the visible record list

When a `Record ID` is entered:

- call `getKbInput`
- render a one-record result set
- reset browser pagination to page 1 for display consistency

### Selection Rules

- on initial load, auto-select the first record when `autoSelectFirstRecord` is enabled
- on page change, auto-select the first record on the new page
- when a user clicks a record, selection moves to that record
- when search dialog selection returns a record, select that record immediately
- if the current selection disappears because results changed, fall back to the first record when available

### Error Rules

- invalid `Record ID` should stay a local browser validation error
- list/retrieve failures should render a browser-local error state
- the parent should not need to handle browser fetch failures unless it opts into `onError`

## Shared Helper Extraction

Move the existing generic query-building helpers into a neutral companion module:

- `kb-input-record-browser.ts` or `kb-input-record-browser.js`

This helper module can hold:

- browser default page size
- shared `listKbInputs` parameter construction
- first-record selection helpers
- optional display-field mapping helpers

The current `topic-tree-record-browser.js` should be folded into this neutral helper or replaced by it.

## Adoption Plan

### Phase 1: Introduce the Reusable Browser

Build `kb-input-record-browser.svelte` and migrate one tree-style consumer first:

- `topic-tree-view.svelte`

This validates:

- neutral naming
- selection event flow
- pagination behavior
- first-record auto-selection

### Phase 2: Migrate Other Record-Browser Consumers

Migrate the remaining sections that already use the same search-and-list pattern:

- `summary-tree-view.svelte`
- `inputs-mgmt-view.svelte`
- `metric-mgmt-view.svelte`
- `chunk-mgmt-view.svelte`
- `doc-structure-view.svelte`

Each migration should remove local copies of:

- record list paging state
- search dialog state
- record browser rendering
- `listKbInputs` fetch boilerplate

### Phase 3: Rename and Cleanup

After the browser is adopted:

- remove topic-specific helper names that actually belong to generic record browsing
- simplify consumer components so they begin from “selected record” rather than “how records are loaded”
- review whether `renderMode` is still needed or whether one shared browser layout is enough

## Consumer Expectations

After migration, consumer screens should look more like:

1. render `KbInputRecordBrowser`
2. receive the selected record
3. load section-specific data for that record
4. render section-specific right-panel content

This should materially reduce the size and responsibility of:

- [topic-tree-view.svelte](/Users/cding/Workspace/ChenWeb/web/src/lib/components/home3/topic-tree-view.svelte)
- [summary-tree-view.svelte](/Users/cding/Workspace/ChenWeb/web/src/lib/components/home3/summary-tree-view.svelte)
- [inputs-mgmt-view.svelte](/Users/cding/Workspace/ChenWeb/web/src/lib/components/home3/inputs-mgmt-view.svelte)
- [metric-mgmt-view.svelte](/Users/cding/Workspace/ChenWeb/web/src/lib/components/home3/metric-mgmt-view.svelte)
- [chunk-mgmt-view.svelte](/Users/cding/Workspace/ChenWeb/web/src/lib/components/home3/chunk-mgmt-view.svelte)
- [doc-structure-view.svelte](/Users/cding/Workspace/ChenWeb/web/src/lib/components/home3/doc-structure-view.svelte)

## Testing Strategy

Minimum regression coverage should include:

- browser loads paginated `kb.inputs` results
- first record auto-selects on initial load
- first record auto-selects after page changes
- manual record selection emits the selected record
- direct `Record ID` retrieval replaces the list with one record
- active-store scoping is applied only when enabled
- search dialog selection updates the active record
- invalid `Record ID` shows validation feedback

Consumer-level tests should verify only the handoff boundary:

- selected record from the browser triggers the correct section-specific loader
- section-specific right-side UI updates when browser selection changes

## Tradeoffs

### Benefits

- one source of truth for `kb.inputs` browser behavior
- consistent pagination and selection semantics across the knowledge workspace
- smaller consumer components
- easier future UX changes to the shared browser

### Costs

- introduces a new component API that must be kept disciplined
- some consumers may need card-mapping hooks to preserve their current copy
- migration will touch several large Svelte files

## Recommendation Summary

Proceed with a self-fetching reusable left-side record browser named `KbInputRecordBrowser`.

Keep the abstraction narrow:

- shared browser on the left
- section-specific content on the right

This matches the actual duplication in the codebase, removes topic/tree-specific naming from generic infrastructure, and gives most `home3/knowledge` sections a common foundation without over-abstracting the rest of the workspace.
