# DR13 — Document Review Request GUI: Design Document

**Date:** 2026-06-21
**Status:** Draft
**Component:** ChenWeb — doc_review
**ADR Reference:** ADR 2026061801 (DR13)

---

## 1. Overview

Implement DR13 of ADR 2026061801: the Review Request GUI for the SemOS document
review service. Full-stack implementation covering database tables, service layer
(DocReviewController, DocReviewReportGenerator), API endpoints, and frontend
components integrated into the home3 layout.

---

## 2. Architecture

```
Frontend (SvelteKit 5 / home3)
  NavRail ── "Apps → Document Review"
  ContentPanel ── DocumentReviewView (5-step form)
               ── DocumentReviewResultsView (polling, findings table, report)
  │
  │ fetch() ── /api/v1/doc-review/*
  ▼
Backend (Go + Echo v4)
  docreviewhandler/ ── HTTP handlers (9 endpoints)
       │
       ▼
  docreview/ ── DocReviewController (service layer)
       │      ── DocReviewReportGenerator (report builder)
       │      ── aspects.go (aspect definitions, tier mappings)
       │      ── models.go (request/response types)
       │
       ▼
  doc-processing/review-document.go ── ReviewProcessor (existing)
       │
       ▼
  Database
    kb.doc_review_findings   (existing)
    kb.doc_review_requests   (new)
    kb.doc_review_reports    (new)
```

---

## 3. Database Migrations

### 3.1 `kb.doc_review_requests`

```sql
CREATE TABLE IF NOT EXISTS kb.doc_review_requests (
    id              BIGSERIAL       PRIMARY KEY,
    input_record_id BIGINT          NOT NULL,
    review_run_id   TEXT,
    tier            TEXT            NOT NULL,       -- "must_review", "should_review", "custom"
    aspects         JSONB           NOT NULL,       -- ["completeness", "grammar_spelling", ...]
    reference_docs  JSONB,
    notes           TEXT,
    model_overrides JSONB,
    requester_name  TEXT            NOT NULL,
    requester_id    BIGINT          NOT NULL,
    report_template TEXT,                           -- template for report doc generation
    doc_template    TEXT,                           -- template for modified doc generation
    status          TEXT            NOT NULL DEFAULT 'accepted',  -- accepted|running|completed|failed|stopped
    create_time     TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    start_time      TIMESTAMPTZ,
    end_time        TIMESTAMPTZ,
    error_message   TEXT
);

CREATE INDEX IF NOT EXISTS idx_doc_review_requests_record ON kb.doc_review_requests (input_record_id);
CREATE INDEX IF NOT EXISTS idx_doc_review_requests_status ON kb.doc_review_requests (status);
```

### 3.2 `kb.doc_review_reports`

```sql
CREATE TABLE IF NOT EXISTS kb.doc_review_reports (
    id                BIGSERIAL       PRIMARY KEY,
    request_id        BIGINT          NOT NULL,
    input_record_id   BIGINT          NOT NULL,
    review_run_id     TEXT            NOT NULL,
    report_json       JSONB           NOT NULL,
    report_markdown   TEXT            NOT NULL,
    executive_summary TEXT            NOT NULL,
    total_findings    INT             NOT NULL,
    high_count        INT             NOT NULL DEFAULT 0,
    medium_count      INT             NOT NULL DEFAULT 0,
    low_count         INT             NOT NULL DEFAULT 0,
    overall_assessment TEXT           NOT NULL,
    create_time       TIMESTAMPTZ     NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_doc_review_reports_request ON kb.doc_review_reports (request_id);
CREATE INDEX IF NOT EXISTS idx_doc_review_reports_record  ON kb.doc_review_reports (input_record_id);
```

---

## 4. DocReviewController — Service Layer

**Package:** `server/api/docreview/`

### 4.1 DocReviewController

A standalone service component (not a pipeline processor) managing the review request lifecycle.

