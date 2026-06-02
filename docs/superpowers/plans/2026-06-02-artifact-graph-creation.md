# Artifact Graph Creation — Implementation Plan

> **For agentic workers:** Steps use checkbox (`- [ ]`) syntax. Follow TDD: failing test → watch fail → minimal code → watch pass → commit. DRY, YAGNI.

**Goal:** Materialize the cross-artifact connection graph specified in
`KnowledgeStore/Capsules/coding-capsules/deep-wiki/artifact-graph-creation.md` into a single
partitioned `kb.artifact_connections` table, written idempotently by the producing doc
processors.

**Architecture:** One canonical edge table partitioned by `relation_method`
(`llm | line_overlap | structural | manual`). A small package (in `docprocessing`) provides
(a) a pure line-overlap derivation function and (b) a SQL store that replaces a document's
edges idempotently. Each producing processor calls the store after it persists its artifact,
mirroring the existing `Reindex*SearchForRecord` hook pattern.

**Tech Stack:** Go, PostgreSQL (schema `kb`), goose migrations, existing
`docprocessing` package and `ApiTypes.ProjectDBHandle`.

---

## Design decisions (locked)

- **Storage:** single `kb.artifact_connections`, `PARTITION BY LIST (relation_method)`.
  Partitions: `llm`, `line_overlap`, `structural`, `manual`.
- **Chunk endpoint identity:** positional — `source_id = "<record_id>_<chunk_index>"`
  (`chunk_index` = `Block.Index`). Chunks are not first-class DB rows; edges are recomputed
  on reprocess, so positional ids are acceptable.
- **Idempotency:** per (re)process, scope-delete the document's edges for the relations the
  processor owns, then insert. FK `input_record_id → kb.inputs(id) ON DELETE CASCADE`.
- **Edge ownership:** the processor that produces the target artifact writes the
  `chunk → artifact` (`line_overlap`) edges; `extract-entity-relation` writes `entity→entity`
  (`llm`); metrics writes `metric→category` (`llm`). Higher-level summaries → `structural`,
  never `line_overlap`.
- **Searchability:** none on the edge table by default; node search is unchanged. (Optional
  `kb.search_artifacts_connection` partition deferred — not in this plan.)

## File structure

- Create: `project_migrations/20260602000002_create_kb_artifact_connections.sql` — table,
  4 partitions, indexes, unique constraint. (Down drops them.)
- Create: `server/api/doc-processing/connections.go` — `Connection` type, `LineSpan`
  helpers, `DeriveLineOverlapConnections(...)` pure function, id helpers.
- Create: `server/api/doc-processing/connections_test.go` — unit tests for derivation.
- Create: `server/api/doc-processing/connections_store.go` — `ConnectionStore` interface +
  `ConnectionSQLStore` with `ReplaceConnections(ctx, recordID, relationNames, []Connection)`.
- Modify: `server/api/doc-processing/extract-metrics.go` — after `SaveMetrics`, derive+write
  `has-metrics` edges (and `metric→category` follow-up).
- Modify (one task each, identical pattern): `generate-topics-processor.go`,
  `extract-provisions.go`, `extract-products.go`, `generate-scene-blocks-processor.go`,
  `generate-summaries-processor.go`, `extract-entity-relation.go`.

## Chunk 1: Foundation (DB-independent core + migration)

### Task 1: Migration for `kb.artifact_connections`

**Files:** Create `project_migrations/20260602000002_create_kb_artifact_connections.sql`

- [ ] **Step 1:** Write goose Up: `kb.artifact_connections` partitioned by LIST
  (`relation_method`); columns per spec (`id BIGSERIAL`, `input_record_id BIGINT NOT NULL
  REFERENCES kb.inputs(id) ON DELETE CASCADE`, `source_type/source_id/target_type/target_id/
  relation_name/relation_method TEXT NOT NULL`, `confidence DOUBLE PRECISION`,
  `overlap JSONB`, `provenance JSONB`, `semantic_signature TEXT`, `extra_info JSONB`,
  `create_time TIMESTAMPTZ NOT NULL DEFAULT NOW()`); `PRIMARY KEY (relation_method, id)`;
  `UNIQUE (relation_method, source_type, source_id, target_type, target_id, relation_name)`;
  4 partitions; per-partition indexes on `(input_record_id)`,
  `(input_record_id, source_type, source_id)`, `(input_record_id, target_type, target_id)`,
  `(input_record_id, relation_name)`. Goose Down drops partitions + table.
