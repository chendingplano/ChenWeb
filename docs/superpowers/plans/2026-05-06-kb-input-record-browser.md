# KB Input Record Browser Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a reusable self-fetching left-side `kb.inputs` record browser and adopt it across the `home3/knowledge` sections that currently duplicate record search, pagination, selection, sizing, and settings behavior.

**Architecture:** Introduce a neutral `KbInputRecordBrowser` component plus small helper/settings modules. The browser owns `kb.inputs` listing, direct record retrieval, search dialog integration, reset behavior, pagination, width resizing, and per-instance persisted settings. Each knowledge screen keeps its right-side detail logic and reacts to record selection events from the browser.

**Tech Stack:** Svelte 5, TypeScript/JavaScript modules, existing `kbService.ts`, localStorage-backed per-view settings helpers, Node test runner, `svelte-check`

---

## File Map

### Create

- `web/src/lib/components/home3/kb-input-record-browser.svelte`
  - Reusable left-side browser UI and behavior
- `web/src/lib/components/home3/kb-input-record-browser-settings.js`
  - Per-instance width/page-size persistence and clamp helpers
- `web/src/lib/components/home3/kb-input-record-browser-settings.test.js`
  - Unit tests for persistence helpers
- `web/src/lib/components/home3/kb-input-record-browser-types.ts`
  - Neutral browser card/filter payload types if keeping type surface separate
- `web/src/lib/components/home3/kb-input-record-browser.test.js`
  - Focused browser behavior tests for helper logic and selection rules

### Modify

- `web/src/lib/components/home3/topic-tree-record-browser.js`
  - Rename or fold into the neutral browser helper module
- `web/src/lib/components/home3/topic-tree-record-browser.test.js`
  - Update or replace with neutral browser helper tests
- `web/src/lib/components/home3/topic-tree-view.svelte`
  - Replace embedded record browser with `KbInputRecordBrowser`
- `web/src/lib/components/home3/summary-tree-view.svelte`
  - Replace embedded record browser with `KbInputRecordBrowser`
- `web/src/lib/components/home3/inputs-mgmt-view.svelte`
  - Replace embedded record browser with `KbInputRecordBrowser`
- `web/src/lib/components/home3/metric-mgmt-view.svelte`
  - Replace embedded record browser with `KbInputRecordBrowser`
- `web/src/lib/components/home3/chunk-mgmt-view.svelte`
  - Replace embedded record browser with `KbInputRecordBrowser`
- `web/src/lib/components/home3/doc-structure-view.svelte`
  - Replace embedded record browser with `KbInputRecordBrowser`
- `web/src/lib/components/home3/kb-input-search-dialog.svelte`
  - Add filter state handoff improvements if required by the browser
- `web/src/lib/services/kbService.ts`
  - Expose any shared query parameter types or helpers needed by the browser

### Verify / Test

- `web/src/lib/components/home3/topic-tree-record-browser.test.js`
- `web/src/lib/components/home3/kb-input-record-browser-settings.test.js`
- `web/src/lib/components/home3/summary-tree-state.test.js`
- `web/src/lib/components/home3/knowledge-sections.test.js`

## Chunk 1: Shared Browser Foundations

### Task 1: Neutralize the Existing Record-Browser Helper

**Files:**
- Create: `web/src/lib/components/home3/kb-input-record-browser-settings.js`
- Modify: `web/src/lib/components/home3/topic-tree-record-browser.js`
- Test: `web/src/lib/components/home3/topic-tree-record-browser.test.js`

- [ ] **Step 1: Write failing tests for neutral helper behavior**

Add tests covering:
- default page size `50`
- direct record retrieval query behavior
- first-record auto-selection helper
- filter reset helper returning default query state

Example test shape:

```js
test('selectFirstRecordId returns the first visible record id', () => {
	assert.equal(selectFirstRecordId([{ id: 7 }, { id: 9 }]), 7);
});
```

- [ ] **Step 2: Run the helper tests to verify the new cases fail**

