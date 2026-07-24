## Context

`content-panel.svelte` renders the right-hand content area of the shared `Dashboard` shell. When `activeMenu.childId === 'docs-users-manual'` (set by `nav-rail.svelte`'s outer `resourcesNav` tree), it currently falls through to a generic placeholder card (heading + "Select a section..." body text) — there is no dedicated view.

Beyond this one page, the product direction (per user) is broader: ChenWeb will eventually have a general-purpose **Document Editor** where users pick a template, fill it in via a Rich Text Editor, preview, and publish new document pages — not limited to the manual. Pages may end up backed by: a Markdown file, an HTML template whose content is bound to a JSON data blob, a Typst source, or a plain HTML page (e.g. authored by Claude Code). That editor is explicitly a **later** change, but this change must not paint the architecture into a corner: the document/content abstraction introduced here needs to be the one the editor and other renderers plug into later, not something manual-specific that gets thrown away.

Three relevant patterns already exist elsewhere in the codebase and should be reused rather than reinvented:
- **Resizable divider**: `dashboard.svelte` implements drag-to-resize for the outer rail/shelf panels (`isDraggingRail`, `startRailDrag`, `onMouseMove`/`onMouseUp` clamping width between min/max, a 4px `cursor-col-resize` handle with hover highlight + grip dots).
- **Folder expand/collapse**: `nav-rail.svelte` implements this via an `accordionOpen: Record<string, boolean>` state object, a `toggleAccordion(id)` function, and a chevron whose rotation reflects open state.
- **Markdown rendering**: `doc-review-results-view.svelte` already depends on `marked` and renders `await marked.parse(text)` output via `{@html ...}`.

## Goals / Non-Goals

**Goals:**
- Replace the `docs-users-manual` placeholder with a working two-panel viewer: left = manual's own tree, right = selected page's rendered content, with a resizable divider between them.
- Reuse the existing drag-resize and accordion-expand interaction patterns (same state shape/behavior), not the exact outer-rail component.
- Introduce a small, generic **document content model** (`Document` type + `DocumentSource` interface + `renderDocument` registry) that is not manual-specific, so future document types and a future DB-backed source can be added without reworking the viewer or the render call sites.
- Ship with a small real content tree (not lorem ipsum) so the feature is demonstrably working.

**Non-Goals:**
- No Document Editor in this change (no template picker, no Rich Text Editor, no preview/publish flow). This change only plants the `Document`/`DocumentSource` shapes that editor will eventually produce and consume.
- No working renderer for `template-json`, Typst, or plain-HTML document types — they exist as named cases in the `Document` union and in `renderDocument`'s dispatch, each an explicit "not yet supported" stub. Only `markdown` is fully implemented.
- No backend/API/DB changes — the User's Manual's `DocumentSource` is static and bundled with the frontend for this change. (A DB-backed `DocumentSource` is expected later, once the editor can publish into it.)
- No changes to the outer `nav-rail.svelte` tree, to `docs-development`, or to any other `content-panel.svelte` branch.
- No search, versioning, or multi-language manual content in this change.
- No persistence of the internal divider width or tree expand/collapse state across reloads (the outer rail/shelf persist theirs via localStorage; this internal one does not need to, since it's a small in-page tool, not a primary layout dimension).

## Decisions

**1. New self-contained component (`user-manual-viewer.svelte`) rather than extending `nav-rail.svelte`/`dashboard.svelte`.**
The outer rail+shelf already do one resizable-divider job; this is a second, unrelated, nested one scoped to a single content pane. Folding it into `dashboard.svelte` would couple unrelated concerns (outer page chrome vs. one document viewer's internal layout). Alternative considered: extend `dashboard.svelte`'s divider logic to be generic/shared — rejected as premature abstraction for a single caller (CLAUDE.md 1.2: no abstractions for single-use code).

**2. Generic `Document`/`DocumentSource`/`renderDocument` abstraction, in `web/src/lib/documents/` (not under `content/user-manual/`).**
Because the user was explicit that document pages (and the eventual editor) are not manual-specific, the type model and renderer dispatch live in a shared location, separate from the manual's own content/tree. Shape:
```ts
// web/src/lib/documents/types.ts
type DocumentType = 'markdown' | 'template-json' | 'typst' | 'html';

interface MarkdownDocument   { type: 'markdown';      id: string; markdown: string; }
interface TemplateJsonDocument { type: 'template-json'; id: string; templateId: string; data: Record<string, unknown>; }
interface TypstDocument      { type: 'typst';          id: string; source: string; }
interface HtmlDocument       { type: 'html';           id: string; html: string; }
type Document = MarkdownDocument | TemplateJsonDocument | TypstDocument | HtmlDocument;

interface DocumentTreeNode { id: string; label: string; children?: DocumentTreeNode[]; } // leaf: no children

interface DocumentSource {
  listTree(): DocumentTreeNode[];
  getDocument(id: string): Document | undefined;
}
```
```ts
// web/src/lib/documents/render.ts
async function renderDocument(doc: Document): Promise<string> {
  switch (doc.type) {
    case 'markdown':      return marked.parse(doc.markdown);
    case 'template-json': throw new Error(`Document type 'template-json' not yet supported`);
    case 'typst':         throw new Error(`Document type 'typst' not yet supported`);
    case 'html':          throw new Error(`Document type 'html' not yet supported`);
  }
}
```
This is the minimum shape that lets (a) the viewer call one function regardless of document type, and (b) a later `DocumentSource` implementation (DB-backed, populated by the Document Editor's publish step) be swapped in without touching the viewer or `renderDocument`'s call sites — only new `case`s get filled in as each type ships. Alternative considered: skip the abstraction and hardcode Markdown-only for now, generalizing "when we get there" — rejected per the user's explicit ask to plant the mechanism now, since retrofitting a type discriminator onto call sites that assumed raw strings would touch more files later than doing it once now.

**3. User's Manual's own `DocumentSource` is a static bundled-file implementation.**
`web/src/lib/content/user-manual/tree.ts` uses `import.meta.glob('./**/*.md', { as: 'raw', eager: true })` (or the project's current Vite raw-import syntax) to load the `.md` files, wraps each as a `MarkdownDocument`, and hand-declares the small tree structure (which files are grouped under which folders). This is one concrete `DocumentSource`, not the only possible one — the interface exists precisely so this can later be swapped for a DB-backed source without changing `user-manual-viewer.svelte`. Alternative considered: reuse the existing `kbhandler` document-upload/storage pipeline — rejected because that pipeline is for user-uploaded knowledge-base inputs (chunking, embeddings, review), unrelated machinery for what is effectively product documentation.

**4. Tree-expand state and divider width are local `$state` in `user-manual-viewer.svelte`, not lifted to `dashboard.svelte`.**
Nothing outside this component needs to know which manual page is open or how wide its internal split is. Keeping it local avoids widening `dashboard.svelte`'s props/state surface.

**5. Reuse `marked` for the `markdown` case of `renderDocument`, matching `doc-review-results-view.svelte`.**
No new dependency; consistent rendering behavior across the app.

## Risks / Trade-offs

- **[Risk]** `{@html ...}` on rendered document output is an XSS vector if content is ever user-supplied (relevant once a DB-backed source / editor exists). → **Mitigation**: for this change, all documents are developer-authored, bundled at build time from files in the repo, never user input; same trust boundary as `doc-review-results-view.svelte`'s existing use. Sanitization becomes a required design point of the future Document Editor change, not this one.
- **[Risk]** The `Document`/`DocumentSource` shapes are a guess at what the future editor/DB-backed source will need, and may need to change once that work starts. → **Mitigation**: kept deliberately minimal (four fields per type, two methods on the source) so the cost of adjusting them later is small; no consumers exist yet outside this change's own viewer.
- **[Risk]** Divider/tree state resets on navigation away and back (no persistence). → **Mitigation**: accepted for v1; matches the Non-Goals above. Can add `localStorage` persistence later if requested, following the outer rail's existing pattern.
- **[Trade-off]** Bundling content at build time means updating the manual requires a frontend rebuild/deploy, not a live edit. → Acceptable for this change: mirrors how the rest of the app's static UI copy already works; a live-editable path arrives with the Document Editor + DB-backed `DocumentSource`, not here.

## Migration Plan

Additive, frontend-only change behind an existing, already-reachable nav id (`docs-users-manual`). No data migration. Rollback is reverting the `content-panel.svelte` branch and deleting the new component/content/document-model files.

## Open Questions

- Exact shape of `TemplateJsonDocument`/`TypstDocument` and their renderers will firm up once the Document Editor change is scoped — not blocking for this change, since those cases are stubs here.
- Where the future DB-backed `DocumentSource` lives (new `kb.documents`-style table vs. reuse of something existing) is a question for that later change, not this one.
