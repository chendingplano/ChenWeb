## Context

CDM Phase 1 shipped four Go packages — `cdm/model` (AST + validator),
`cdm/rendering` (Typst renderer, anchors, line file, SVG), `cdm/store`
(persistence + publish lifecycle) — and five `kb.cdm_*` tables. All of it is
library-only: no route calls any of it, and `web/src` has no editor.

ADR 2026072603 (accepted) settles the architecture: a Svelte-owned block list
with TipTap confined to inline content (DR1), a thin HTTP API over the Phase 1
packages (DR2), an MVP that is the full authoring loop and nothing else (DR3),
on-demand server-rendered SVG preview (DR4), client-allocated block slugs
(DR5), optimistic concurrency from day one (DR6), routes under `/home3/cdm`
(DR7).

Reading the Phase 1 code to plan this change surfaced three things the ADR
assumed were settled and are not. They are the substance of this design.

**`input_record_id` is dead.** `kb.cdm_documents` has an
`input_record_id BIGINT REFERENCES kb.inputs(id)` column with an index, and no
Go code reads or writes it — `grep` across `server/api/cdm/` returns nothing.
`Store.Save` does not populate it; `Publisher.Publish(ctx, documentKey,
inputRecordID)` requires the caller to supply it from somewhere else. Phase 1's
tests pass because they create both and hold the ID in a local variable. An API
cannot: given a `document_key` from a URL, there is currently no way to find the
document's input row, which is what carries tenant scope, publication state, and
the pipeline handoff.

**`Store.Save` cannot express optimistic concurrency.** It takes only
`*model.Document` and its upsert unconditionally does
`content_version = kb.cdm_documents.content_version + 1`. DR6 cannot be
implemented in the handler with a `Load`-then-`Save`, because that is a
read-modify-write race across two transactions.

**Nothing knows what "published" means.** D8 makes a published document
read-only, but publication state lives in `kb.inputs.status` and `cdm/store`
has no accessor for it. `Store.Save` will happily rewrite a published document.

## Goals / Non-Goals

**Goals:**

- Close the authoring loop end to end: create → edit → save → publish → preview.
- Expose Phase 1 over HTTP without reimplementing validation or persistence.
- Make D8 (published is frozen) and DR6 (optimistic concurrency) real
  invariants enforced in the database transaction, not conventions.
- Keep the editor's in-memory model isomorphic to `[]model.Block` so save is a
  serialization (D11).
- Make D1 structurally true in the editor: a presentation mark should be
  inexpressible, not merely absent.

**Non-Goals:**

- Any feature listed out-of-scope in the proposal (search, annotation, LLM
  tools, reviewers, ontology, chunking, appendices, lifecycle FSM, retention,
  template management, concurrency locks).
- A general-purpose rich-text editor. The surface is a block editor whose inline
  vocabulary is exactly CDM's.
- Multi-user concurrent editing. DR6's version check is a correctness guard
  against lost updates, not a collaboration feature.
- Changing any Phase 1 rendering behavior.

## Decisions

### D1 — Routes are `/api/v1/cdm/...`, not `/api/cdm/...`

ADR 2026072603 DR2 wrote `/api/cdm`. The existing API group is
`e.Group("/api/v1")` with `authmiddleware.AuthMiddleware`
(`server/api/routes.go:226`), and every existing route (`/kb/inputs`,
`/kb/stores`, …) sits under it. CDM joins that group as `/kb`'s sibling:

```text
POST   /api/v1/cdm/documents
GET    /api/v1/cdm/documents
GET    /api/v1/cdm/documents/:key
PUT    /api/v1/cdm/documents/:key
POST   /api/v1/cdm/documents/:key/publish
DELETE /api/v1/cdm/documents/:key
GET    /api/v1/cdm/documents/:key/render
POST   /api/v1/cdm/documents/:key/render-preview
```

Correcting the ADR's path is a documentation fix, recorded here rather than
silently diverging.

_Alternative:_ a separate un-versioned group. Rejected — it would be the only
API in the app outside `/api/v1` and would need its own auth wiring.

### D2 — Creating a document is one transaction that writes both rows and links them

`Store.Create(ctx, doc, DraftInput) (*CreateResult, error)` inserts the
`kb.inputs` row and the `kb.cdm_documents` row in a single transaction and sets
`input_record_id`. This is the fix for the dead column.

Everything downstream depends on the link existing: tenant scoping reads
`kb.inputs.tenant_id`, publish needs the input record ID, and the frozen check
(D4) reads the input row's status. Populating it at creation — the only moment
both rows are being written anyway — is both the cheapest and the only place
where the two cannot diverge.

