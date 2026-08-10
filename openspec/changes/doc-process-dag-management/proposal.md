## Why

ADR 2026081001 (DR10) defines a **Doc Process DAG** as a first-class object: a doc process pipeline (`kb.pipelines` row), its DAG edges and processor gates (`kb.pipeline_rules` rows), and optionally a knowledge-store binding (`kb.pipeline_bindings` row). The backend now authors and validates these atomically, but there is **no UI** to search, view, create, modify, or delete them — admins must hand-edit SQL or use raw API calls. This change adds a frontend management page under *Development → System Admin → Doc Process Pipeline → Doc Process DAG* plus the small backend surface it needs.

## What Changes

- **New backend composite "Doc Process DAG" API** (`server/api/kbhandler/doc_process_dag_handler.go`), registered under `/api/v1/kb/doc-process-dags`:
  - `GET /doc-process-dags?search=` — list one entry per pipeline name (current `active` version), optionally filtered by name substring.
  - `GET /doc-process-dags/:name` — full detail: pipeline, its rules (gates + `depends_on_processors` DAG edges), and its bindings.
  - `POST /doc-process-dags` — create a DAG: validate (name required + unique, ≥1 processor, ADR DR8 `ValidatePipelineVersion`), author the pipeline version + rules in **one transaction**.
  - `PUT /doc-process-dags/:name` — modify a DAG: processor/rule changes author a **new version** (superseding the prior, in one transaction); cosmetic-only changes (`display_name`, `description`) update in place.
  - `DELETE /doc-process-dags/:name` — delete the DAG (all versions, rules, and referencing bindings) in **one transaction**; rejected when the DAG is the system default.
- **System-default invariant** (requirement 6): at most one DAG is `is_system_default` (already enforced by `idx_kb_pipelines_one_system_default`); the new handler additionally guarantees *at least* one — the first DAG created is auto-marked default, unmarking the sole default is rejected, and deleting the default is rejected.
- **≥1 processor validation** (requirement 3): `ValidatePipelineVersion` rejects an empty processor set.
- **New processor-catalog endpoint** `GET /api/v1/kb/doc-process-processors` returning the registered processor specs (name/phase/requires/produces/class/cost) so the page can offer a real picker.
- **Frontend page** `doc-process-dag-view.svelte` (searchable list, create/edit modal, delete with confirm) wired into the nav under *System Admin → Doc Process Pipeline → Doc Process DAG*, plus a typed `doc-process-dag-client.ts`.
- No migration is required: the schema already carries `version`/`status`/`is_system_default` on `kb.pipelines`, `depends_on_processors` on `kb.pipeline_rules`, and the one-default partial unique index.

## Capabilities

### New Capabilities
- `doc-process-dag-management`: search, view, create, modify, and delete Doc Process DAGs, with unique names, ≥1 processor, DR8 validation before save, transaction-protected writes, and exactly one system-default DAG.

### Modified Capabilities
<!-- None — this change introduces new behavior; it does not change spec-level behavior of
     an existing capability (no prior doc-process frontend capability exists). -->

## Impact

- **Backend:** `ChenWeb/server/api/kbhandler/doc_process_dag_handler.go` (new), `server/api/doc-processing/pipeline_version_validate.go` (empty-processor check), `server/api/routes.go` (route registration).
- **Frontend:** `ChenWeb/web/src/lib/components/home3/nav-rail.svelte` (nav subgroup + child), `content-panel.svelte` (view branch), new `doc-process-dag-view.svelte`, new `doc-process-dag-client.ts`.
- **Tests:** new handler tests; updated `pipeline_version_validate_test.go` if the empty-processor check changes existing expectations.
- **No schema migration, no shared-library change, no change to the existing `/kb/pipelines` CRUD handlers** (they stay for other consumers; the new DAG endpoint is a higher-level composite).