**Methods:**
- `ValidateRequest(ctx, input)` — check document exists, aspects selected, requester resolved
- `ResolveReviewerSet(tier, aspects)` → translate tier/custom list to concrete reviewer configs
- `MergeModelOverrides(configs, overrides)` — apply per-request model overrides
- `MergeReferenceDocs(userRefs, autoRefs)` — combine user-supplied with auto-discovered refs
- `AcceptRequest(ctx, input)` — validate + store as "accepted"
- `RunReview(ctx, requestID)` — status → "running", delegate to ReviewProcessor.PostProcessIndex
- `CollectFindings(ctx, reviewRunID)` — read from kb.doc_review_findings
- `GenerateReport(ctx, requestID, findings)` — delegate to DocReviewReportGenerator
- `StopRequest(ctx, requestID)` — transition running → stopped
- `GetRequest(ctx, requestID)` — return request + findings
- `GetReport(ctx, reportID)` — return report JSON
- `GetReportHTML(ctx, reportID)` — render HTML template from report JSON

### 4.2 State Machine

```
accepted → running → completed
                   → failed (retryable)
                   → stopped (user cancelled)
```

### 4.3 Aspect/Tier Definitions

**Package:** `server/api/docreview/aspects.go`

Defined inline, matching the Document Review Checklist spec:
- `ListAspects()` — returns all ~40 aspects with group, priority, description, default model
- `ListTiers()` — returns tier→aspect mapping (must_review, should_review, etc.)
- `AspectPriority` — "Must Review", "Should Review", "Review for External/Public", "Review for Regulated"

---

## 5. DocReviewReportGenerator

**Package:** `server/api/docreview/report.go`

### 5.1 Inputs
- `[]ReviewFinding` for the review_run_id
- Document metadata (title, doc_no, file_name)
- Review request row (aspects ran, user notes)
- `kb.summaries` (for executive summary context)

### 5.2 Report Assembly

```
1. Load findings by review_run_id
2. Group by pass (P1–P6)
3. Group findings by finding_type within each pass
4. Compute severity counts (high/medium/low)
5. Build compliance_summary from reference_doc-tagged findings
6. Delegate executive summary to cheap LLM (one-shot Haiku)
7. Build recommendations from high-severity findings
8. Fill JSON skeleton → serialize
9. Render Go HTML template from JSON
10. Render Go Markdown template from JSON
11. Persist JSON + Markdown to kb.doc_review_reports
```

### 5.3 Report Representations

| Format | Storage | Served via |
|--------|---------|------------|
| JSON | `report_json` (DB) | `GET /reports/<id>` |
| HTML | On-the-fly template render | `GET /reports/<id>/html` |
| Markdown | `report_markdown` (DB) | `GET /reports/<id>/export?format=md` |

The HTML template is a single Go `html/template` driven by the report JSON struct
with inline CSS for standalone viewing. The Svelte results page can embed it as
structured content or link to the HTML endpoint.

---

## 6. API Endpoints

**Package:** `server/api/docreviewhandler/`

Route registration in `server/api/routes.go` under `/api/v1/doc-review/` group.

| Method | Path | Handler | Purpose |
|--------|------|---------|---------|
| `GET` | `/api/v1/doc-review/aspects` | `ListAspects` | All aspects + groups + priorities |
| `GET` | `/api/v1/doc-review/tiers` | `ListTiers` | Tier→aspect mappings |
| `POST` | `/api/v1/doc-review/requests` | `SubmitRequest` | Create + trigger review |
| `GET` | `/api/v1/doc-review/requests/:id` | `GetRequest` | Status + findings |
| `GET` | `/api/v1/doc-review/reports/:id` | `GetReport` | Full report JSON |
| `GET` | `/api/v1/doc-review/reports/:id/html` | `GetReportHTML` | HTML report view |
| `GET` | `/api/v1/doc-review/reports/:id/export` | `ExportReport` | Markdown/PDF/DOCX |
| `PATCH` | `/api/v1/doc-review/findings/:id` | `UpdateFinding` | Accept/reject/defer |
| `POST` | `/api/v1/doc-review/requests/:id/stop` | `StopRequest` | Cancel running review |

### 6.1 Request Validation — Requester Resolution

On `POST /requests`:
1. Parse body (requester_name, requester_id, tier, aspects, ...)
2. Look up requester_id in Kratos/users table
3. If not found → return 422 with error message guiding to register
4. If found → continue validation