`InputRegistrar.CreateDraft` currently owns its own `*sql.DB` and commits
independently. It grows a `tx`-accepting variant so both writes join one
transaction; the existing method stays as a thin wrapper so Phase 1 tests are
unaffected.

_Alternative:_ leave `input_record_id` null and resolve the input row by
matching `kb.inputs.title`. Rejected — titles are neither unique nor stable, and
this is exactly the kind of implicit join that breaks silently.

### D3 — `Store.Save` takes an expected `content_version`, checked in-transaction

```go
func (s *Store) Save(ctx context.Context, doc *model.Document, expectedVersion int64) (*SaveResult, error)
```

The upsert's `ON CONFLICT DO UPDATE` gains
`WHERE kb.cdm_documents.content_version = $expected`. A mismatch returns no row,
which the store maps to a typed `*StaleVersionError{Expected, Actual}` after
re-reading the current version. `expectedVersion = 0` means "create" and is only
valid when no row exists.

The check must be inside the same statement as the increment. A handler-side
`Load`, compare, then `Save` is a read-modify-write across two transactions —
two clients both reading version 7 would both pass the check and one would
silently lose its edit, which is precisely what DR6 exists to prevent.

This is a **breaking signature change** to a Phase 1 exported function. Its only
callers are Phase 1's own tests, so the blast radius is contained, but the
change is real and the tests move with it.

_Alternative:_ an `If-Match`/ETag header carrying the version. Rejected as
equivalent in effect but requiring the same store change anyway, plus HTTP
plumbing; the version is a first-class field of the document, not an opaque tag.

### D4 — Frozen state is derived from `kb.inputs`, enforced in `Store.Save`

D8 makes a published document read-only. Publication state is already recorded
in `kb.inputs.status` — `publishedStatus` clears the `doc_processing` entry so
`pipeline_state` derives back to `pending`. Rather than adding a second source
of truth, `Store.Save` joins to the linked input row (available now because of
D2) and refuses to write when the document is published, returning
`*FrozenError{DocumentKey}`.

Enforcing this in the store rather than the handler means every future writer —
the generative tools of D10, an import path, a CLI — inherits it. A handler-only
check would protect exactly one caller.

The editor turns `FrozenError` into the "open a new version?" affordance (D8).

_Note on scope:_ the new-version endpoint itself needs the `kb.inputs`
version-relation field from ADR 2026072602 DR3, which does not exist yet.
**Creating a new version is deferred out of this change**; the MVP surfaces the
frozen state and explains it, but does not yet offer the action. This is a
deliberate reduction from the ADR's DR3 wording, taken because the alternative
is a migration plus a lineage model that the MVP does not otherwise need. The
proposal's "new-version" bullet is narrowed accordingly.

### D5 — `document_key` is server-allocated, slug-derived from the title

`doc:<slugified-title>`, with a numeric suffix on collision. Allocated by the
server at creation because uniqueness is global (CDM §1.1) and only the server
can check it; readability comes from the title the author just typed.

_Alternative:_ a UUID key. Rejected — CDM §1.1 makes `document_key` explicitly
"stable, human-readable document identity," and it appears in export files.

### D6 — Block IDs are minted client-side, uniqueness enforced server-side

Per ADR DR5. The editor derives a slug from heading text or block type plus a
short disambiguator and never changes it after creation. The server does not
trust it: `model.Validate` already rejects empty and duplicate IDs, and
`Store.Save` already maps the `(document_id, block_id)` unique violation to
`*ConflictError` naming the slug. No new server work; the editor must surface
`ConflictError` against the offending block.

### D7 — TipTap is used through `@tiptap/core`, mounted imperatively, with a CDM-only schema

Only `@tiptap/core` and `@tiptap/pm` are taken as dependencies — not
`starter-kit`, which bundles nodes CDM has no equivalent for (horizontal rule,
blockquote-as-mark, heading-as-node) and would let content into the editor that
cannot be serialized back to CDM.

The schema is written by hand and contains exactly CDM's inline vocabulary:

| CDM `Inline.Type`                     | ProseMirror                   |
| ------------------------------------- | ----------------------------- |
| `text`                                | text node                     |
| `strong`, `emphasis`, `code`          | marks                         |
| `link`                                | mark with a `url` attribute   |
| `math`, `citation`, `cross_reference` | atom nodes, rendered as chips |

The document node is a single paragraph — one TipTap instance edits one CDM
block's `[]Inline`, never a sequence of blocks. Block structure is Svelte's.

This is what makes D1 structural: a schema with no `fontSize`, `color`, or
`textAlign` mark cannot produce one, so "no presentation properties" is enforced
by the editor's type system rather than by code review.