- [ ] **Step 2:** Verify it parses and applies on the staging DB (goose up), then `goose down`
  / `goose up` round-trips cleanly. Expected: no errors.
- [ ] **Step 3:** Commit.

### Task 2: `Connection` model + line-overlap derivation (TDD, no DB)

**Files:** Create `connections.go`, Test `connections_test.go`

- [ ] **Step 1 (RED):** Write `TestDeriveLineOverlapConnections_singleChunkSingleMetric` — a
  chunk `{Index:1, Lines: lines 1..10}` and one artifact with spans `["3","5:6"]` yields one
  `Connection{SourceType:"chunk", SourceID:"<rec>_1", TargetType:"metric",
  RelationName:"has-metrics", RelationMethod:"line_overlap"}` with `OverlapCount==3`.
- [ ] **Step 2:** Run, watch it fail to compile/assert.
- [ ] **Step 3 (GREEN):** Implement `Connection`, `ArtifactRef{Type,ID,Spans}`, and
  `DeriveLineOverlapConnections(recordID int64, relationName string, chunks []Block,
  targets []ArtifactRef) []Connection`. Chunk line set = `{l.LineNumber}`; target line set
  = expanded spans (reuse `parseMetricLineSpan`); emit edge when intersection non-empty with
  `OverlapCount = |intersection|` and sorted `OverlapLines`.
- [ ] **Step 4:** Run, watch pass.
- [ ] **Step 5 (RED):** Add `TestDeriveLineOverlapConnections_noOverlapNoEdge` and
  `_spanAcrossTwoChunks` (artifact lines 9..12 over chunk1=1..10, chunk2=11..20 ⇒ two edges).
- [ ] **Step 6:** Implement to green (loop over chunks). Verify pass.
- [ ] **Step 7:** Commit.

### Task 3: `ConnectionStore.ReplaceConnections` (idempotent write)

**Files:** Create `connections_store.go` (+ tests if a DB harness exists; otherwise integration-verified)

- [ ] **Step 1:** Define `ConnectionStore` interface and `ConnectionSQLStore{DB *sql.DB}`.
- [ ] **Step 2:** Implement `ReplaceConnections(ctx, recordID, relationNames []string, conns
  []Connection)`: in a tx, `DELETE FROM kb.artifact_connections WHERE input_record_id=$1 AND
  relation_method=$2 AND relation_name = ANY($3)` for the owned relations, then batch insert
  with `ON CONFLICT (...) DO UPDATE`. Marshal `overlap`/`provenance` to JSONB.
- [ ] **Step 3:** `go build ./server/...`. Commit.

## Chunk 2: Processor wiring (one vertical slice, then replicate)

### Task 4: Wire `extract-metrics.go` (proven slice)

- [ ] **Step 1:** After `SaveMetrics` succeeds, build `[]ArtifactRef` from saved metrics
  (`TargetID = metric_id`, `Spans = source_line_spans`), call
  `DeriveLineOverlapConnections(recordID, "has-metrics", inputChunks, refs)`, then
  `ReplaceConnections(ctx, recordID, []string{"has-metrics"}, conns)`. Log + degrade on error
  (mirror `ReindexMetricSearchForRecord` non-fatal handling).
- [ ] **Step 2:** `metric→category` (`llm`) follow-up from `category_paths`.
- [ ] **Step 3:** `go build ./server/...`; run existing metrics tests. Commit.

### Tasks 5–10: Replicate the Task 4 pattern

One task each — `has-topic` (topics), `has-provision` (provisions),
`has-part-component` (products), `has-scene` (scene-blocks), Level-0 `chunk→summary` +
Level≥1 `structural` (summaries), `entity→entity` mirror (entity-relation). Each: derive →
`ReplaceConnections` → build → existing tests → commit.

