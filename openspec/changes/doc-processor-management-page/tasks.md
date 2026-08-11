## 1. Database migration

- [x] 1.1 Create `project_migrations/20260811000001_create_kb_doc_processors.sql`:
  `Up` = `CREATE TABLE IF NOT EXISTS kb.doc_processors (name_as_id VARCHAR(128) PRIMARY KEY,
  display_name VARCHAR(255) NOT NULL, description TEXT, type VARCHAR(16) NOT NULL CHECK
  (type IN ('mandatory','configurable')), require_llm BOOLEAN NOT NULL DEFAULT false,
  status VARCHAR(16) NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled','suspended')),
  notes TEXT, create_time TIMESTAMPTZ NOT NULL DEFAULT NOW(), modify_time TIMESTAMPTZ NOT NULL
  DEFAULT NOW());` and `Down` = `DROP TABLE IF EXISTS kb.doc_processors;`
- [x] 1.2 Add the idempotent seed to the migration `Up`: `INSERT ... ON CONFLICT (name_as_id)
  DO NOTHING` for all 22 processors from capsule §7 (blocking, structure_analyzer, chunking,
  extract_metadata, extract_metrics, extract_provisions, extract_semantic_projections,
  generate_summaries, generate_topics, generate_scene_blocks, extract_entity_relation,
  extract_inventory_items, review_document, extract_metric_definitions, extract_test_methods,
  extract_product_structure, normalize_assertions, associate_semantics, project_semantics,
  classify_document, facet_tier1, facet_tier2) with `type`/`require_llm` from §7 (routed → 
  `configurable`), `status='active'`, humanized `display_name`, §7 explanation as `description`,
  and `seqno`/`dependence` provenance in `notes`.
- [x] 1.3 Verify the migration applies cleanly (live dev server's air applies it; check
  `project_db_migration` if effects don't show) and that a re-run is a no-op (`ON CONFLICT`).

## 2. Backend: doc processors CRUD handler

- [x] 2.1 Create `server/api/kbhandler/doc_processors_handler.go` with:
  - `docProcessorRecord` (name_as_id, display_name, description, type, require_llm, status,
    notes, create_time, modify_time) and `docProcessorListResponse` / `docProcessorResponse`.
  - `ListDocProcessors` — `GET` all rows, optional `?search=` ILIKE on `name_as_id`/`display_name`,
    ordered by `name_as_id`.
- [x] 2.2 `CreateDocProcessor` — parse payload; require non-empty `name_as_id`; validate
  `type`/`status` against the enums; reject duplicate `name_as_id` (check + return clean message);
  `INSERT` with `status` defaulting to `active`; return the created record.
- [x] 2.3 `UpdateDocProcessor` — by `:name`; return not-found for unknown names; update only
  editable fields (`display_name`, `description`, `type`, `require_llm`, `status`, `notes`);
  validate enums; set `modify_time = NOW()`; `name_as_id` is immutable (not updatable).
- [x] 2.4 `DeleteDocProcessor` — by `:name`; return not-found for unknown names; `DELETE` the row;
  return the number deleted.
- [x] 2.5 Follow the house style: `EchoFactory.NewFromEcho(c, "CWB_KB_<CODE>")` + structured
  logging, shared `errorResponse` for errors, `ApiTypes.ProjectDBHandle` pool, and reuse the
  existing `decodeStringValue` / `decodeStringArrayValue` helpers where applicable.

## 3. Backend: routes + tests

- [x] 3.1 Register routes in `server/api/routes.go` beside the DAG routes:
  `GET/POST /kb/doc-processors`, `PUT/DELETE /kb/doc-processors/:name`.
- [x] 3.2 Write `server/api/kbhandler/doc_processors_handler_test.go` (go-sqlmock, modeled on
  `doc_process_dag_handler_test.go`) covering: list with/without search, create success, create
  missing-name rejection, create duplicate-name rejection, create invalid-type/invalid-status
  rejection, update success (modify_time refreshed), update unknown-name not-found, update
  rejects changing `name_as_id`, delete success, delete unknown-name not-found.
- [x] 3.3 Verify: `go build ./...`, `go vet ./server/api/kbhandler/...`,
  `go test ./server/api/kbhandler/...` (all `DocProcessor` tests pass; a set of pre-existing
  search/registry/summary test failures exist in this suite — column-count mismatch in the
  search handler — unrelated to this change, last touched by older commits).

## 4. Frontend: client + view + nav

- [x] 4.1 Create `web/src/lib/components/home3/doc-processors-client.ts` using the same `req<T>`
  wrapper as `doc-process-dag-client.ts`: `DocProcessor` type + `listDocProcessors(search)`,
  `createProcessor`, `updateProcessor(name, input)`, `deleteProcessor(name)`.
- [x] 4.2 Create `web/src/lib/components/home3/doc-processors-view.svelte` modeled on
  `doc-process-dag-view.svelte`: debounced search box, list table with `type`/`status`/`require_llm`
  badges, create/edit modal (`name_as_id` editable only on create; type/status as select dropdowns;
  require_llm as a toggle), delete-with-confirm, and backend validation error messages surfaced
  verbatim.
- [x] 4.3 Add the nav child in `web/src/lib/components/home3/nav-rail.svelte`: extend the
  `sysadmin-doc-process-pipeline` subgroup's `children` with
  `{ id: 'sysadmin-doc-process-processors', label: 'Doc Processors' }`.
- [x] 4.4 Add the branch in `web/src/lib/components/home3/content-panel.svelte`:
  `activeMenu?.childId === 'sysadmin-doc-process-processors'` → render
  `<DocProcessorsView {darkMode} />`, and import it.

## 5. Verify & commit

- [x] 5.1 Frontend build: `bun run check` passes for the new files (only pre-existing error
  remains, in `doc-processor-dashboard-state.test.ts` — `.ts`-suffixed import under
  `allowImportingTsExtensions=off`, last touched by older commits, unrelated to this change).
- [x] 5.2 Smoke-test against the running dev server: `kb.doc_processors` has the 22 seeded rows;
  goose recorded `20260811000001` as applied; vite compiles the new view/client/content-panel
  (200s); `/api/v1/kb/doc-processors` is behind the session auth middleware (401 unauthenticated)
  and the CRUD logic is covered by the 14 go-sqlmock tests. Interactive browser click-through
  (create/edit/delete) still to be done in a logged-in session.
- [x] 5.3 Commit the migration, backend, and frontend changes via `jj` (workspace CLAUDE.md:
  commit via jj, not raw git; confirm linear history with `jj log`).
