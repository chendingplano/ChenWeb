## Context

ADR 2026081001 (implemented in `pipeline-versioning-dependency-graph`) made `kb.pipelines`
immutable and versioned, added real DAG edges (`kb.pipeline_rules.depends_on_processors`) and
creation-time validation (`ValidatePipelineVersion`, `pipeline_version_validate.go`), and defined
the composite **Doc Process DAG** object (DR10): a named pipeline + its rules + optional
bindings. The existing `/kb/pipelines`, `/kb/pipeline-bindings`, `/kb/pipeline-rules` handlers
expose the raw rows but there is no UI and no delete path, and no enforcement of the
"exactly one system default" or "at least one processor" invariants.

This change adds a frontend management page under *Development → System Admin → Doc Process
Pipeline → Doc Process DAG* and the small backend composite API it drives.

Confirmed current schema (read from migrations `20260731000004` → `20260810000008`):
- `kb.pipelines`: `id, name, display_name, description, processors TEXT[], legacy_equivalent,
  is_system_default BOOLEAN (unique partial index `idx_kb_pipelines_one_system_default`),
  version INT, status ('active'|'superseded'), create_time, modify_time`; `UNIQUE(name, version)`.
- `kb.pipeline_rules`: `id, name, priority, pipeline_id FK RESTRICT, predicate, predicate_checksum,
  target_processor, effect ('require'|'enable'|'skip'), required_facets, depends_on_processors TEXT[],
  active, approval_status, ...`.
- `kb.pipeline_bindings`: `id, name, priority, ks_store_id (nullable), pipeline_id FK RESTRICT,
  binding_kind ('store_default'|'conditional'), predicate, predicate_checksum, active,
  tenant_id, user_id, input_record_id, ...`; partial unique index
  `idx_kb_pipeline_bindings_one_active_default_per_context`.

## Goals / Non-Goals

**Goals:**
- A searchable, viewable, CRUD-able Doc Process DAG management page reachable at
  *System Admin → Doc Process Pipeline → Doc Process DAG*.
- Backend guarantees for all six requirements: unique DAG names, ≥1 processor, DR8 validation
  before save, transaction-protected create/modify/delete, exactly one system default.
- A processor catalog endpoint so the form offers the real processor set (union of the Go literal
  and `kb.processor_registry`, via `RegisteredProcessors()`), not a hardcoded frontend list.

**Non-Goals:**
- **No topological-sort execution engine** — `depends_on_processors` stays validated-but-not-
  consumed, exactly as the ADR scoped it.
- **No binding authoring in this page** — knowledge-store bindings are shown read-only on the
  detail view; creating/editing bindings stays with the existing bindings API (a separate concern).
- **No change to the existing `/kb/pipelines` etc. handlers** — they remain for other consumers;
  the new DAG endpoint is an additive higher-level composite.
