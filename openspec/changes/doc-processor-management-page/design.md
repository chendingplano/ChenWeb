## Context

The doc processor pipeline (capsule `+CAPSULE.md` §7) currently has 22 processors — `blocking`
through `facet_tier2` — whose roster lives only as documentation and as the Go literal
`productionProcessorSpecs` plus the mechanical `kb.processor_registry` seam (ADR 2026081001 DR7).
There is no database catalog and no admin UI: an operator who wants to search, create, update,
delete, or enable/disable a processor must hand-edit SQL. The existing `doc-process-dag-management`
change already established the full pattern for a management page under
*System Admin → Doc Process Pipeline* (backend `kbhandler` handler + go-sqlmock tests, typed
Svelte client, view wired into `nav-rail.svelte` / `content-panel.svelte`).

This change adds a `kb.doc_processors` table and a sibling page at
*System Admin → Doc Process Pipeline → Doc Processors*.

## Goals / Non-Goals

**Goals:**
- A `kb.doc_processors` catalog table with exactly the requested fields: `name_as_id`,
  `display_name`, `description`, `type ('mandatory'|'configurable')`, `require_llm`,
  `status ('active'|'disabled'|'suspended')`, `notes`, `create_time`, `modify_time`.
- A searchable, CRUD-able management page reachable at *System Admin → Doc Process Pipeline →
  Doc Processors*, seeded with the current §7 roster so it opens populated.
- Server-side validation of the `type`/`status` enums and unique `name_as_id`, with clean
  error messages surfaced verbatim in the UI.

**Non-Goals:**
- **No pipeline-execution coupling.** `kb.doc_processors` is an administrative catalog; it is
  not consulted by the pipeline at runtime in this change. `kb.processor_registry`,
  `productionProcessorSpecs`, and all existing pipeline handlers are untouched.
- **No versioning / composite semantics** (unlike the DAG): a catalog row is self-contained,
  so the handler is a plain CRUD surface — no transactions spanning multiple tables.
- **No cross-table referential integrity.** Processor names elsewhere
  (`kb.pipeline_rules.target_processor`, `kb.input_proc_status.processor`, JetStream
  `operation`) are strings, not FKs; this catalog does not introduce the first FK.
- **No syncing between the catalog and the Go roster** in either direction.

## Decisions

### D1 — `kb.doc_processors` is a standalone catalog, separate from `kb.processor_registry`

The two tables are keyed by the same canonical processor name but serve different purposes.
`kb.processor_registry` is the pipeline-execution extension seam (phase/class/cost/requires/
produces) with no editorial fields. `kb.doc_processors` is the human-facing admin catalog
(display_name/description/type/require_llm/status/notes). No FK links them — a Go-registered
processor may not yet have a catalog row, and a catalog row may describe a processor before its
spec ships.
**Alternative considered:** extending `kb.processor_registry` with display/status columns.
**Rejected:** it mixes editorial metadata into a hot mechanical seam and would force a CHECK
constraint change on a table the runtime reads on every plan; a separate table keeps the change
surgical.

### D2 — `name_as_id` is the natural primary key

The §7 "Processor Name" is the canonical identity used everywhere else (JetStream
`operation`/`doc-processors`, `kb.pipeline_rules.target_processor`, `kb.input_proc_status.processor`,
`kb.doc_proc_logs`). Making it the PK means the catalog is keyed by exactly the identity the rest
of the system uses, and the page's "name" column is the lookup key.
`name_as_id` is **immutable after creation** (rename = create new + delete old), mirroring how the
DAG handler treats pipeline `name`. No table FKs this catalog, so a surrogate id adds nothing.
**Alternative considered:** bigserial `id` + unique `name_as_id`. **Rejected:** no consumer needs
the surrogate; keeping the PK human-readable simplifies the delete route (`/:name`) and the UI.

### D3 — `type`/`status` are DB CHECK-constrained enums, validated server-side too

```sql
type   VARCHAR(16) NOT NULL CHECK (type   IN ('mandatory','configurable')),
status VARCHAR(16) NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled','suspended'))
```

The handler also validates the submitted value and returns a clean
`errorResponse` message rather than letting a raw `23514 check_violation` leak to the UI. The
capsule's `routed`/`routed (Phase C)` processors are typed `configurable` in the seed (user
decision) — routing behavior is already captured in `kb.processor_registry` / `kb.pipeline_rules`,
and §7's `mandatory`/`configurable` distinction is the axis this catalog manages.

### D4 — Flat CRUD surface, no detail endpoint