---

## 7. Frontend — Home3 Integration

### 7.1 Nav Rail

Add child under existing "Applications" parent in `nav-rail.svelte`:
```typescript
{ id: 'apps-document-review', label: 'Document Review' }
```

Update `content-panel.svelte` to map this child ID to the view component.

### 7.2 View Components

**`document-review-view.svelte`** — The 5-step form (main view):
1. **Select Document** — searchable list of processed documents (`kb.inputs`)
2. **Choose Check Level** — Must Review / Should Review / Custom radio buttons
3. **Customize Aspects** — checkboxes grouped by P1–P6 (shown when Custom selected)
4. **Supporting Documents** — optional search-and-add reference standards
5. **Notes/Requester** — requester name lookup + optional notes
6. **Submit** — sends POST /requests, redirects to results

**`doc-review-results-view.svelte`** — Results page (sub-view after submission):
1. **Progress indicator** — polling spinner while `status: "running"`
2. **Summary cards** — total findings, severity bar chart, pass-by-pass breakdown
3. **Findings table** — filterable, sortable, expandable rows with accept/reject/defer
4. **Report view** — full report rendering (from JSON or embedded HTML)
5. **Export buttons** — Markdown/PDF

### 7.3 Service Layer

**`web/src/lib/services/docReviewService.ts`** — Following existing service pattern:
```typescript
const BASE = '/api/v1/doc-review';

export async function listAspects(): Promise<Aspect[]> { ... }
export async function listTiers(): Promise<Tier[]> { ... }
export async function submitRequest(input: SubmitInput): Promise<SubmitResult> { ... }
export async function getRequest(id: number): Promise<RequestWithFindings> { ... }
export async function getReport(id: number): Promise<Report> { ... }
export async function updateFinding(id: number, status: string): Promise<void> { ... }
export async function stopRequest(id: number): Promise<void> { ... }
```

### 7.4 Results Page Navigation

After submission, the view switches to the results view either by:
- Changing `activeMenu` selection to `apps-document-review-results` with a parameter
- Or using a component-internal state toggle (`$state('submitted') → $state('results')`)

Using internal state keeps it simpler — the nav rail item stays "Document Review" and the view component manages its own form/results screens internally.

---

## 8. Files to Create/Modify

### New Files
| Path | Purpose |
|------|---------|
| `server/api/docreview/controller.go` | DocReviewController service |
| `server/api/docreview/models.go` | Request/response types |
| `server/api/docreview/aspects.go` | Aspect definitions, tier mappings |
| `server/api/docreview/report.go` | DocReviewReportGenerator |
| `server/api/docreview/report_template.html` | HTML report template |
| `server/api/docreview/report_markdown.tmpl` | Markdown report template |
| `server/api/docreviewhandler/handler.go` | HTTP handlers |
| `project_migrations/20260621000001_create_doc_review_requests.sql` | Migration 1 |
| `project_migrations/20260621000002_create_doc_review_reports.sql` | Migration 2 |
| `web/src/lib/components/home3/document-review-view.svelte` | 5-step form view |
| `web/src/lib/components/home3/doc-review-results-view.svelte` | Results view |
| `web/src/lib/services/docReviewService.ts` | API service functions |

### Modified Files
| Path | Change |
|------|--------|
| `server/api/routes.go` | Register doc-review routes |
| `web/src/lib/components/home3/nav-rail.svelte` | Add "Document Review" nav item |
| `web/src/lib/components/home3/content-panel.svelte` | Wire view component |

---

## 9. Tests

- **Controller — requester resolution:** valid requester → request accepted; unknown requester → 422 error
- **Controller — state machine:** lifecycle transitions (accepted→running, running→completed, running→failed, running→stopped)
- **Controller — override merging:** model override for P5 replaces P5 default, leaves P1–P4 untouched
- **ReportGenerator — JSON skeleton:** report structure conforms to schema, counts match underlying findings
- **ReportGenerator — HTML template:** renders without error given valid JSON
- **API — submit + retrieve:** POST request → GET returns status
- **API — findings PATCH:** update review_status, GET reflects change
