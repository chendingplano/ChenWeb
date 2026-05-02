# Document Summaries Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the `Document Summaries` section inside `/home3/knowledge` with nested navigation, a mocked-but-testable `Summary Graph` workspace, and a mocked-but-testable document-centric `Summary Tree` workspace.

**Architecture:** Extend the existing `home3/knowledge` route to support a parent `Document Summaries` menu with child views. Keep the first delivery phase frontend-only by introducing summary-specific mock service helpers, small pure state utilities for tab and graph behavior, and two focused Svelte views that reuse the current knowledge shell and PDF viewer patterns. Live filesystem and mutation wiring come later without changing the phase-1 UI contracts.

**Tech Stack:** SvelteKit, Svelte 5 runes, TypeScript, existing `kbService.ts` fetch helpers, shared `home3` PDF viewer/components, `svelte-check`, ESLint, small JS/TS unit tests for pure helpers.

---

## File Structure

### Existing files to modify

- `web/src/routes/home3/knowledge/+page.svelte`
  - extend section typing
  - add nested `Document Summaries` menu behavior
  - render `Summary Graph` and `Summary Tree` views
- `web/src/lib/services/kbService.ts`
  - add summary-domain mock types and phase-1 service entry points

### New files to create

- `web/src/lib/components/home3/summary-types.ts`
  - shared types for category nodes, metadata, summary cards, tree search results, and tab state
- `web/src/lib/components/home3/summary-mock-data.ts`
  - mock category graph, mock category summaries, mock tree search records, mock PDF jump targets
- `web/src/lib/components/home3/summary-graph-state.ts`
  - pure helper functions for category-path tabs, node expansion, and optimistic mock mutations
- `web/src/lib/components/home3/summary-graph-state.test.ts`
  - tests for fixed-tab rules, category-path dedupe, and graph-state mutations
- `web/src/lib/components/home3/summary-tree-state.ts`
  - pure helper functions for tree result view modes and selected-summary PDF targeting
- `web/src/lib/components/home3/summary-tree-state.test.ts`
  - tests for search-result shaping and selection behavior
- `web/src/lib/components/home3/summary-graph-view.svelte`
  - top-level `Summary Graph` workspace
- `web/src/lib/components/home3/summary-graph-tabs.svelte`
  - fixed graph tab plus category-path tabs
- `web/src/lib/components/home3/summary-graph-canvas.svelte`
  - graph / horizontal tree rendering and node action affordances
- `web/src/lib/components/home3/summary-node-dialog.svelte`
  - mocked dialogs for rename, metadata edit, add, delete, merge, and split
- `web/src/lib/components/home3/summary-category-tab.svelte`
  - split panel for summary list and PDF panel
- `web/src/lib/components/home3/summary-card.svelte`
  - reusable summary card UI for category tabs and tree detail lists
- `web/src/lib/components/home3/summary-tree-view.svelte`
  - document-centric summary browser
- `web/src/lib/components/home3/summary-tree-search-dialog.svelte`
  - search dialog mirroring `inputs-mgmt-view.svelte`

### Existing reference files to read during implementation

- `web/src/lib/components/home3/inputs-mgmt-view.svelte`
  - copy the `Document Details` search-dialog pattern
- `web/src/lib/components/home3/doc-structure-view.svelte`
  - mirror the two-panel document workflow and PDF behavior
- `web/src/lib/components/home3/shared-pdf-viewer.svelte`
  - reuse viewer integration patterns instead of inventing a new PDF surface

## Chunk 1: Shared Types and Mock Data Contracts

### Task 1: Define summary-domain types

**Files:**
- Create: `web/src/lib/components/home3/summary-types.ts`
- Modify: `web/src/lib/services/kbService.ts`

- [ ] **Step 1: Add shared frontend types**

Create `summary-types.ts` with exact exported shapes for:

- `SummaryCategoryMetadata`
- `SummaryCategoryNode`
- `SummaryCategoryTab`
- `SummaryRecordCard`
- `SummaryTreeRecord`
- `SummaryGraphAction`

Include fields for:

- category path
- category name
- metadata payload
- child ids
- summary ids
- selected PDF target `{ inputId, page, summaryId }`

- [ ] **Step 2: Add phase-1 mock service response types**

In `kbService.ts`, add exported types for:

- `ListSummaryGraphResponse`
- `GetSummaryCategoryResponse`
- `SearchSummaryTreeResponse`

Keep them near the other KB response types and reuse consistent `status: boolean` payload style.

- [ ] **Step 3: Run type check**

Run: `npm run check`  
Workdir: `ChenWeb/web`  
Expected: PASS with no new type errors.

- [ ] **Step 4: Commit**

```bash
git add web/src/lib/components/home3/summary-types.ts web/src/lib/services/kbService.ts
git commit -m "feat: add document summaries frontend types"
```

### Task 2: Add mock summary data

**Files:**
- Create: `web/src/lib/components/home3/summary-mock-data.ts`
- Modify: `web/src/lib/services/kbService.ts`

- [ ] **Step 1: Add mock category graph data**

Create a small but realistic `SUMMARY_TREE_DIR`-like hierarchy with:

- 2 root categories
- nested subcategories
- metadata
- summary ids

Include at least one long category path to exercise tab truncation.

- [ ] **Step 2: Add mock category summary records**

For at least 3 category paths, define mock summary cards with:

- `pdfFileName`
- `keywords`
- `summaryText`
- `inputId`
- `page`

- [ ] **Step 3: Add mock tree search results**

Create mock `kb.inputs`-style records with summary snippets and PDF targets so the tree page can be fully exercised without backend work.

- [ ] **Step 4: Add temporary mock service helpers**

Export mock-backed async helpers from `kbService.ts`:

- `listSummaryGraphMock()`
- `getSummaryCategoryMock(categoryPath: string)`
- `searchSummaryTreeMock(params)`

Return `Promise.resolve(...)` payloads shaped like real service helpers.

- [ ] **Step 5: Run type check**

Run: `npm run check`  
Workdir: `ChenWeb/web`  
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/lib/components/home3/summary-mock-data.ts web/src/lib/services/kbService.ts
git commit -m "feat: add document summaries mock data services"
```

## Chunk 2: Pure State Helpers and Tests

### Task 3: Implement summary graph tab and mutation helpers

**Files:**
- Create: `web/src/lib/components/home3/summary-graph-state.ts`
- Test: `web/src/lib/components/home3/summary-graph-state.test.ts`

- [ ] **Step 1: Write failing tests for fixed-tab and dedupe rules**

Add tests that assert:

- the initial tab list always contains non-closable `Summary Graph`
- opening a category path creates one tab
- reopening the same category path focuses the existing tab instead of duplicating it

- [ ] **Step 2: Add failing tests for mock graph mutations**

Add tests for:

- expand/collapse toggle
- rename updates display name only for target node
- delete removes a node from parent children

Keep merge and split tests small and deterministic using mock ids.

- [ ] **Step 3: Run the targeted tests to verify failure**

Run: `npm exec vitest --run web/src/lib/components/home3/summary-graph-state.test.ts`  
Workdir: `ChenWeb/web`  
Expected: FAIL because helper functions do not exist yet.

- [ ] **Step 4: Implement the pure helpers**

Create:

- `createSummaryGraphTabs()`
- `openCategorySummaryTab()`
- `toggleNodeExpanded()`
- `renameNode()`
- `addChildNode()`
- `deleteNode()`
- `mergeNodes()`
- `splitNode()`

Keep them framework-agnostic so Svelte views only orchestrate them.

- [ ] **Step 5: Re-run targeted tests**

Run: `npm exec vitest --run web/src/lib/components/home3/summary-graph-state.test.ts`  
Workdir: `ChenWeb/web`  
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/lib/components/home3/summary-graph-state.ts web/src/lib/components/home3/summary-graph-state.test.ts
git commit -m "feat: add summary graph state helpers"
```

### Task 4: Implement summary tree selection helpers

**Files:**
- Create: `web/src/lib/components/home3/summary-tree-state.ts`
- Test: `web/src/lib/components/home3/summary-tree-state.test.ts`

- [ ] **Step 1: Write failing tests for tree selection**

Add tests that assert:

- result view mode toggles between compact and card layouts
- selecting a record updates the active record id
- selecting a summary maps to the right PDF target

- [ ] **Step 2: Run the targeted tests to verify failure**

