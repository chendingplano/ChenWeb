## Why

Doc processors are the pipeline's processing units (see the capsule
`KnowledgeStore/Capsules/coding-capsules/doc-processor/+CAPSULE.md` §7 "Doc Processing
Pipeline" — currently 22 processors from `blocking` to `facet_tier2`). There is no database
catalog and no UI to manage them: the roster lives only as documentation and as a Go literal
(`productionProcessorSpecs`) plus the mechanical `kb.processor_registry`. Admins cannot
search, create, update, delete, or enable/disable processors without hand-writing SQL.
This change adds a `kb.doc_processors` table and a management page under
*Development → System Admin → Doc Process Pipeline → Doc Processors*.

## What Changes

- **New table `kb.doc_processors`** (goose migration, auto-applied by the live dev server):
  `name_as_id` (PK, canonical processor name from the capsule §7 table), `display_name`,
  `description`, `type` (`mandatory` | `configurable`), `require_llm` (boolean), `status`
  (`active` | `disabled` | `suspended`), `notes`, `create_time`, `modify_time`. The migration
  **seeds** the table with the current §7 roster (22 processors; the capsule's `routed` /
  `routed (Phase C)` processors are typed `configurable` here — their routing behavior stays in
  `kb.processor_registry` / `kb.pipeline_rules`; `require_llm` and the `mandatory`/`configurable`
  type come straight from the §7 table).
- **New backend CRUD handler** `server/api/kbhandler/doc_processors_handler.go`, registered as
  `/api/v1/kb/doc-processors`:
  - `GET /kb/doc-processors?search=` — list, optional substring filter (ILIKE) on
    `name_as_id` / `display_name`.
  - `POST /kb/doc-processors` — create (name required + unique, `type`/`status` enum
    validated server-side).
  - `PUT /kb/doc-processors/:name` — update editable fields (`name_as_id` is immutable;
    rename = create new + delete old).
  - `DELETE /kb/doc-processors/:name` — delete a row.
- **New frontend page** `web/src/lib/components/home3/doc-processors-view.svelte` (searchable
  list, create/edit modal, delete-with-confirm, status/type/require_llm indicators), wired into
  the nav under *System Admin → Doc Process Pipeline → Doc Processors*, plus a typed
  `doc-processors-client.ts`.
- **No change to the pipeline execution machinery** — the new table is an administrative catalog;
  `kb.processor_registry` and `productionProcessorSpecs` are untouched.

## Capabilities

### New Capabilities
- `doc-processor-management`: search, view, create, update, and delete doc processors in
  `kb.doc_processors`, with unique `name_as_id`, validated `type`/`status` enums, and a seeded
  initial roster matching capsule §7.

### Modified Capabilities
<!-- None — no existing OpenSpec capability is changing (openspec/specs/ is currently empty). -->

## Impact

- **Backend:** `ChenWeb/project_migrations/20260811000001_create_kb_doc_processors.sql` (new),
  `server/api/kbhandler/doc_processors_handler.go` (new), `server/api/routes.go` (route
  registration, ~5 lines).
- **Frontend:** `web/src/lib/components/home3/nav-rail.svelte` (child under the existing
  `sysadmin-doc-process-pipeline` subgroup), `content-panel.svelte` (view branch),
  new `doc-processors-client.ts`, new `doc-processors-view.svelte`.
- **Tests:** new `doc_processors_handler_test.go` (go-sqlmock, following
  `doc_process_dag_handler_test.go` patterns).
- **No shared-library change, no change to existing pipeline handlers.**
