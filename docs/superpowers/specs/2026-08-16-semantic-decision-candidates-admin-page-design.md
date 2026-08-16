# Semantic Decision Candidates Admin Page

## Context

`kb.semantic_decision_candidates` stores generated semantic-association proposals and
their operational lifecycle. The table already has a domain store in
`server/api/ontology/assertions/decision_candidates_store.go`, including payload
fingerprinting, revision reuse, status transitions, deferral/retry, and resolution
updates. This change exposes those operations to administrators at
**System Admin → Doc Process Pipeline → Semantic Decision Candidates**.

## Goals

- Provide a searchable, paginated admin browser for decision candidates.
- Allow creation through the existing `Propose` lifecycle, preserving fingerprint and
  revision semantics.
- Allow inspection and safe mutation through existing lifecycle operations only.
- Surface payload, source spans, and decision metadata without hiding JSON details.
- Follow existing ChenWeb admin navigation, Echo handler, and Svelte 5 page conventions.

## Non-goals

- No SQL delete or UI Delete action for now. Terminal rejection/superseding preserves
  the audit trail.
- No direct editing of generated identity, revision, fingerprint, timestamps, or
  supersession fields.
- No change to semantic-association processing or candidate-generation behavior.
- No new database migration; the table already exists.

## Design

### Backend API

Add a dedicated handler in `server/api/kbhandler` and register routes beside the other
knowledge pipeline APIs:

- `GET /kb/semantic-decision-candidates` — paginated list with filters for status,
  candidate kind, method, logical identity, source artifact, and input record.
- `POST /kb/semantic-decision-candidates` — create via `DecisionCandidateStore.Propose`.
- `GET /kb/semantic-decision-candidates/:id` — retrieve one full record.
- `POST /kb/semantic-decision-candidates/:id/transition` — use
  `TransitionStatus`, returning validation errors for illegal arcs.
- `POST /kb/semantic-decision-candidates/:id/resolution` — use `SetResolution`.
- `POST /kb/semantic-decision-candidates/:id/defer` — use `DeferCandidate`.
- `POST /kb/semantic-decision-candidates/:id/retry` — use `RetryDeferred`.
- `POST /kb/semantic-decision-candidates/:id/assertion` — use
  `SetResultingAssertion`.

The list response includes `results`, `page`, `page_size`, and `total`. Filtering and
ordering are implemented with allow-listed SQL fields and parameterized values. The
handler validates IDs, JSON payloads, enum values, confidence bounds, and required
fields before invoking the domain store. Errors use the existing `{status, error_msg}`
shape and structured logger codes.

The existing store needs a focused paginated/filterable list method; its lifecycle
methods remain the source of truth for mutations. No hard-delete store method or route
will be added.

### Frontend

Add `semantic-decision-candidates-view.svelte` under `web/src/lib/components/home3/`.
The page will match the existing dark/light admin surfaces and contain:

- header with table description and refresh action;
- filter panel for lifecycle/status and source fields;
- paginated table with identity/revision, kind, method, source, confidence, status,
  resolution, and timestamps;
- detail/edit dialog showing all fields, formatted JSON, and only legal lifecycle
  actions for the selected row;
- create dialog using JSON textareas for `proposed_payload` and `source_line_spans`;
- clear success/error feedback and loading states.

Generated/immutable fields are displayed read-only. The UI offers transition,
resolution, defer, retry, and assertion-link operations; it has no Delete control.

Wire the view into `nav-rail.svelte` as a child of the existing
`sysadmin-doc-process-pipeline` group and into `content-panel.svelte`. The page is an
app-shell page with internal scrolling, like the existing admin log pages.

### Testing

- Go store tests for list filters/pagination and handler tests for request validation,
  response shape, and lifecycle endpoint delegation.
- Frontend type/check validation, plus focused client tests if a standalone client
  module is introduced.
- Manual smoke checks against the dev server: list, create/reuse, inspect, transition,
  defer/retry, resolution update, and assertion link; confirm no Delete action exists.

## Alternatives considered

1. Reuse the ontology-candidate API shape. Rejected because the two tables have
   different lifecycle rules and domain packages; a dedicated API keeps semantics clear.
2. Build a read-only browser with a few actions. Rejected because it would not provide
   the requested create and lifecycle-safe update operations.
3. Expose raw SQL-like CRUD. Rejected because it would bypass revision, fingerprint,
   and transition invariants.

## Open decisions

None. Hard deletion is explicitly out of scope for this version.