Run:

```bash
node --test web/src/lib/components/home3/topic-tree-record-browser.test.js
```

Expected: FAIL because the neutral helper API and reset/query helpers do not exist yet.

- [ ] **Step 3: Implement the minimal neutral helper changes**

Update the helper module so it provides:
- default page size constant `50`
- shared `listKbInputs` parameter builder
- first-record selection helper
- filter-reset helper
- neutral naming that no longer implies “topic tree”

- [ ] **Step 4: Re-run the helper tests**

Run:

```bash
node --test web/src/lib/components/home3/topic-tree-record-browser.test.js
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git -C /Users/cding/Workspace/ChenWeb add \
  web/src/lib/components/home3/topic-tree-record-browser.js \
  web/src/lib/components/home3/topic-tree-record-browser.test.js
git -C /Users/cding/Workspace/ChenWeb commit -m "refactor: neutralize shared record browser helpers"
```

### Task 2: Add Per-Instance Browser Settings Helpers

**Files:**
- Create: `web/src/lib/components/home3/kb-input-record-browser-settings.js`
- Create: `web/src/lib/components/home3/kb-input-record-browser-settings.test.js`

- [ ] **Step 1: Write failing tests for per-instance settings persistence**

Cover:
- default settings
- page-size clamping
- list-width clamping
- storage-key isolation by `instanceKey`

Example:

```js
test('storage keys are isolated by instance key', () => {
	assert.notEqual(
		createKbInputRecordBrowserSettingsStorageKey('metrics'),
		createKbInputRecordBrowserSettingsStorageKey('chunks')
	);
});
```

- [ ] **Step 2: Run the new settings tests to verify they fail**

Run:

```bash
node --test web/src/lib/components/home3/kb-input-record-browser-settings.test.js
```

Expected: FAIL because the settings module does not exist yet.

- [ ] **Step 3: Implement the settings helper**

Match the style of:
- `web/src/lib/components/home3/chunk-mgmt-settings.js`
- `web/src/lib/components/home3/doc-structure-settings.js`

Include:
- default page size `50`
- min/max page size
- min/max/default list width
- `instanceKey`-based storage key
- merge/clamp/read/write helpers

- [ ] **Step 4: Re-run the settings tests**

Run:

```bash
node --test web/src/lib/components/home3/kb-input-record-browser-settings.test.js
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git -C /Users/cding/Workspace/ChenWeb add \
  web/src/lib/components/home3/kb-input-record-browser-settings.js \
  web/src/lib/components/home3/kb-input-record-browser-settings.test.js
git -C /Users/cding/Workspace/ChenWeb commit -m "feat: add kb input browser settings helpers"
```

## Chunk 2: Reusable Browser Component

### Task 3: Build `KbInputRecordBrowser`

**Files:**
- Create: `web/src/lib/components/home3/kb-input-record-browser.svelte`
- Create: `web/src/lib/components/home3/kb-input-record-browser.test.js`
- Modify: `web/src/lib/components/home3/kb-input-search-dialog.svelte`
- Modify: `web/src/lib/services/kbService.ts`

- [ ] **Step 1: Write failing tests for browser interaction rules**

Add focused tests for:
- Search button exists
- Reset button is disabled with no filters
- page-change auto-selects first record
- page-size setting reloads from page 1
- instance-scoped settings do not bleed across browser instances

If a full Svelte component test harness is not available, keep the component thin and push logic into testable helpers.

- [ ] **Step 2: Run the browser tests to verify they fail**

Run:

```bash
node --test web/src/lib/components/home3/kb-input-record-browser.test.js
```

Expected: FAIL because the component and/or extracted logic does not exist yet.

- [ ] **Step 3: Implement the reusable browser component**

