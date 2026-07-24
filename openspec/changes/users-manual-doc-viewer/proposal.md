## Why

The "User's Manual" leaf under Resources → Documents (`docs-users-manual`) is currently a non-functional placeholder in `content-panel.svelte` (added as a stub by `add-document-nav-page-with-videos`). Users need an actual manual reader: a document tree on one side and the selected document's content on the other, so ChenWeb's own documentation can be browsed in-app instead of staying a dead link.

## What Changes

- Add a new `UserManualViewer` content component, wired into `content-panel.svelte` under `childId === 'docs-users-manual'` (replacing the generic placeholder fallthrough for that id only).
- The component renders **two internal panels** with a **vertical drag-to-resize divider** between them, reusing the existing rail/shelf divider drag pattern from `dashboard.svelte` (`isDraggingRail`/`startRailDrag`-style state, min/max clamped width, 4px `cursor-col-resize` handle with hover highlight and grip dots).
  - **Left panel**: the manual's own menu tree, independent of the outer `nav-rail` tree. Folder nodes (nodes with children) expand/collapse on click via a chevron, matching `nav-rail.svelte`'s existing `accordionOpen`/`toggleAccordion` pattern. Leaf nodes are selectable/highlighted.
  - **Right panel**: renders the selected leaf's content as HTML via a shared, type-aware document renderer (see below).
- Introduce a generic, app-wide **document content model** (not specific to the manual): a `Document` type discriminated by `type` (`markdown` | `template-json` | `typst` | `html`), a `renderDocument(doc)` registry that dispatches on `type`, and a `DocumentSource` interface (`listTree()` + `getDocument(id)`) that supplies documents to any viewer. This is the mechanism the user asked to "plant" now, ahead of:
  - A future **Document Editor** (separate, later change) where users pick a template, fill it in via a Rich Text Editor, preview, and publish new document pages (not limited to the manual) — **out of scope for this change**, but the `Document`/`DocumentSource` shapes are chosen so that a later DB-backed `DocumentSource` (populated by the editor's "publish" step) can be swapped in without changing the renderer contract or the viewer component.
  - Future `template-json` (HTML template bound to JSON data), Typst, and plain-HTML document types — **out of scope for this change**; only `type: 'markdown'` has a working renderer. Other types are represented in the model (so the shape exists) but their renderers are explicit "not yet supported" stubs, not silently missing behavior.
- For this change specifically, the User's Manual's `DocumentSource` is a **static, bundled implementation**: Markdown files under `web/src/lib/content/user-manual/*.md`, loaded via Vite's `import.meta.glob` (raw) and wrapped as `MarkdownDocument`s — no backend/API or DB changes needed for this first version.
- Seed a small real tree (2+ folders, several leaves) with real placeholder-quality manual content (e.g. Getting Started, Navigating the Dashboard, Resources) to prove the interaction end-to-end; not exhaustive end-user documentation.

## Capabilities

### New Capabilities
- `document-content-model`: the app-wide, type-discriminated `Document`/`DocumentSource`/`renderDocument` abstraction. Ships with a working `markdown` renderer; `template-json`, `typst`, and `html` are modeled but not yet rendered (explicit "not yet supported" stubs). Not scoped to the manual — intended for reuse by future document pages and the future Document Editor.
- `users-manual-viewer`: the two-panel (tree + content, resizable divider) UI rendered for the Resources → Documents → User's Manual page, including its expand/collapse tree behavior and leaf-selection behavior. Consumes `document-content-model` rather than rendering Markdown directly; its own `DocumentSource` implementation for this change is the static bundled-file one described above.

### Modified Capabilities
<!-- openspec/specs/ is empty; no previously archived capability's requirements change. The prior `document-page-menu` capability (from add-document-nav-page-with-videos) was never archived to openspec/specs/, so there is no delta spec to write against it. -->
(none)

## Impact

- **Frontend only**:
  - `web/src/lib/components/home3/content-panel.svelte` — new `docs-users-manual` branch rendering the new component instead of the generic placeholder.
  - New generic module `web/src/lib/documents/` (`types.ts` for `Document`/`DocumentSource`, `render.ts` for `renderDocument`) — the reusable content model, not manual-specific.
  - New component `web/src/lib/components/home3/user-manual-viewer.svelte` (tree state, divider drag, content pane), built on top of `web/src/lib/documents/`.
  - New static content directory `web/src/lib/content/user-manual/*.md` plus a tree/`DocumentSource` module for the manual specifically.
  - `marked` is already a dependency (used by `doc-review-results-view.svelte`); no new packages.
- **No changes** to backend, database, routes, `nav-rail.svelte`'s outer tree, or any other `content-panel.svelte` branch.
- **Explicitly deferred** (future changes, not this one): the Document Editor (template picker, Rich Text Editor, preview, publish), a DB-backed `DocumentSource`, and the `template-json`/Typst/HTML renderers.