A catalog row is fully self-contained, so the handler exposes four verbs only:
`GET /kb/doc-processors?search=` (list, optional ILIKE on `name_as_id`/`display_name`),
`POST /kb/doc-processors` (create), `PUT /kb/doc-processors/:name` (update), and
`DELETE /kb/doc-processors/:name` (delete). **No GET-detail endpoint:** the list returns every
column, so the edit form binds from the list row — the DAG page needed a detail fetch only
because its rules/bindings weren't in the list shape. This keeps the surface minimal per
ChenWeb CLAUDE.md "Simplicity First".

### D5 — Seed the catalog in the same migration, idempotently

The migration `Up` is `CREATE TABLE` + a `INSERT ... ON CONFLICT (name_as_id) DO NOTHING` seeding
all 22 §7 processors: `type` from §7 (routed → `configurable`), `require_llm` from §7,
`status='active'`, humanized `display_name`, `description` = the §7 explanation, `notes` = the
`seqno`/`dependence` provenance (e.g. `"seqno 5 · depends on 3"`) so operators can see pipeline
wiring at a glance. `ON CONFLICT DO NOTHING` makes the seed safe if the live air server applies
the migration while it is still being authored, and never clobbers a later admin edit.

### D6 — Follow the established handler/view conventions

- **Backend:** Echo handlers via `EchoFactory.NewFromEcho(c, "CWB_KB_<CODE>")` with structured
  logging; shared `errorResponse` for `{status, error_msg}` errors; the `ApiTypes.ProjectDBHandle`
  pool; routes registered in `server/api/routes.go` next to the DAG routes; go-sqlmock tests
  mirroring `doc_process_dag_handler_test.go`.
- **Frontend:** Svelte 5 runes view modeled on `doc-process-dag-view.svelte` — debounced search
  box, list with `type`/`status`/`require_llm` badges, create/edit modal (`name_as_id` editable
  only on create), delete-with-confirm, backend error messages surfaced verbatim — plus a typed
  `doc-processors-client.ts` using the same `req<T>` wrapper as `doc-process-dag-client.ts`.
- **Nav:** add a child `sysadmin-doc-process-processors` to the existing
  `sysadmin-doc-process-pipeline` subgroup and a branch in `content-panel.svelte`.

## Risks / Trade-offs

- **[Risk] The catalog can drift from the Go roster / `kb.processor_registry`** (a processor
  shipped in Go without a catalog row, or a catalog row with no implementation yet).
  → **Mitigation:** this catalog is not consumed at runtime in this change, so drift is cosmetic,
  not functional; the page is the tool to close the gap. Documented rather than auto-synced.
- **[Risk] Seed values become stale as §7 evolves.**
  → **Mitigation:** `ON CONFLICT DO NOTHING` means future migrations/edits are never clobbered,
  and admins update rows through the page.
- **[Risk] Deleting a catalog row whose name is referenced elsewhere** (target_processor,
  input_proc_status, doc_proc_logs) leaves dangling name strings.
  → **Mitigation:** those tables already store names as strings with no FK, so nothing blocks the
  delete; the page's delete-with-confirm warns the operator. Consistent with how the rest of the
  system treats processor names.
- **[Risk] The live dev server (air) may auto-apply the migration while it is being authored.**
  → **Mitigation:** finalize the full `Up` (table + seed) before letting it apply; `ON CONFLICT
  DO NOTHING` makes an early apply safe, and any missed statements can be run manually per the
  known workspace caveat (check `project_db_migration` before editing an already-applied file).

## Migration Plan

1. Add `project_migrations/20260811000001_create_kb_doc_processors.sql`
   (`Up`: `CREATE TABLE` + idempotent seed of the 22 §7 processors; `Down`: `DROP TABLE`).
2. Backend: add `doc_processors_handler.go`, register routes, add go-sqlmock tests.
   Verify: `go build ./...`, `go vet ./server/api/kbhandler/...`,
   `go test ./server/api/kbhandler/...`.
3. Frontend: add `doc-processors-client.ts`, `doc-processors-view.svelte`, nav child, and
   content-panel branch. Verify: `cd web && bun run check`.
4. Manual smoke against the live dev server: create a processor, search it, edit it, attempt an
   invalid `type` (rejected with a clean message), delete it.

Rollback: revert the handler/route registration and the frontend nav/branch; drop the table with
the migration `Down`. The catalog is not consumed by the pipeline, so reverting fully restores
prior behavior.

## Open Questions

None. The migration timestamp `20260811000001` is the next free slot after the latest existing
migration `20260810000008`.