Required behavior:
- top controls: `Record ID`, `Retrieve`, `Search`, `Reset`, `Settings`
- `Search` opens `KbInputSearchDialog`
- `Reset` clears active filters and disables when none are active
- list uses `listKbInputs`
- direct `Record ID` uses `getKbInput`
- pager remains visible when multiple pages exist
- resize handle adjusts browser width
- settings surface persists `pageSize` and `listWidth` by `instanceKey`
- emits selection/results/error/filter events or callback equivalents

- [ ] **Step 4: Keep the component API neutral and minimal**

Verify props include:
- `instanceKey`
- `scopeToActiveStore`
- `selectedRecordId`
- `autoSelectFirstRecord`
- `mapRecord`
- `renderMode`

Verify it does **not** own any right-panel behavior.

- [ ] **Step 5: Re-run the browser tests**

Run:

```bash
node --test web/src/lib/components/home3/kb-input-record-browser.test.js
```

Expected: PASS

- [ ] **Step 6: Run targeted static verification**

Run:

```bash
npm run check -- --fail-on-warnings=false
```

Expected: the repo may still report pre-existing failures, but there should be no new errors attributed to:
- `kb-input-record-browser.svelte`
- `kb-input-record-browser-settings.js`
- any directly touched browser helper files

- [ ] **Step 7: Commit**

```bash
git -C /Users/cding/Workspace/ChenWeb add \
  web/src/lib/components/home3/kb-input-record-browser.svelte \
  web/src/lib/components/home3/kb-input-record-browser.test.js \
  web/src/lib/components/home3/kb-input-record-browser-settings.js \
  web/src/lib/components/home3/kb-input-search-dialog.svelte \
  web/src/lib/services/kbService.ts
git -C /Users/cding/Workspace/ChenWeb commit -m "feat: add reusable kb input record browser"
```

## Chunk 3: Migrate Tree-Style Consumers First

### Task 4: Migrate Topic / Provision Tree View

**Files:**
- Modify: `web/src/lib/components/home3/topic-tree-view.svelte`
- Test: `web/src/lib/components/home3/topic-tree-record-browser.test.js`

- [ ] **Step 1: Write a failing test or assertion for event handoff**

Target behavior:
- when the browser auto-selects the first record, `topic-tree-view.svelte` loads record topics
- provision mode still keeps Search, Reset, pager, resize, and settings through the shared browser

- [ ] **Step 2: Run the focused tests**

Run:

```bash
node --test web/src/lib/components/home3/topic-tree-record-browser.test.js
```

Expected: FAIL or missing coverage for the browser-driven flow.

- [ ] **Step 3: Replace the embedded left panel with `KbInputRecordBrowser`**

Keep local ownership of:
- topic/provision item loading
- PDF target selection
- right-side item detail and PDF display

Remove local ownership of:
- search dialog open state
- `listKbInputs` pagination state
- direct list rendering and pager controls
- width/settings state for the left record browser

- [ ] **Step 4: Re-run the focused tests**

Run:

```bash
node --test web/src/lib/components/home3/topic-tree-record-browser.test.js
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git -C /Users/cding/Workspace/ChenWeb add web/src/lib/components/home3/topic-tree-view.svelte
git -C /Users/cding/Workspace/ChenWeb commit -m "refactor: use shared browser in topic tree view"
```

### Task 5: Migrate Summary Tree View

**Files:**
- Modify: `web/src/lib/components/home3/summary-tree-view.svelte`
- Test: `web/src/lib/components/home3/summary-tree-state.test.js`

- [ ] **Step 1: Write or update a failing test for summary-tree selection handoff**

Verify:
- first selected record from the browser loads summaries
- browser reset and pager behavior remain available on the left panel

- [ ] **Step 2: Run the summary-tree tests**

Run:

```bash
node --test web/src/lib/components/home3/summary-tree-state.test.js
```

Expected: FAIL or missing coverage for browser-driven record selection.

- [ ] **Step 3: Replace the embedded summary-tree record list with `KbInputRecordBrowser`**

Keep local ownership of:
- summary loading
- summary target selection
- PDF detail rendering

