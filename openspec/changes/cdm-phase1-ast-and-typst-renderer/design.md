## Context

SemOS today represents a document as a row in `kb.inputs` plus artifacts
extracted from a parsed file, all anchored by `line_range` provenance
(`kb.metrics`, `kb.provisions`, `kb.entities`, `kb.summaries`, `kb.topics`,
`kb.chunk_ranges`, `kb.search_artifacts`). That model assumes a document arrived
as a file and was parsed. It cannot represent a document SemOS authors itself,
and it has no publication path.

Spec `2026072501-spec-canonical-doc-model` (CDM v1.0) defines a block-based
semantic AST that stores meaning once and derives presentation and retrieval
from it. ADR `2026072501-adr-canonical-doc-model-schema` records its schema
decisions DR1–DR10. This change implements Phase 1 (spec §13.1): the AST, its
validator, its storage, and the Typst renderer.

The dominant constraint is that CDM lands inside a large, live subsystem. The
`kb` schema already has ~50 tables, an established `line_range` provenance
convention, an existing chunking and embedding path, and a doc-processor that
polls `kb.inputs` for work. Phase 1 must add a parallel representation without
disturbing any of it.

The CDM Editor proposal (`2026072502-spec-cdm-editor`) was reviewed against this
design; ADR 2026072602 records the outcome. **It does not change Phase 1 scope.**
Four of its decisions do constrain later work and are worth knowing while
implementing: formatting stays in Typst templates and never enters the AST;
author annotations are ordinary artifacts distinguished by an `origin` flag, not
a new CDM construct; a new document version is a new document with its own
`kb.inputs` row, so `content_version` remains a within-document counter only;
and author-declared chunking is semantic in the AST, resolved to line ranges
after the line file exists.

## Goals / Non-Goals

**Goals:**
- A canonical AST in Go that round-trips to JSON losslessly and is validated
  against the spec §1.2 invariants before it is ever stored.
- Storage for canonical documents that follows existing `kb` conventions and
  collides with nothing already there.
- Authored documents visible to existing knowledge search and artifact tooling
  via a `kb.inputs` row, entering the standard doc-process pipeline on the same
  terms as uploaded documents.
- A deterministic Typst renderer: same document plus same renderer version
  yields byte-identical output.
- An anchor map giving every line-file unit an exact `{page, x, y, w, h}`, so
  navigate-to-location and highlight work identically for CDM and uploaded
  documents.
- A structure that a later phase can extend (projections, semantic blocks, math
  AST) without re-migrating.

**Non-Goals:**
- The CDM Editor (spec §14) — a separate change; this delivers the model it
  will write to. This phase makes the *capability* possible — a draft's
  `kb.inputs` row can host an on-demand doc-processing run without entering a
  worklist, and artifacts are keyed to `content_version` so such a run doesn't
  collide with a later one. It does **not** build the *trigger*: no editor
  action to invoke a processor on demand, and no human/LLM artifact
  reconciliation (ADR 2026072602 DR5b, DR5c) — both are editor-side work.
- Retrieval projections and chunking policy (Phase 2). The
  `kb.cdm_projections` table is created now so the schema is settled, but
  nothing writes to it in this change.
- Semantic blocks (Phase 3), semantic math AST and diagrams (Phase 4).
- HTML, Markdown, and plain-text renderers.
- Any change to ingestion, `line_range` provenance, `kb.chunks`, or embeddings.

## Decisions

### D1 — Scope: authored documents only, coexisting with the ingestion path

CDM v1.0 serves documents SemOS creates (CDM Editor, LLM output, pipeline
output). Uploaded files keep the existing `kb.inputs` + parse + `line_range`
path unchanged.

This is the decision that makes Phase 1 tractable. The alternative — making CDM
the parse target for ingested PDFs and Word documents — would require bridging
block-slug provenance to the `line_range` convention used by roughly a dozen
extractor and search tables, and re-embedding the corpus. Deferring that keeps
this change additive and reversible, and lets the model be proven on content
whose structure we control before it meets messy parser output.

Consequence: two document origins now exist — *uploaded from outside* and
*self-created* — and `kb.inputs.type` is what distinguishes them.

### D2 — `kb.cdm_*` table namespace

Tables are `kb.cdm_documents`, `kb.cdm_blocks`, `kb.cdm_renderings`,
`kb.cdm_projections` — not the spec's original `kb.documents` /
`kb.document_projections`.