Run: `npm exec vitest --run web/src/lib/components/home3/summary-tree-state.test.ts`  
Workdir: `ChenWeb/web`  
Expected: FAIL.

- [ ] **Step 3: Implement the pure helpers**

Create:

- `toggleSummaryTreeListMode()`
- `selectSummaryTreeRecord()`
- `selectRecordSummaryTarget()`

- [ ] **Step 4: Re-run targeted tests**

Run: `npm exec vitest --run web/src/lib/components/home3/summary-tree-state.test.ts`  
Workdir: `ChenWeb/web`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/components/home3/summary-tree-state.ts web/src/lib/components/home3/summary-tree-state.test.ts
git commit -m "feat: add summary tree state helpers"
```

## Chunk 3: Summary Graph Workspace UI

### Task 5: Build the tab shell and graph canvas

**Files:**
- Create: `web/src/lib/components/home3/summary-graph-view.svelte`
- Create: `web/src/lib/components/home3/summary-graph-tabs.svelte`
- Create: `web/src/lib/components/home3/summary-graph-canvas.svelte`
- Modify: `web/src/lib/services/kbService.ts`

- [ ] **Step 1: Create the top-level graph workspace**

Implement `summary-graph-view.svelte` with:

- same knowledge-page color token style used by nearby components
- fixed `Summary Graph` tab
- local state bootstrapped from `listSummaryGraphMock()`
- active tab tracking

- [ ] **Step 2: Create the tab strip**

Implement `summary-graph-tabs.svelte` with:

- non-closable first tab
- closable category-path tabs
- truncated labels with `title` attribute for full path

- [ ] **Step 3: Create the graph canvas**

Implement `summary-graph-canvas.svelte` with a mocked horizontal tree layout that supports visible actions for:

- expand/collapse
- rename
- metadata
- delete
- add
- merge
- split
- show summaries

Use simple layout primitives first; do not pull in a graph library in phase 1.

- [ ] **Step 4: Wire `show summaries` into tab opening**

Clicking `show summaries` must call `openCategorySummaryTab()` and activate the resulting category-path tab.

- [ ] **Step 5: Run global check**

Run: `npm run check`  
Workdir: `ChenWeb/web`  
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/lib/components/home3/summary-graph-view.svelte web/src/lib/components/home3/summary-graph-tabs.svelte web/src/lib/components/home3/summary-graph-canvas.svelte web/src/lib/services/kbService.ts
git commit -m "feat: add summary graph workspace shell"
```

### Task 6: Build mocked graph action dialogs and category tabs

**Files:**
- Create: `web/src/lib/components/home3/summary-node-dialog.svelte`
- Create: `web/src/lib/components/home3/summary-category-tab.svelte`
- Create: `web/src/lib/components/home3/summary-card.svelte`
- Create: `web/src/lib/components/home3/summary-category-tab.svelte`
- Modify: `web/src/lib/components/home3/summary-graph-view.svelte`

- [ ] **Step 1: Build the reusable summary card**

Implement `summary-card.svelte` for:

- file name
- keyword pills
- summary text preview
- selected state

- [ ] **Step 2: Build the mocked graph action dialog**

Implement one dialog component that can switch by action mode for:

- rename
- edit metadata
- add
- delete confirm
- merge confirm
- split editor

Back it with mock state helpers only.

- [ ] **Step 3: Build the category summary tab**

Implement the split layout with:

- resizable left summary list
- right PDF panel
- selected summary driving mocked PDF target state

Use `shared-pdf-viewer.svelte` if the mocked target can be expressed through current props; otherwise use a placeholder shell with the same dimensions and a clear TODO comment.

- [ ] **Step 4: Wire category tabs into the main graph workspace**

Render `summary-category-tab.svelte` when the active tab is a category-path tab.

- [ ] **Step 5: Run global check**

Run: `npm run check`  
Workdir: `ChenWeb/web`  
Expected: PASS.

- [ ] **Step 6: Manual verification**

Verify in browser:

- fixed graph tab cannot be closed
- opening the same category twice reuses the tab
- selecting summary cards updates the right panel state

- [ ] **Step 7: Commit**

```bash
git add web/src/lib/components/home3/summary-node-dialog.svelte web/src/lib/components/home3/summary-category-tab.svelte web/src/lib/components/home3/summary-card.svelte web/src/lib/components/home3/summary-graph-view.svelte
git commit -m "feat: add summary graph action dialogs and category tabs"
```