- [ ] **Step 4: Re-run the summary-tree tests**

Run:

```bash
node --test web/src/lib/components/home3/summary-tree-state.test.js
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git -C /Users/cding/Workspace/ChenWeb add web/src/lib/components/home3/summary-tree-view.svelte
git -C /Users/cding/Workspace/ChenWeb commit -m "refactor: use shared browser in summary tree view"
```

## Chunk 4: Migrate Detail / Management Screens

### Task 6: Migrate Inputs Management View

**Files:**
- Modify: `web/src/lib/components/home3/inputs-mgmt-view.svelte`

- [ ] **Step 1: Write a failing test or manual verification checklist**

Verify:
- page list still loads
- selected record details still load
- page size/list width persistence is isolated to `Document Details`

- [ ] **Step 2: Replace the embedded list with `KbInputRecordBrowser`**

The parent should keep:
- selected record details
- raw lines loading
- field editing
- right-side document/source viewer

- [ ] **Step 3: Verify the screen**

Run:

```bash
npm run check -- --fail-on-warnings=false
```

Expected: no new errors in `inputs-mgmt-view.svelte`

- [ ] **Step 4: Commit**

```bash
git -C /Users/cding/Workspace/ChenWeb add web/src/lib/components/home3/inputs-mgmt-view.svelte
git -C /Users/cding/Workspace/ChenWeb commit -m "refactor: use shared browser in inputs management view"
```

### Task 7: Migrate Metrics View

**Files:**
- Modify: `web/src/lib/components/home3/metric-mgmt-view.svelte`

- [ ] **Step 1: Write a failing test or manual verification checklist**

Verify:
- browser record selection still triggers metrics + raw-line loading
- `Metrics` browser settings persist separately from other sections

- [ ] **Step 2: Replace the embedded metrics record browser**

Keep local ownership of:
- metric loading
- metric selection
- raw-line highlight computation
- right-side PDF/source rendering

- [ ] **Step 3: Verify the screen**

Run:

```bash
npm run check -- --fail-on-warnings=false
```

Expected: no new errors in `metric-mgmt-view.svelte`

- [ ] **Step 4: Commit**

```bash
git -C /Users/cding/Workspace/ChenWeb add web/src/lib/components/home3/metric-mgmt-view.svelte
git -C /Users/cding/Workspace/ChenWeb commit -m "refactor: use shared browser in metrics view"
```

### Task 8: Migrate Chunks View

**Files:**
- Modify: `web/src/lib/components/home3/chunk-mgmt-view.svelte`

- [ ] **Step 1: Write a failing test or manual verification checklist**

Verify:
- browser record selection still loads chunks
- `Chunks` browser settings do not affect `Metrics`

- [ ] **Step 2: Replace the embedded chunks record browser**

Keep local ownership of:
- chunk loading
- chunk selection
- chunk highlight calculation
- right-side PDF rendering

- [ ] **Step 3: Verify the screen**

Run:

```bash
npm run check -- --fail-on-warnings=false
```

Expected: no new errors in `chunk-mgmt-view.svelte`

- [ ] **Step 4: Commit**

```bash
git -C /Users/cding/Workspace/ChenWeb add web/src/lib/components/home3/chunk-mgmt-view.svelte
git -C /Users/cding/Workspace/ChenWeb commit -m "refactor: use shared browser in chunks view"
```

### Task 9: Migrate Document Structure View

**Files:**
- Modify: `web/src/lib/components/home3/doc-structure-view.svelte`

- [ ] **Step 1: Write a failing test or manual verification checklist**

Verify:
- browser record selection still loads doc-structure lines
- `Document Structure` browser settings do not affect `Chunks`

- [ ] **Step 2: Replace the embedded doc-structure record browser**

Keep local ownership of:
- structure-line loading
- line selection
- line editing
- right-side PDF rendering

- [ ] **Step 3: Verify the screen**

Run:

```bash
npm run check -- --fail-on-warnings=false
```