## Verification (per verification-before-completion)

- [ ] `go build ./server/...` clean.
- [ ] `go test ./server/api/doc-processing/...` green (new derivation tests + unchanged tests).
- [ ] Migration up/down round-trips on staging DB.
- [ ] Spot-check: reprocess a document twice → edge counts stable (idempotent), no duplicates.

## Notes

- Schema changes go through goose (`db-migration` skill).
- Do not add a tsvector to the edge table; node search is unchanged.
- `parseMetricLineSpan` already exists in-package — reuse it; do not duplicate span parsing.

---

## Implementation status (2026-06-02)

**Done & verified (`go build ./server/...` clean, unit tests green):**

- Migration `20260602000002_create_kb_artifact_connections.sql` (partitioned table + 4
  partitions + cascading indexes + unique constraint).
- Core: `connections.go` (`Connection`, `OverlapInfo`, `ArtifactRef`,
  `DeriveLineOverlapConnections`, `ChunkConnectionID`, relation-name constants) with
  `connections_test.go` (derivation + JSONB encode + registry-row parsing — all TDD).
- Store: `connections_store.go` — `ConnectionSQLStore.ReplaceConnections` (idempotent
  scope-delete + upsert), plus helpers `WriteLineOverlapConnectionsFromRegistry`
  (canonical ids sourced from `kb.search_artifacts`), `WriteLineOverlapConnectionsForRefs`,
  and `WriteConnections`.
- **All 5 File3 `line_overlap` relations wired**, each right after the producing
  processor's `Reindex*SearchForRecord` hook, registry-sourced for id-correctness:
  - `has-metrics` — `extract-metrics.go`
  - `has-topic` — `fix-size-chunking.go` (handleGenerateTopicsLines) **and**
    `semantic-chunking.go`
  - `has-provision` — `extract-provisions.go`
  - `has-part-component` — `extract-products.go`
  - `has-scene` — `generate-scene-blocks-processor.go`
- **Bonus (spec, not File3):** Level-0 `has-summary` — `fix-size-chunking.go`
  (handleGenerateSummariesLines), explicitly excluding Level≥1 (wide-span) summaries.

**Deferred — require identity resolution not available in stored data (do NOT ship blind):**

- **`entity relations` (entity→entity, llm)** — `kb.relations` stores `subject`/`object`
  as free text with no entity-id FKs; relations are their own `relation` artifact type.
  Producing joinable `entity`→`entity` edges needs a subject/object-text → entity
  `artifact_id` resolver (ambiguous; LLM- or match-based). Wire in
  `extract-entity-relation.go` via `WriteConnections(..., RelationMethodLLM, RelationEntityRelations, ...)`
  once that resolver exists.
- **`belong-to-category` (metric→category, llm)** — metrics carry `category_paths` (names),
  but categories are not registered artifacts and metrics carry no `category_id`. Needs a
  category-path → category-node-id lookup against the category tree. Wire in
  `extract-metrics.go` / `category_tree_indexing.go`.
- **Level≥1 `structural` summary rollup** — `SummaryItem.Children` holds child summary IDs
  (`buildSummaryID` form); needs Children-ID → `SeqNo` → `BuildArtifactID(...,"summary",seq)`
  mapping to emit `structural` parent→child edges. Wire in
  `handleGenerateSummariesLines` via `WriteConnections(..., RelationMethodStructural, ...)`.

**Integration verification still required (needs running server + Postgres):** apply the
migration on startup, process a document, confirm edge rows land in the right partitions
with canonical `target_id`s, and reprocess to confirm idempotency (stable counts, no dupes).

**Pre-existing unrelated test failures** (fail identically without these changes; `sqlmock`
expectation drift): `TestDocProcLoggerLogSummary_InsertsMSUsed`,
`TestGenerateTopicsProcessor_WritesSummaryLog`, `TestGenerateSummariesProcessor_WritesSummaryLog`,
`TestChunkingProcessor_WritesMethodSpecificSummaryLog`.
