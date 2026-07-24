## 1. Document content model

- [x] 1.1 Create `web/src/lib/documents/types.ts` with the `DocumentType`/`Document` discriminated union (`markdown`, `template-json`, `typst`, `html`), `DocumentTreeNode`, and the `DocumentSource` interface (`listTree()`, `getDocument(id)`).
- [x] 1.2 Create `web/src/lib/documents/render.ts` with `renderDocument(doc): Promise<string>` dispatching on `doc.type`; implement the `markdown` case via `marked.parse(...)`; implement `template-json`/`typst`/`html` as explicit "not yet supported" stubs (throw a clear error, not silent failure).

## 2. User's Manual content and DocumentSource

- [x] 2.1 Create `web/src/lib/content/user-manual/` with a small set of Markdown files (e.g. `getting-started/introduction.md`, `getting-started/installation.md`, `navigating-the-dashboard.md`, `resources.md`) covering real, useful ChenWeb usage content (not lorem ipsum).
- [x] 2.2 Create `web/src/lib/content/user-manual/tree.ts` implementing `DocumentSource` for the manual: load the `.md` files via `import.meta.glob(..., { as: 'raw', eager: true })` (or the project's current raw-import syntax), wrap each as a `MarkdownDocument`, and hand-declare the tree structure (which files nest under which folders).

## 3. Component

- [x] 3.1 Create `web/src/lib/components/home3/user-manual-viewer.svelte` with local `$state` for: selected leaf id, per-folder expand/collapse (`Record<string, boolean>`), and left-panel width. Takes the manual's `DocumentSource` (from task 2.2) as its data source.
- [x] 3.2 Implement the left tree panel from `DocumentSource.listTree()`: folder nodes toggle expand/collapse on click (chevron indicator, matching `nav-rail.svelte`'s `toggleAccordion` pattern); leaf nodes are clickable, set the selected leaf id, and show a selected/active visual state.
- [x] 3.3 Implement the right content panel: on leaf selection, call `DocumentSource.getDocument(id)` then `renderDocument(doc)` (from task 1.2) and render the result via `{@html ...}`; show a "select a page" placeholder when nothing is selected yet.
- [x] 3.4 Implement the vertical drag-to-resize divider between the two panels, reusing the drag-state/clamping approach from `dashboard.svelte` (`isDraggingRail`/`startRailDrag`/`onMouseMove`/`onMouseUp`) scoped locally to this component, with its own min/max width constants.

## 4. Wiring

- [x] 4.1 In `content-panel.svelte`, add an `{:else if activeMenu?.childId === 'docs-users-manual'}` branch (before the generic placeholder fallthrough) rendering `<UserManualViewer />`, following the existing sibling pattern for `videos-training`.
- [x] 4.2 Import `UserManualViewer` in `content-panel.svelte`.

## 5. Verification

- [x] 5.1 Run the frontend dev server, navigate to Resources → Documents → User's Manual, and confirm: folders expand/collapse, clicking a leaf renders its content, and dragging the divider resizes the two panels within their clamped bounds.
- [x] 5.2 Confirm no other `content-panel.svelte` branch, the outer `nav-rail.svelte` tree, or `docs-development` are affected.
- [x] 5.3 Run existing frontend checks (lint/typecheck/build) to confirm no regressions.