Two collisions motivate this. `kb.semantic_projections` already exists and means
something unrelated: a search-oriented enrichment of an input (keywords,
category paths, search vectors) produced by the doc-processor. A CDM projection
is not a summary at all — it is a de-formatted textual rendition of the
document. Two tables whose names both say "projections" but mean different
things will be confused by developers and coding assistants alike. Separately,
a bare `kb.documents` would sit beside `kb.doc_process_runs`, `kb.doc_proc_logs`,
and the `kb.doc_review_*` family, which is already a crowded namespace.

A subsystem prefix follows the convention already used by `kb.artifact_*`,
`kb.benchmark_*`, `kb.inventory_*`, and `kb.doc_review_*`. It disambiguates
every CDM table at once and leaves the spec's vocabulary intact — "projection"
remains the concept name in prose, so no documentation churn.

*Alternative considered:* keep the spec names and document the distinction in
comments. Rejected — the failure mode is silent misuse by a future reader, and a
comment does not prevent it.

### D3 — CDM documents join the standard doc-process pipeline, born parsed

Every CDM document gets a `kb.inputs` row. `input_record_id` is the foreign-key
hub that the artifact and search tables hang off, so a CDM document without one
would be invisible to all existing knowledge tooling.

**One pipeline, regardless of origin.** A CDM document has this lifecycle:

```text
editing → publish → render (anchored) → line-file-generated → doc-process-pipeline
```

From the line file onward this is *exactly* the path an uploaded document takes
after parsing. CDM replaces only the front half — instead of
`PDF → MinerU parse → line file + bboxes`, it does
`CDM AST → Typst render → line file + anchors`. Extraction, artifacts, search,
and review all run unchanged over the result.

The mechanical hazard is that `kb.inputs.parse_state` and `pipeline_state` are
*derived* from the `status` JSONB by `kb.input_status_parse_state` and
`kb.input_status_pipeline_state`, and both default to `'pending'` when `status`
is empty. Two worklists select on them:

- `parse_state = 'pending'` — documents awaiting file parse
  (`server/api/kbhandler/handler.go:679`)
- `parse_state = 'parsed_success' AND pipeline_state = 'pending'` — documents
  awaiting doc-processing (`server/api/kbhandler/handler.go:748`)

A naively inserted CDM row would land on the first worklist and a parser would
attempt to parse a `staging_filename` that does not exist. So a published CDM
document is written *born parsed* but **pipeline-pending**:

```json
[{ "operation": "parsed", "proc_status": "success" }]
```

This yields `parse_state = 'parsed_success'` and leaves
`pipeline_state = 'pending'`, so the document is skipped by the parse worklist
and **picked up by the doc-processing worklist like any other document**. Born
parsed is literally true: the CDM AST *is* the parse result, and the line file is
generated at publish rather than recovered from a PDF.

A document in `editing` **does** have a `kb.inputs` row, created with the
document, because author-triggered extraction in the editor needs somewhere to
attach artifacts before the document is published. A draft is kept off *both*
worklists by writing both derived states terminal:

```json
[{ "operation": "parsed",         "proc_status": "success" },
 { "operation": "doc_processing", "proc_status": "success" }]
```

Publishing then **clears the `doc_processing` entry**, so `pipeline_state`
derives back to `'pending'` and the doc-processing worklist enqueues the
document for its authoritative run. Publish is a status transition on an
existing row, not a row creation.

Note the shape of the draft status is the same combination an earlier draft of
this design proposed for *published* documents. It was wrong there — it would
have excluded authored content from extraction permanently. It is right here,
for a different reason: a draft is processed on demand by an author, never by a
poller.

`type = 'cdm'` marks the origin. `tenant_id` and `ks_store_id` are inherited
from the input row, which also settles spec §16's open question about tenant
scoping of `document_key`.

*Superseded:* an earlier draft of this design marked CDM documents
pipeline-complete (`doc_processing`/`success`) so extraction would never run over
them. That was wrong. It would have split SemOS into two classes of document —
authored content invisible to metrics, provisions, entity, and review tooling —
which is the opposite of the goal. Extraction runs over CDM documents on the
same terms as everything else.

*Alternative considered:* keep `kb.cdm_documents` standalone with no `kb.inputs`
row. Simpler and zero-risk, but authored documents would not appear in knowledge
search — defeating much of the point of authoring inside SemOS.

### D4 — Module ownership: ChenWeb, with a dependency-light core

CDM lives in ChenWeb at `server/api/cdm/`, split into three packages:

- `cdm/model` — canonical types, JSON encoding, validator. No database, no
  ChenWeb imports.
- `cdm/rendering` — Typst renderer. Depends only on `cdm/model`.
- `cdm/store` — persistence and `kb.inputs` registration. The only package that
  touches the database.