Expected: no new errors in `doc-structure-view.svelte`

- [ ] **Step 4: Commit**

```bash
git -C /Users/cding/Workspace/ChenWeb add web/src/lib/components/home3/doc-structure-view.svelte
git -C /Users/cding/Workspace/ChenWeb commit -m "refactor: use shared browser in document structure view"
```

## Chunk 5: Final Cleanup and Verification

### Task 10: Remove Dead Browser Duplication

**Files:**
- Modify: `web/src/lib/components/home3/topic-tree-view.svelte`
- Modify: `web/src/lib/components/home3/summary-tree-view.svelte`
- Modify: `web/src/lib/components/home3/inputs-mgmt-view.svelte`
- Modify: `web/src/lib/components/home3/metric-mgmt-view.svelte`
- Modify: `web/src/lib/components/home3/chunk-mgmt-view.svelte`
- Modify: `web/src/lib/components/home3/doc-structure-view.svelte`

- [ ] **Step 1: Delete now-unused browser state and imports**

Remove:
- local `searchOpen`
- local `KbInputSearchDialog` mounting where replaced
- duplicated pager state
- duplicated left-panel width/settings state
- dead helper functions for record list loading that are now owned by the browser

- [ ] **Step 2: Run repository-wide targeted checks**

Run:

```bash
node --test \
  web/src/lib/components/home3/topic-tree-record-browser.test.js \
  web/src/lib/components/home3/kb-input-record-browser-settings.test.js \
  web/src/lib/components/home3/summary-tree-state.test.js \
  web/src/lib/components/home3/knowledge-sections.test.js
```

Expected: PASS

- [ ] **Step 3: Run Svelte static verification**

Run:

```bash
npm run check -- --fail-on-warnings=false
```

Expected: existing unrelated repo failures may remain, but no new failures should point to:
- `kb-input-record-browser.svelte`
- `topic-tree-view.svelte`
- `summary-tree-view.svelte`
- `inputs-mgmt-view.svelte`
- `metric-mgmt-view.svelte`
- `chunk-mgmt-view.svelte`
- `doc-structure-view.svelte`

- [ ] **Step 4: Manual UI verification in `home3/knowledge`**

Check:
- Search button visible in every migrated browser
- Reset disabled until filters are active
- Reset clears filters
- pager visible for multi-page results
- width can be adjusted by dragging
- Settings button opens and persists page size + width
- `Chunks` settings do not affect `Metrics`
- `Document Structure` settings do not affect `Document Details`

- [ ] **Step 5: Commit**

```bash
git -C /Users/cding/Workspace/ChenWeb add \
  web/src/lib/components/home3/topic-tree-view.svelte \
  web/src/lib/components/home3/summary-tree-view.svelte \
  web/src/lib/components/home3/inputs-mgmt-view.svelte \
  web/src/lib/components/home3/metric-mgmt-view.svelte \
  web/src/lib/components/home3/chunk-mgmt-view.svelte \
  web/src/lib/components/home3/doc-structure-view.svelte
git -C /Users/cding/Workspace/ChenWeb commit -m "refactor: adopt shared kb input browser across knowledge views"
```

## Notes for Execution

- Follow `@superpowers:test-driven-development` for each helper/module change before production edits.
- Use the existing settings helper style as the reference implementation.
- Do not widen scope into right-panel refactors.
- Preserve the existing `KbInputSearchDialog`; adapt its data handoff only as needed for shared filter state.
- Prefer extracting browser logic into testable helpers when Svelte component tests would otherwise be brittle.

## Plan Review

Subagent-based plan review is skipped in this session because this thread has not authorized delegated subagents. If review is needed before execution, perform a human review of:

- [2026-05-06-kb-input-record-browser-design.md](/Users/cding/Workspace/ChenWeb/docs/superpowers/specs/2026-05-06-kb-input-record-browser-design.md)
- this implementation plan

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-05-06-kb-input-record-browser.md`. Ready to execute?
