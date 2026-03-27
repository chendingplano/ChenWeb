# Document Generation Feature — Design Spec
Date: 2026-03-27

## Overview

Add a Document Generation feature to ChenWeb that merges Word (`.docx`) templates with structured
data retrieved from the database, producing finished documents (`.docx` or `.pdf`). Jobs are
processed asynchronously by a background worker pool. The feature is surfaced under
**Applications → Generate Doc** in the home3 nav rail.

---

## Architecture

```
Backend
├── api/docgenhandler/            HTTP handlers
├── api/docgenworker/             background worker pool
├── api/appdatastores/
│   ├── table-doc-gen-jobs.go     doc_gen_jobs table
│   ├── table-doc-gen-log.go      doc_gen_log table (per-document results)
│   └── table-doc-gen-queries.go  doc_gen_queries table (predefined SQL)
└── server/migrations/            goose migration (single file, all three tables)

Frontend
└── lib/components/home3/doc-gen-view.svelte   3-tab UI
```

### Async flow

1. `POST /api/v1/docgen/jobs` inserts a `doc_gen_jobs` row (`status=pending`), pushes the job ID
   onto an in-memory buffered channel, and returns `{job_id}` immediately.
2. A worker goroutine picks the job ID from the channel, executes the pipeline, inserts one
   `doc_gen_log` row per generated document, and updates the job's `status`, `success_count`,
   and `fail_count`.
3. On server restart, `docgenworker.RequeueStalledJobs()` scans for `pending`/`processing` jobs
   and re-pushes them to the channel.

---

## Database Tables

All three tables are created via a single goose migration file. They are **not** registered in
`database.CreateTables()`.

### `doc_gen_jobs`

| Column | Type | Notes |
|---|---|---|
| `job_id` | BIGSERIAL PK | |
| `request_name` | VARCHAR(255) NOT NULL UNIQUE | |
| `purpose` | VARCHAR(255) NOT NULL | |
| `remarks` | TEXT | optional |
| `sql_query_id` | BIGINT | FK → `doc_gen_queries`, null if ad-hoc |
| `sql_statement` | TEXT NOT NULL | copied at submission time |
| `template_type` | VARCHAR(16) NOT NULL | `word` or `typst` |
| `template_path` | TEXT NOT NULL | server path or uploaded filename |
| `converter` | JSONB NOT NULL | maps SQL columns → template token names |
| `output_dir` | TEXT NOT NULL | |
| `output_format` | VARCHAR(16) NOT NULL | `docx` or `pdf` |
| `status` | VARCHAR(32) NOT NULL | `pending` / `processing` / `completed` / `failed` |
| `total_count` | INT DEFAULT 0 | |
| `success_count` | INT DEFAULT 0 | |
| `fail_count` | INT DEFAULT 0 | |
| `error_msg` | TEXT | top-level error (e.g. bad SQL) |
| `created_by` | VARCHAR(255) NOT NULL | |
| `created_at` | TIMESTAMPTZ NOT NULL DEFAULT NOW() | |
| `updated_at` | TIMESTAMPTZ NOT NULL DEFAULT NOW() | |

### `doc_gen_log`

Per-document audit log, one row per generated file. Schema matches the spec with `job_id` FK added.

| Column | Type | Notes |
|---|---|---|
| `id` | BIGSERIAL PK | |
| `job_id` | BIGINT NOT NULL | FK → `doc_gen_jobs` |
| `request_name` | VARCHAR(255) NOT NULL | |
| `customer_id` | VARCHAR(128) NOT NULL | |
| `customer_name` | VARCHAR(255) NOT NULL | |
| `email` | VARCHAR(255) NOT NULL | may be empty string |
| `phone_num` | VARCHAR(64) | optional |
| `purpose` | VARCHAR(255) NOT NULL | |
| `filename` | VARCHAR(512) NOT NULL | |
| `status` | VARCHAR(32) NOT NULL | `generated` or `failed` |
| `error_msg` | TEXT | optional |
| `remarks` | TEXT | optional |
| `created_by` | VARCHAR(255) NOT NULL | |
| `created_at` | TIMESTAMPTZ NOT NULL DEFAULT NOW() | |
| `updated_at` | TIMESTAMPTZ NOT NULL DEFAULT NOW() | |

### `doc_gen_queries`

Admin-managed predefined SQL statements.

| Column | Type | Notes |
|---|---|---|
| `id` | BIGSERIAL PK | |
| `name` | VARCHAR(255) NOT NULL UNIQUE | used for search-and-pick in the UI |
| `description` | TEXT | |
| `sql_statement` | TEXT NOT NULL | |
| `created_by` | VARCHAR(255) NOT NULL | |
| `created_at` | TIMESTAMPTZ NOT NULL DEFAULT NOW() | |
| `updated_at` | TIMESTAMPTZ NOT NULL DEFAULT NOW() | |

---

## API Endpoints

All under `/api/v1/docgen`, protected by `AuthMiddleware`.

### Jobs

