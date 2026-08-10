## 1. Backend: validator hardening

- [ ] 1.1 Add a non-empty-processor check at the top of `ValidatePipelineVersion` in `server/api/doc-processing/pipeline_version_validate.go` (empty `draft.Processors` → error naming the requirement). Add a test in `pipeline_version_validate_test.go` asserting an empty processor set is rejected and existing scenarios still pass.

## 2. Backend: Doc Process DAG composite handler

- [ ] 2.1 Create `server/api/kbhandler/doc_process_dag_handler.go` with:
  - `pipelineDAGRecord` (list shape: id, name, display_name, description, version, processors, is_system_default, rule count, create_time, modify_time) and `pipelineDAGDetail` (adds full `kb.pipeline_rules` gates incl. `depends_on_processors`, and `kb.pipeline_bindings` read-only list).
  - `ListDocProcessDAGs` — one row per pipeline name (current `active` version, `version = MAX`), optional `?search=` ILIKE on name/display_name.
  - `GetDocProcessDAG` — by name, current version, with rules and bindings.
- [ ] 2.2 `CreateDocProcessDAG` — parse payload (name, display_name, description, processors, is_system_default, rules as `pipelineRuleDraftPayload`-shaped gates); reject duplicate name, empty processors, invalid predicates; run `ValidatePipelineVersion`; in one tx: lock prior version for name (`FOR UPDATE`), compute next version, insert pipeline row, clear prior default row if this one is default, set this row default when no default exists, supersede prior version, insert rule rows, commit. Return the created detail.
- [ ] 2.3 `UpdateDocProcessDAG` — by name; compare submitted processors+rules against current version: if changed, reuse the create-authoring path (new version, supersede, default transfer); else in-place `UPDATE` for `display_name`/`description`/`is_system_default` only. Reject unsetting the sole default. Reject unknown names.
- [ ] 2.4 `DeleteDocProcessDAG` — by name; reject if any row for the name is the system default (message: promote another first); in one tx delete `kb.pipeline_bindings` (WHERE pipeline_id IN versions), then `kb.pipeline_rules` (same), then `kb.pipelines` rows.
- [ ] 2.5 `ListDocProcessProcessors` — return `docprocessing.RegisteredProcessors()` (name, phase, class, cost, on_undetermined, idempotent, requires, produces).

## 3. Backend: routes + tests

- [ ] 3.1 Register routes in `server/api/routes.go`: `GET/POST /kb/doc-process-dags`, `GET/PUT/DELETE /kb/doc-process-dags/:name`, `GET /kb/doc-process-processors`.
- [ ] 3.2 Write `server/api/kbhandler/doc_process_dag_handler_test.go` covering: duplicate-name rejection, empty-processor rejection, cycle/dangling-edge rejection (no partial rows), create-default auto-marking, default transfer, last-default-unset rejection, delete-default rejection, delete removes bindings+rules+pipelines atomically, processor-change authors new version + supersedes, cosmetic change does not new-version. Use the existing test patterns from `pipelines_handler_test.go` / `pipeline_bindings_handler_test.go`.
- [ ] 3.3 Verify: `go build ./...`, `go vet ./server/api/kbhandler/... ./server/api/doc-processing/...`, `go test ./server/api/kbhandler/... ./server/api/doc-processing/...`.

## 4. Frontend: client + view + nav

- [ ] 4.1 Create `web/src/lib/components/home3/doc-process-dag-client.ts` (typed `req<T>` wrapper like `schedules-client.ts`) with `listDAGs`, `getDAG`, `createDAG`, `updateDAG`, `deleteDAG`, `listProcessors`.
- [ ] 4.2 Create `web/src/lib/components/home3/doc-process-dag-view.svelte` following `schedules-view.svelte` patterns: search box, DAG list (name, version, default badge, processor count), create/edit modal (name, display_name, description, processor multi-select with per-processor `depends_on_processors` checkboxes, `is_system_default` toggle, notice when no default exists), delete-with-confirm. Surface backend validation error messages verbatim.
- [ ] 4.3 Add nav entry in `web/src/lib/components/home3/nav-rail.svelte` under the `system-admin` group: subgroup `{ id: 'sysadmin-doc-process-pipeline', label: 'Doc Process Pipeline', children: [{ id: 'sysadmin-doc-process-dag', label: 'Doc Process DAG' }] }`.
- [ ] 4.4 Add the branch in `web/src/lib/components/home3/content-panel.svelte`: `activeMenu?.childId === 'sysadmin-doc-process-dag'` → render `<DocProcessDagView {darkMode} />`.

## 5. Verify & commit

- [ ] 5.1 Frontend build: `cd web && bun run check` (or the project's build script) passes.
- [ ] 5.2 Smoke-test against the running dev server: create a DAG, search it, edit processors (new version appears), flip default, attempt to delete default (rejected), delete a non-default DAG.
- [ ] 5.3 Commit the backend and frontend changes via `jj` (workspace CLAUDE.md: commit via jj, not raw git; confirm linear history with `jj log`).