- **No conditional-gate predicate editor** (ADR DR5's follow-on) — the form supports the
  vacuously-true default gate per processor plus optional `depends_on_processors` edges.

## Decisions

### D1 — A dedicated composite "Doc Process DAG" handler, not frontend calls to raw endpoints

A new `doc_process_dag_handler.go` exposes `GET/POST/PUT/DELETE /api/v1/kb/doc-process-dags`
(plus `GET /api/v1/kb/doc-process-processors`). Rationale: the default invariant (D4) and the
delete path span multiple tables and are easiest to enforce centrally; raw-endpoint composition
from the frontend would duplicate the invariant logic in JS and cannot make the delete atomic.
Alternatives considered: (a) reuse `CreatePipeline`/`UpdatePipeline` from the page — rejected,
the default-flag transfer across versions and the delete don't exist there and would have to be
bolted on anyway; (b) extend the existing handlers — rejected, muddies their contract and risks
the prior spec's "no delete path ever" guarantee by adding one to the low-level pipeline API
rather than the new composite object.

### D2 — List/detail shape: one DAG per pipeline name (current version), with rules and bindings

`GET /doc-process-dags` returns one row per pipeline `name` using its **current** version
(`status='active'`, `version = MAX(...)`), including `is_system_default`, processor count, and
rule summary. `GET /doc-process-dags/:name` returns that pipeline plus its full
`kb.pipeline_rules` (gates + DAG edges) and its `kb.pipeline_bindings` (read-only). Searching is
a `ILIKE '%name%'` filter on the current version's `name`/`display_name`. A name with only
superseded versions is still listed via its latest version (a superseded current row is
informative, not an error). Alternative considered: listing every version — rejected, the page's
unit of management is the named DAG (DR10), versions are a detail surfaced in the view.

### D3 — Create/modify semantics: processor/rule change ⇒ new version; cosmetic ⇒ in-place

`POST` always authors a new version (`version = MAX+1`, marks prior `superseded`, inserts all
rules) in one transaction — matching the existing `CreatePipeline` atomic authoring. `PUT` compares
the submitted processors + rules against the current version: if they differ, it authors a new
version (same transaction path); if only `display_name`/`description`/`is_system_default` change,
it updates the current row in place (matching the existing cosmetic `UpdatePipeline` path).
`name` is immutable on `PUT` (rename = create new + delete old). This preserves DR1/DR2's
"no incremental edit after creation" while giving the page a natural "Save" that does the right
thing.

### D4 — Exactly-one system default, enforced at the application layer

`schema: at most one` is already enforced by `idx_kb_pipelines_one_system_default`. The handler
adds `at least one`, inside the same transaction as the write:
- **Create:** if the request marks `is_system_default=true` and another row currently holds the
  flag, unset that other row in the same transaction. If **no** row currently holds the flag, the
  new DAG is made default automatically (so the invariant never starts empty), honoring an
  explicit `false` only when a default already exists.
- **Modify:** setting default on a non-default unsets the incumbent in the same transaction.
  Unsetting the flag is rejected if this DAG is the *only* default (`ERR last default`).
- **Delete:** deleting the default DAG is rejected (`ERR default cannot be deleted`); the operator
  must first promote another DAG.
- **Version transfer:** authoring a new version of the default DAG carries `is_system_default`
  to the new row and clears it from the superseded row (required to satisfy the partial unique
  index, which spans all versions of all names).

Alternative considered: a CHECK/trigger guaranteeing ≥1 default. Rejected — a partial unique index
can enforce ≤1 natively, but ≥1 across a table that can legitimately be emptied during a migration
window is better enforced where the business rule lives (the write handlers) than as a hard table
constraint that would block legitimate bootstrapping.

### D5 — ≥1 processor and DR8 validation before any write

`ValidatePipelineVersion` gains a leading check that the processor set is non-empty (requirement
3). The full `ValidatePipelineVersion(Processors, Rules)` then runs inside the create/modify
transaction, before commit, so a failing DAG rolls back the whole version (ADR DR8, requirement
4). The handler returns the named validation error verbatim (cycle path, missing producer, missing
fact) so the form can surface it. Adding the check to the shared validator also fixes the
low-level `CreatePipeline` path, which is desirable and backward-compatible (an empty processor
set is meaningless and currently slips past all three DR8 checks as vacuous passes).

### D6 — Delete semantics

`DELETE /doc-process-dags/:name` removes, in one transaction: (1) all `kb.pipeline_bindings`
referencing any version of the name, (2) all `kb.pipeline_rules` for those versions, (3) the
`kb.pipelines` rows themselves. Bindings/rules use `ON DELETE RESTRICT` to `kb.pipelines`, so
order matters (bindings and rules must be removed before their pipeline rows). This is a deliberate
departure from DR1's "no physical delete of a pipeline row" — the ADR's immutability rationale was
"don't delete a version out from under a binding"; a composite DAG delete removes the bindings in
the same atomic unit, and the staging-server context makes destructive ops acceptable. The low-level
`/kb/pipelines` API keeps its no-delete guarantee; only the new DAG object can be deleted.

## Risks / Trade-offs

- **[Risk] Deleting a DAG that downstream systems reference (execution plans, audit events).**
  `kb.doc_process_plans` and policy-audit rows store pipeline name/version as text/JSON, not FKs,
  so no FK blocks the delete and historical rows keep their provenance strings.
  → **Mitigation:** reject deleting the system default; document that old plan/audit rows retain
  the deleted name/version as historical provenance. This matches how the ADR treats persisted
  snapshots (never re-validated).
- **[Risk] Auto-marking the first DAG as default may surprise an operator who intended a
  non-default DAG.** → **Mitigation:** the create form defaults `is_system_default` to `false`
  and the page shows a notice when no default exists ("no default exists — this DAG will become
  the system default").
- **[Risk] The composite handler duplicates rule-parsing/insertion logic already in
  `CreatePipeline`.** → **Mitigation:** accepted duplication is small and keeps the change
  surgical (per ChenWeb CLAUDE.md); the alternative (refactoring shared helpers out of
  `pipelines_handler.go`) would touch the prior ADR's code more broadly.
- **[Trade-off] Name uniqueness is per `(name, version)`; a DAG's identity is its name.** The
  page treats the current version as "the DAG"; a fresh name implies `version=1`. This matches
  DR1's versioning model exactly.

## Migration Plan

No schema migration. Deployment order:
1. Backend: add the empty-processor check to `ValidatePipelineVersion` (with test), add
   `doc_process_dag_handler.go`, register routes.
2. Verify: `go build ./...`, `go vet ./server/...`, `go test ./server/api/doc-processing/... ./server/api/kbhandler/...`.
3. Frontend: add client + view + nav entry + content-panel branch; verify with a Svelte build.
4. Manual smoke against the running dev server (air auto-reloads migrations and code).

Rollback: revert the handler/route registration and the validator check; no data changes are made
outside the new endpoints, so reverting the code fully restores prior behavior. Any DAG rows created
by the page remain valid rows readable by the old `/kb/pipelines` API.

## Open Questions

None outstanding. Exact Go signature/JSON field names for the composite payloads are left to
implementation (they follow the existing `pipelineRecord`/`pipelineRuleDraftPayload` shapes in
`pipelines_handler.go`).