| Method | Path | Description |
|---|---|---|
| `POST` | `/docgen/jobs` | Submit job; returns `{job_id}` immediately |
| `GET` | `/docgen/jobs` | List jobs (filters: status, date range, request_name; paginated) |
| `GET` | `/docgen/jobs/:id` | Get job status + its `doc_gen_log` entries |

### Predefined SQL Queries

| Method | Path | Description |
|---|---|---|
| `GET` | `/docgen/queries` | Search queries by name (all authenticated users) |
| `POST` | `/docgen/queries` | Create query — admin only, returns 403 otherwise |
| `PUT` | `/docgen/queries/:id` | Update query — admin only |
| `DELETE` | `/docgen/queries/:id` | Delete query — admin only |

### Templates

| Method | Path | Description |
|---|---|---|
| `GET` | `/docgen/templates` | List templates on server filesystem |
| `POST` | `/docgen/templates` | Upload a template file (multipart/form-data) |

Admin enforcement: check `user_info.Role == "admin"` from JWT claims; return 403 if not admin.

---

## Worker Design

**Package:** `api/docgenworker/`

- Fixed-size goroutine pool (default 3 workers), buffered channel (capacity 100).
- `Start(db, jobCh, workerCount)` — called once at server boot after DB is ready.
- `RequeueStalledJobs(db, jobCh)` — called at boot to recover `pending`/`processing` jobs.

### Pipeline (per job)

1. Fetch full job row from `doc_gen_jobs`.
2. Set `status = processing`, `updated_at = NOW()`.
3. Execute `sql_statement` against the project DB (read-only validated: must begin with `SELECT`).
4. Load template file from `template_path`.
5. For each result row:
   a. Apply Converter map to produce template token values.
   b. Render document using `github.com/nguyenthenguyen/docx` (token replacement: `{{key}}` → value).
   c. Write file to `<output_dir>/<request_name>/<filename>`.
   d. Insert one `doc_gen_log` row with `status = generated` or `failed`.
6. Update `doc_gen_jobs`: set `status = completed` (or `failed` for top-level errors),
   `total_count`, `success_count`, `fail_count`, `updated_at`.

### Word rendering

- Library: `github.com/nguyenthenguyen/docx`
- Tokens in template: `{{key}}` where `key` is a value from the Converter map.
- Converter JSON example: `{"customer_name": "companyName", "invoice_date": "invoiceDate"}`
  (left = SQL column, right = template token name).
- The Converter **must** include mappings for the reserved `doc_gen_log` fields:
  `customer_id`, `customer_name`, `email` (and optionally `phone_num`). If any required mapping
  is absent, the job fails at validation time (before the worker starts) with a 400 error.

### Typst

Deferred. The `template_type` field accepts `typst` but the worker returns a structured error
`"typst templates not yet supported"` for those jobs. The API shape is already correct.

---

## Frontend UI

### Nav change

Add child `{ id: 'apps-generate-doc', label: 'Generate Doc' }` to the `applications` item in
`nav-rail.svelte`. No other nav items change.

### Component: `doc-gen-view.svelte`

Three tabs. Tab 3 is hidden for non-admin users.

#### Tab 1 — Generate

Form fields:
- **Request Name** — text input (required, must be unique)
- **Purpose** — text input (required)
- **SQL Query** — search-and-pick: type to filter predefined queries by name; selected query shows
  a read-only SQL preview below. Calls `GET /docgen/queries?q=<term>`.
- **Template Type** — dropdown: `word` / `typst`
- **Template** — dropdown of server-side templates (from `GET /docgen/templates`) plus an
  "Upload new…" option that reveals a file picker; upload calls `POST /docgen/templates`.
- **Converter** — JSON textarea with basic parse-on-blur validation.
- **Output Directory** — text input (required)
- **Output Format** — dropdown: `docx` / `pdf`
- **Remarks** — optional textarea

On submit: `POST /api/v1/docgen/jobs` → success banner shows job ID with "View in History" link.

#### Tab 2 — History

- Filter bar: status dropdown, date-range picker, request-name search input.
- Paginated table columns: Request Name, Purpose, Status (coloured badge), Success / Fail counts,
  Created By, Created At.
- Row click expands inline to show the `doc_gen_log` entries for that job: Filename, Customer,
  Status badge, Error message.
- Auto-refreshes every 5 seconds while any job has `pending` or `processing` status.

#### Tab 3 — SQL Queries (admin only)

- Search bar to filter by name.
- Table: Name, Description, Created By, Created At — with Edit (inline) and Delete (confirm dialog)
  actions.
- "+ Add Query" button reveals an inline form: Name (required), Description, SQL Statement textarea
  (required).

---

## Error Handling

- SQL statement is validated to start with `SELECT` before execution; non-SELECT returns 400.
- Unique `request_name` enforced at DB level (UNIQUE constraint) and checked before inserting;
  returns 409 Conflict with clear message.
- Template file not found → job fails with `error_msg` set; no partial `doc_gen_log` rows.
- Per-document render failures are logged individually in `doc_gen_log` without aborting the rest
  of the batch.
- Worker panics are recovered and the job is marked `failed`.
