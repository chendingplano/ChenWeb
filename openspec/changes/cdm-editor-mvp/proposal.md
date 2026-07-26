## Why

CDM Phase 1 delivered the engine for authored documents — the block AST, the
validator, the Typst renderer with anchored rendering, and storage with a
publish lifecycle — but every one of those is a Go library that nothing calls.
No route in `server/api/routes.go` mentions CDM, and `web/src` contains no
editor. A browser cannot reach any of it. So SemOS can render and publish an
authored document but has no way for a human to author one.

This change builds the smallest thing that closes that loop end to end: an HTTP
API over the Phase 1 packages, and an editor UI that uses it. Its value is
partly the feature and partly the proof — it exercises every architectural seam
(AST↔editor mapping, API, validation, versioning, render) while adding no new
backend capability, so if the spine works the remaining editor features are
ordinary feature work against it.

## What Changes

- Add `server/api/cdmhandler`, a thin HTTP layer over the existing
  `cdm/model`, `cdm/rendering`, and `cdm/store` packages. Validation and
  persistence stay where they already live; handlers do not reimplement either.
- Register CDM routes in `server/api/routes.go` behind the same auth middleware
  the existing `kbhandler` routes use.
- Carry **canonical CDM JSON itself** (`model.Document`) as the request and
  response body for document load and save, rather than a parallel DTO.
- Return validation failures as the structured `model.ValidationError` violation
  list, so the editor can attribute each violation to the block that caused it.
- Enforce **optimistic concurrency** on save: the client sends the
  `content_version` it loaded, and a stale write is rejected rather than
  silently overwriting.
- Reject saves to a published (frozen) document, and add an endpoint that opens
  a new version of one, carrying the relation type.
- Add a SvelteKit editor at `/home3/cdm` (document list) and `/home3/cdm/[key]`
  (the editor), following the existing `home3` route convention.
- Build the editing surface as a **Svelte-owned block list** holding
  `[]model.Block`, with **TipTap confined to the inline content** of
  `paragraph`, `heading`, and `quote`. Structured blocks (`table`, `list`,
  `code`, `equation`, `image`, `callout`) get purpose-built editors.
- Constrain the TipTap schema to CDM's inline vocabulary only, so a presentation
  mark is not merely discouraged but inexpressible.
- Allocate block ID slugs client-side at block creation, validated server-side.
- Add **preview** as server-rendered Typst SVG pages, on demand — the same
  artifact publishing produces, not an HTML approximation of it.
- Add `@tiptap/*` as a `web/` dependency.

Not breaking: no existing table, column, endpoint, route, or pipeline behavior
changes. Every CDM table already exists; this change adds no migration.

## Capabilities

### New Capabilities
- `cdm-http-api`: The HTTP surface over the Phase 1 CDM packages — document
  create, list, load, save, publish, new-version, delete, and preview render;
  the canonical-JSON wire contract; structured validation errors; optimistic
  concurrency; and the frozen-document rule.
- `cdm-editor-ui`: The browser editing surface — the block list that owns
  document structure and block identity, the constrained inline editor, the
  structured editors for non-text block types, client-side slug allocation, and
  the save/publish/preview interactions.

### Modified Capabilities
None. No existing capability's requirements change. The four Phase 1
capabilities (`cdm-document-model`, `cdm-storage`, `cdm-typst-renderer`,
`cdm-anchored-rendering`) are consumed as-is; this change adds a caller, not a
behavior change.

## Impact

**New code** — `server/api/cdmhandler/` (handlers plus route registration in
`server/api/routes.go`); `web/src/routes/home3/cdm/` (list and editor pages);
`web/src/lib/components/cdm/` (block list, block editors, inline editor, the
CDM↔ProseMirror mapping).

**Database** — none. `kb.cdm_documents`, `kb.cdm_blocks`, `kb.cdm_renderings`,
`kb.cdm_anchors`, and `kb.inputs` all exist. The version-relation field on
`kb.inputs` (ADR 2026072602 DR3) is needed by the new-version endpoint and is
additive; design.md settles whether it lands here or is deferred with the
endpoint.

**Dependencies** — TipTap (`@tiptap/core`, `@tiptap/pm`, `@tiptap/starter-kit`
or a narrower node/mark set) added to `web/package.json`. Confined to one
component, which is the narrowest place to take the dependency and the easiest
to replace. Typst is already a runtime dependency of publish; preview reuses it.

**Existing systems** — creating a draft writes a `kb.inputs` row in the
terminal-state form CDM §10.1 requires, so drafts stay off both worklists;
publish is a status transition that hands the document to the standard
doc-processing worklist. Both behaviors already exist in `cdm/store` and are
exposed, not re-implemented.

**Out of scope**, each already deferred or gated by spec `2026072502`: search
(§2.1), semantic annotation (§3.2 — gated on artifact types D2 does not add),
document reviewers (§3.3), all LLM tools (§3.4–§3.6, §3.11–§3.12), ontology
(§3.8), author-declared chunking (§3.9), artifact appendices (D5b), the
lifecycle FSM (D6), retention (§2.7), template management (§2.8), and
concurrency locks (D16 — only its optimistic-concurrency guard is in scope).

**Reference** — ADR `2026072603-adr-cdm-editor-frontend` (accepted, DR1–DR7),
spec `2026072502-spec-cdm-editor` (D1, D8, D9, D10, D11, D16, D17), spec
`2026072501-spec-canonical-doc-model` §5.7, §10.1.