CDM's storage is the `kb` schema, which is ChenWeb's, and no other workspace
project needs a document model today. Per the workspace guidance that
`shared/go` is for *truly* common functionality, promoting it now would be
speculative. Keeping `model` and `rendering` free of ChenWeb-specific imports
makes a later move to `shared/go` a package move rather than a rewrite.

### D5 — `semantic_document` JSONB is authoritative; `kb.cdm_blocks` is derived

The full canonical JSON is stored in `kb.cdm_documents.semantic_document`.
`kb.cdm_blocks` is a flattened, derived index of that JSON, rewritten
transactionally whenever the document is written.

This keeps exactly one source of truth and makes the write path trivially
correct — no partial-update reconciliation between a tree and its flattening.
The flattened table exists for block-level queries, provenance, and content
hashing, all read-side concerns. `doc_type`, `rendering_type`, and `authors` are
likewise promoted into columns for filtering and must be written from the same
JSON in the same transaction.

*Trade-off:* rewriting all block rows on every save is more write amplification
than a targeted diff. Acceptable for authored documents, which are saved by a
human at human frequency. If that ever changes, `content_hash` per block is
already in the schema to support a diffing write path.

### D6 — Phase 1 equations use the fallback path

Equations store `original` (the source as authored) with
`parse_status = 'skipped'` and no `normalized` AST. The renderer takes the
spec §6.4 fallback branch: pass through `typst` source, or convert `latex`.

This defers the math parser to Phase 4 while keeping the schema final, so no
migration is needed when the AST arrives. Notably it does **not** mean storing
parallel LaTeX and Typst variants — that is Option A, which ADR DR2 rejects,
and which the spec's own Phase 1 text used to recommend before it was corrected.

### D7 — Determinism is a tested property, not an aspiration

`kb.cdm_renderings` is keyed by
`(document_id, content_version, renderer, renderer_version)` and treated as a
cache. That is only sound if rendering is deterministic, so the renderer must
avoid map iteration order and any other unordered traversal — which is why table
cells are emitted by iterating the declared `columns` slice, never the `cells`
map. Golden-file tests enforce it.

### D8 — Slug collisions are a human's problem

Block IDs are text slugs (ADR DR1). Uniqueness is enforced by the
`(document_id, block_id)` constraint; on violation the store returns a typed
conflict error that the editor surfaces so the author renames the block. No
auto-suffixing, no generated fallback IDs — silent renaming would break the
stable-ID property the slugs exist to provide. Under authored-only scope this
costs almost nothing, since a human is present at creation time.

### D9 — Anchored rendering replaces PDF as the location substrate (verified)

Applications built on the knowledge base need to jump to where an artifact was
extracted and highlight it. Today that works because MinerU emits per-element
bounding boxes alongside the line file, and `shared-pdf-viewer.svelte` paints
`.pdf-highlight` overlays at those boxes over rendered PDF pages. The viewer's
real contract is narrow:

> **`line span → {page, x, y, w, h}` + paginated pages to draw on.**

Typst can satisfy that contract directly, with no PDF in the loop. This was
verified against the installed Typst 0.14.2:

- **Positions.** Wrapping each anchorable unit in a marker that records
  `here().position()` and `measure()`, then reading it back with
  `typst query <label> --field value`, yields exact
  `{id, page, x, y, w, h}` in points for every unit.
- **Pagination.** Units correctly report `page: 2` with page-relative `y` once
  content flows past a page break.
- **Pages to draw on.** `typst compile --format svg` emits per-page SVG whose
  `viewBox` is the page box in the same coordinate space as the query output.
- **Alignment.** A rectangle placed at an externally queried bbox lands exactly
  on its block when rendered — confirmed visually, not just numerically.

So a CDM document renders to *paginated SVG pages plus an anchor map*, and the
existing highlight-overlay technique applies unchanged.

Two properties make this **better** than the PDF path rather than merely equal:
the coordinates are ground truth from the layout engine rather than
reverse-engineered from a rendered PDF; and because CDM generates the line file
*and* the anchors from the same AST in the same pass, the line-span↔location
mapping is exact **by construction**. The existing
`server/cmd/backfill-mineru-list-bboxes` repair tool exists precisely because the
inferred mapping is not exact for uploaded documents.

PDF generation remains available for export and download, but it is no longer on
the critical path for the viewing, navigation, or highlight experience.

### D10 — SVG, not Typst HTML export

Typst 0.14.2 has an `html` export target, and it produces clean semantic HTML.
It is nevertheless the wrong target here, for two verified reasons: it warns
`html export is under active development and incomplete … do not rely on this
feature for production use cases`, and it discards pagination —
`page set rule was ignored during HTML export`. Without pages there are no page
coordinates, and the entire highlight contract collapses.