## Chunk 4: Summary Tree Workspace UI

### Task 7: Build the document-centric browser shell

**Files:**
- Create: `web/src/lib/components/home3/summary-tree-view.svelte`
- Create: `web/src/lib/components/home3/summary-tree-search-dialog.svelte`
- Modify: `web/src/lib/services/kbService.ts`

- [ ] **Step 1: Build the tree page shell**

Implement `summary-tree-view.svelte` with:

- left search launch / filters summary
- left results list
- list mode toggle for compact vs card
- right detail / PDF surface

- [ ] **Step 2: Mirror the Document Details search dialog**

Use `inputs-mgmt-view.svelte` as the direct reference and reproduce:

- identity section
- processing section
- time-window section
- store-scope header copy
- reset and search toolbar

The search action should call `searchSummaryTreeMock(...)`.

- [ ] **Step 3: Render mock search results**

Support:

- one-line list rows
- richer blocks with title, file name, and summary snippet

- [ ] **Step 4: Wire result and summary selection**

Selecting a record should populate the right panel. Selecting a summary snippet should update the mocked PDF target.

- [ ] **Step 5: Run global check**

Run: `npm run check`  
Workdir: `ChenWeb/web`  
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/lib/components/home3/summary-tree-view.svelte web/src/lib/components/home3/summary-tree-search-dialog.svelte web/src/lib/services/kbService.ts
git commit -m "feat: add summary tree workspace"
```

## Chunk 5: Route Integration and Final Verification

### Task 8: Integrate nested menu into `/home3/knowledge`

**Files:**
- Modify: `web/src/routes/home3/knowledge/+page.svelte`
- Modify: `web/src/lib/components/home3/knowledge-store-state.svelte.ts` (only if needed for section defaults)

- [ ] **Step 1: Extend section ids**

Add:

- `kb-document-summaries-graph`
- `kb-document-summaries-tree`

and define a parent nav shape that can render children.

- [ ] **Step 2: Implement nested menu rendering**

Refactor the menu loop so `Document Summaries` is a non-leaf item with two child buttons.

- [ ] **Step 3: Update page header behavior**

Ensure the header title reflects the active child page:

- `Summary Graph`
- `Summary Tree`

while still living under the `Knowledge System` shell.

- [ ] **Step 4: Render the new views**

Wire:

- `SummaryGraphView`
- `SummaryTreeView`

into the existing section switch.

- [ ] **Step 5: Preserve active-store gating**

Both summary pages must require an active knowledge store in the same way as other non-search knowledge pages.

- [ ] **Step 6: Run global verification**

Run: `npm run check`  
Workdir: `ChenWeb/web`  
Expected: PASS.

Run: `npm run lint`  
Workdir: `ChenWeb/web`  
Expected: PASS.

- [ ] **Step 7: Manual verification**

Verify:

- desktop nested nav behavior
- mobile stacked nav behavior
- category-path tab reuse
- long tab title truncation with hover tooltip
- summary tree search dialog parity with `Document Details`
- dark and light mode rendering

- [ ] **Step 8: Commit**

```bash
git add web/src/routes/home3/knowledge/+page.svelte web/src/lib/components/home3
git commit -m "feat: add document summaries knowledge views"
```

## Chunk 6: Phase 2+ Follow-Up Backlog

### Task 9: Capture post-phase-1 implementation handoff

**Files:**
- Modify: `docs/superpowers/specs/2026-05-01-document-summaries-design.md` (only if scope decisions changed during implementation)
- Optional docs note in PR description

- [ ] **Step 1: List remaining backend dependencies**

Document the exact backend APIs or filesystem adapters still needed for:

- reading `SUMMARY_TREE_DIR`
- reading category summaries
- real PDF jump mapping
- write operations for rename/add/delete/merge/split

- [ ] **Step 2: Confirm no mock-only UX leaked into public contracts**

Review labels and comments so mock wording is internal-only unless the user explicitly wanted placeholder badges.

- [ ] **Step 3: Final commit if docs changed**

```bash
git add docs/superpowers/specs/2026-05-01-document-summaries-design.md
git commit -m "docs: capture document summaries follow-up work"
```
