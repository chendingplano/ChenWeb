## Why

SemOS stores documents only as parsed artifacts of uploaded files: `kb.inputs`
plus line-range-anchored extractions. There is no representation for a document
that SemOS itself authors, and no way to publish a document as a well-formatted
PDF. The Canonical Document Model (CDM) spec defines a block-based semantic AST
that stores what a document *means* once and derives presentation and retrieval
from it. This change implements CDM Phase 1 — the AST, its storage, and the
Typst renderer — which is the foundation every later CDM phase and the CDM
Editor depend on.

## What Changes

- Add Go canonical types for the CDM AST: `Document`, `Metadata`, `Block`,
  `Inline`, `TableColumn`, `TableRow`, `Equation`, `MathSource`, `MathExpr`,
  `RefTarget` (spec §5.2), with JSON round-tripping.
- Add a validator enforcing the CDM content-model invariants (spec §1.2):
  ID uniqueness within a document, known block/inline types, the
  content/children/items exclusivity rule, table cell keys as a subset of
  declared columns, and equation well-formedness.
- Add five new tables in a dedicated `kb.cdm_*` namespace via goose migrations:
  `kb.cdm_documents`, `kb.cdm_blocks`, `kb.cdm_renderings`, `kb.cdm_projections`,
  and `kb.cdm_anchors` (spec §11).
- Register every CDM document in `kb.inputs` with `type='cdm'`, born parsed but
  pipeline-pending, so authored documents enter the standard doc-process
  pipeline on the same terms as uploaded ones and never hit the file-parse
  worklist.
- Add a deterministic Typst renderer covering `heading`, `paragraph`, `list`,
  `table`, `code`, `equation`, `image`, and `quote` (spec §13.1), emitting
  Typst source against a theme file.
- Add a publish lifecycle (`editing` → `published` → `rendered` →
  `line_file_generated` → doc-process pipeline) that generates a line file from
  the AST, so existing extractors consume CDM documents with no changes.
- Add an anchor map giving every line-file unit an exact `{page, x, y, w, h}`,
  plus paginated SVG page rendering, so navigate-to-location and highlight work
  identically for CDM and uploaded documents — without requiring a PDF.

Not breaking: no existing table, column, endpoint, or pipeline behavior changes.
CDM is additive and runs alongside the current ingestion path.

## Capabilities

### New Capabilities
- `cdm-document-model`: The canonical block AST — its Go types, JSON encoding,
  identifier rules, and the validator that enforces the content-model
  invariants every consumer relies on.
- `cdm-storage`: Persistence for canonical documents — the `kb.cdm_*` schema,
  content versioning, block extraction, and registration of CDM documents in
  `kb.inputs`.
- `cdm-typst-renderer`: Deterministic rendering of a canonical document to
  Typst source, including the equation fallback path and Typst escaping.
- `cdm-anchored-rendering`: The publish lifecycle, line-file generation, the
  per-unit anchor map with page-relative coordinates, paginated SVG page
  rendering, and resolution of `source_line_spans` to highlight fragments.

### Modified Capabilities
None. No existing spec's requirements change; `openspec/specs/` contains no
capability this change alters.

## Impact

**New code** — CDM package (module ownership resolved in design.md):
canonical types, validator, store, and Typst renderer.

**Database** — five new tables in the `kb` schema via goose migrations under
`ChenWeb/project_migrations/`. Requires **PostgreSQL 15+** for
`UNIQUE NULLS NOT DISTINCT`. `kb.cdm_projections.embedding` is `vector(1536)`,
matching the dimension already used in `kb`.

**Existing systems** — `kb.inputs` gains rows with `type='cdm'`, written
born-parsed so the parse worklist
(`ChenWeb/server/api/kbhandler/handler.go:679`, `parse_state = 'pending'`) never
selects them, and pipeline-pending so the doc-processing worklist
(`handler.go:748`) does. Extractors consume the generated line file and need no
changes. `kb.chunks` / `kb.chunk_ranges` and `kb.semantic_projections` are
untouched and keep serving the uploaded-document path.

**Dependencies** — the Typst toolchain becomes a runtime dependency of publish
(SVG page rendering and anchor extraction via `typst query`), not just an export
convenience. ChenWeb already shells out to `typst compile` in
`server/api/doc-reviews/typst_report.go`, so the pattern and the binary are
established. Verified against Typst 0.14.2.

**Out of scope** — CDM Editor (spec §14); the frontend viewer work to generalize
`shared-pdf-viewer.svelte` into an origin-agnostic viewer with an SVG page
backend (follow-on change — this change delivers the `{page, bbox}` contract it
consumes); retrieval projections and chunking policy (Phase 2); semantic blocks
(Phase 3); semantic math AST and diagrams (Phase 4); and the HTML / Markdown /
plain-text renderers.

**Reference** — spec `2026072501-spec-canonical-doc-model` (CDM v1.0),
ADR `2026072501-adr-canonical-doc-model-schema` (DR1–DR10).