SVG export keeps the page box exactly, is stable, and renders as vector at any
zoom. Its cost is that text is emitted as glyph references rather than
selectable text, so text selection and in-document search must be served from
the line file and anchor map rather than from the rendered output. That is
acceptable: those features are already driven by the line file for uploaded
documents.

### D11 — Anchor granularity is a line-file unit, with paired start/end marks

Anchors are emitted per *line-file unit* — the same unit that becomes one line
in the generated line file (a paragraph, a list item, a table row, an equation)
— not per top-level block. This makes `line span → anchor` a 1:1 lookup instead
of a range computation.

Each unit emits **two** marks, start and end, rather than one position plus a
measured height. `measure()` reports a unit's height in isolation, which is
wrong whenever the unit breaks across a page: a probe block measured 108pt of
content inside an 80pt page body, so a single-record highlight would have run off
the bottom of page 1 and painted nothing on page 2. Paired marks report the true
truth — start on page 1, end on page 2 — from which per-page highlight fragments
are derived the same way a PDF viewer paints a multi-page selection.

## Risks / Trade-offs

- **CDM rows pollute an existing operational surface** → `kb.inputs` now
  contains rows with no file. Mitigated by `type = 'cdm'` and by the born-parsed
  status; admin views that assume every input has a `staging_filename` should be
  checked during implementation.
- **The status contract is duplicated knowledge** → the born-parsed JSON encodes
  assumptions living in two SQL functions. If those derivations change, CDM
  breaks silently — either reappearing on the parse worklist or vanishing from
  the doc-processing one. Mitigated by an integration test asserting both
  `parse_state` and `pipeline_state` on an inserted CDM row.
- **Anchor drift between line file and render** → the line file and the anchor
  map must be generated from the same AST in the same pass. If they are ever
  produced separately, highlights silently point at the wrong content — the
  failure mode that made `backfill-mineru-list-bboxes` necessary. Mitigated by
  generating both in one operation and testing that every line-file line has
  exactly one anchor.
- **Typst layout changes shift coordinates** → a Typst version upgrade or theme
  edit can move content, invalidating stored anchors. Mitigated by keying the
  anchor map to `renderer_version` and `content_version` alongside the SVG pages,
  so a stale map is detectable rather than silently wrong.
- **PostgreSQL 15 floor** → `UNIQUE NULLS NOT DISTINCT` is required to constrain
  ordinals among top-level blocks. Verify the deployed server version before
  migrating; the fallback is a unique index on
  `COALESCE(parent_block_id, '')`.
- **Write amplification on save** (D5) → acceptable at authoring frequency;
  `content_hash` supports a diffing path later.
- **Empty `kb.cdm_projections`** → the table ships unused in Phase 1. The risk
  is that Phase 2 finds the schema wrong. Accepted deliberately: settling the
  shape now is what keeps Phase 2 from re-migrating.
- **Typst escaping is security-adjacent** → unescaped author text could inject
  Typst markup or `#` directives into rendered output. `escapeTypst` must cover
  the full metacharacter set, with tests for each.

## Migration Plan

1. Add goose migrations under `ChenWeb/project_migrations/` creating the four
   `kb.cdm_*` tables (next available prefix `20260726000001`).
2. Register table creation in `server/api/database/createtables.go` per the
   workspace convention that projects create their own tables.
3. Ship model, validator, store, and renderer. Nothing writes CDM documents
   until the editor or a generator exists, so the code is dormant on deploy.
4. Verify on staging that no CDM input row appears on either worklist.

**Rollback:** the change is purely additive — dropping the five tables and
reverting the package removes it entirely. No existing data is altered, so
rollback carries no data-loss risk.

## Open Questions

1. **Which projection types ship in Phase 2?** The spec describes BM25,
   embedding, and LLM-context projections; whether all three or only the
   embedding projection is needed first is unresolved.
2. **Does the extraction pipeline need any CDM awareness at all?** The intent is
   no: it consumes a line file and emits `source_line_spans`, and CDM supplies a
   line file. This must be confirmed against a real extractor run, since some
   extractors may read MinerU-specific consolidated JSON rather than the line
   file alone.
3. **Line-file dialect.** The generated line file must match whatever shape the
   existing extractors expect from MinerU's canonical `.txt`. The exact
   contract needs to be read off the current implementation before the generator
   is written.
4. **SVG page delivery.** Whether SVG pages are pre-rendered and cached in
   `kb.cdm_renderings` at publish, or rendered on demand, is a performance
   question to settle once page counts are known.
5. **Text selection in the viewer.** SVG output is not selectable text (D10). If
   users must select text in CDM documents, an invisible text layer over the SVG
   — the technique PDF.js uses — is the likely answer, driven by per-unit
   anchors.