TipTap mounts into a DOM element with plain JS, so no framework-specific binding
is required and Svelte 5 compatibility is not a gamble on a wrapper package's
release cadence.

_Alternative:_ `@tiptap/starter-kit`. Rejected as above — convenience that
admits unserializable content.

### D8 — The block list is a Svelte 5 `$state` array of `model.Block`

The editor's document state is `$state<Block[]>` with the same field names and
shapes as the Go struct's JSON encoding. Saving is `JSON.stringify` of the
document wrapper; loading is `JSON.parse`. There is no view model, no
normalization step, and no mapping layer to keep in sync — which is D11's
isomorphism requirement expressed as "there is nothing to translate."

TypeScript types are hand-written to mirror `model.Document` in
`web/src/lib/components/cdm/types.ts`, with a round-trip test against the same
`cdmfixtures` documents the Go tests use, so drift between the two definitions
is caught rather than assumed absent.

_Alternative:_ generate TS types from the Go structs. Rejected for the MVP as
build-toolchain work disproportionate to nine block types; the round-trip test
gives most of the safety at none of the cost. Worth revisiting if the AST grows.

### D9 — Saved artifacts are cached; live drafts render without persistence

`GET /api/v1/cdm/documents/:key/render` returns the SVG pages. On a cache miss
it compiles through the existing `Publisher` path; `kb.cdm_renderings` is
already keyed by `(document_id, content_version, renderer, renderer_version,
page)`, so a repeat request at an unchanged `content_version` is a table read.

`POST /api/v1/cdm/documents/:key/render-preview` accepts the current canonical
document JSON and returns SVG pages without saving the document or writing
rendering artifacts. The editor debounces input, cancels a superseded request
and its Typst subprocess, and applies only the newest completed response.
Existing SVG page roots stay mounted while compilation runs and are patched
synchronously when the response arrives, preventing a blank repaint.

Reusing the SVG path rather than adding an HTML preview renderer means preview
shows exactly the published artifact — same renderer, same template, same
pagination — instead of an approximation that drifts from it.

## Risks / Trade-offs

**Changing `Store.Save`'s signature breaks a Phase 1 API** → Its only callers
are Phase 1's own tests, all in-repo, and the compiler finds every one. The
alternative — a second `SaveWithVersion` alongside the unchecked original —
leaves a footgun that silently skips the concurrency check, which is worse than
a contained break.

**The editor duplicates the CDM type definitions in TypeScript** → Mitigated by
a round-trip test against the shared Go fixtures (D8). Accepted rather than
solved, because codegen is disproportionate at this size; revisit if the AST
grows or a third consumer appears.

**Hand-writing a ProseMirror schema is more work than `starter-kit`** →
Accepted deliberately. The schema _is_ the D1 enforcement mechanism; taking the
convenient bundle would trade away the main reason for choosing ProseMirror.

**Live preview compilation may saturate the server** → Input is debounced and
each new edit cancels the prior request and Typst subprocess. Saved-version
preview remains cached by `content_version`; draft preview is intentionally not
persisted. If latency or load is still unacceptable, the next step is a
long-lived Typst watch session rather than an approximate HTML renderer.

**TipTap is a new frontend dependency** → Confined to one component behind the
CDM inline types, which is the narrowest surface it can occupy and makes
replacing it a rewrite of one file rather than of the editor.

**Deferring the new-version action leaves a dead end** → An author who edits a
published document is told why they cannot and is not offered a way forward.
This is a real gap in the MVP, accepted because the alternative is a migration
and a lineage model out of scope. It is the first thing to build after this
change.

## Migration Plan

No database migration. Every table exists and no column changes.

Deploy order is irrelevant: the API is additive and nothing calls it until the
frontend ships. Rollback is removing the route registrations; no data written by
this change is read by anything else, and drafts sit off both worklists
(CDM §10.1) so an abandoned draft is inert rather than queued.

The one non-additive item is `Store.Save`'s signature (D3), which is internal to
the repo and lands with its callers in the same commit.

## Open Questions

1. **Where the version-relation field lives** when the new-version action is
   built (ADR 2026072602 DR3 offers a dedicated column or
   `kb.inputs.doc_metadata`). Not needed by this change; D4 defers the action.
2. **Whether `Store.Load` should also return the input row's state** rather than
   the handler making a second query. Deferred until the handler exists and the
   query pattern is visible — designing the accessor before seeing two callers
   would be speculative.
3. **Autosave cadence.** The MVP saves explicitly. Whether autosave is added,
   and at what interval, interacts with preview caching and with the eventual
   lock design (D16), so it is left to observation rather than guessed at now.
