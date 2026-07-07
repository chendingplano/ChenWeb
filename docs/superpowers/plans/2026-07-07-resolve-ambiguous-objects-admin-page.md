# Resolve Ambiguous Objects Admin Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a "Resolve Ambiguous Objects" admin page (System Admin → Database Maintenance) that lets an admin manually resolve `kb.artifact_objects` rows stuck at `reconcile_status = 'ambiguous'` by editing the artifact object and its candidate `kb.object_nodes` side by side.

**Architecture:** Four new Go/Echo endpoints reuse the existing `docprocessing.ArtifactObjectSQLStore` / `ObjectNodeSQLStore` / `ObjectReconciler` types (two new store methods + one new ranking function); a new self-fetching Svelte component wired into the existing System Admin → Database Maintenance nav group renders a left list / right two-block editor with Prev/Next/Cancel/Save/Help.

**Tech Stack:** Go + Echo + `database/sql` (Postgres) on the backend; Svelte 5 (runes) + TypeScript on the frontend; `go test` with `sqlmock`; `bun test` with `node:test`/`node:assert`.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-07-07-resolve-ambiguous-objects-admin-page-design.md` — every requirement below traces back to this file.
- Follow existing conventions exactly; do not introduce new abstractions (no new shared tag-input component, no shadcn `Dialog`/`Select` — this codebase's admin views use plain `<select>`/`<input>` and hand-rolled `.overlay`/`.dialog` modals, e.g. `summary-node-dialog.svelte`).
- Two independent PATCH calls on Save (artifact_object, then each edited node) — no combined transactional endpoint.
- `source_line_spans` and `ext_info` are NOT shown or editable in the UI.
- Candidates shown are exactly whatever `ObjectNodeSQLStore.FindCandidates` returns (no new/broader query) — this doc-processing method is unit-tested already and untouched by this plan.
- Do not refactor `ResolveAmbiguousArtifactObjects` (the existing automated backfill) to reuse the new `RankAmbiguousCandidates` helper — keep it as-is; only add the new function alongside it (surgical changes, per `ChenWeb/CLAUDE.md` §1.3).
- Go module: `github.com/chendingplano/deepdoc`. doc-processing package import path: `docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"`.
- Go tests use `github.com/DATA-DOG/go-sqlmock`; run with `go test ./server/api/doc-processing/... ./server/api/kbhandler/...`.
- Frontend tests use `node:test` + `node:assert/strict`, run with `bun test <path>` (NOT `node --test` — this repo has no compiled-JS step, and `bun test` is the only runner that resolves the `.js`-suffixed imports back to sibling `.ts` files).
- Frontend type-check: `bun run check` (runs `svelte-kit sync && svelte-check`) from `ChenWeb/web`.
- Commit with `jj describe -m "..."` then `jj new` after each task (per Workspace `CLAUDE.md`: "Always use jj").

---

### Task 1: doc-processing store additions (LoadByID, ListAmbiguousSummaries, RankAmbiguousCandidates)

**Files:**
- Modify: `server/api/doc-processing/object_ambiguous_resolution.go`
- Test: `server/api/doc-processing/object_ambiguous_resolution_test.go`

**Interfaces:**
- Consumes: existing `ArtifactObjectSQLStore` (DB field), `ArtifactObject` struct (`object_ambiguous_resolution.go`/`artifact_objects.go`), `ObjectReconciler`, `ObjectNodeCandidate`, `pickTieBreakCandidate` (unexported, same package), `jsonStringArray` (unexported helper, same package).
- Produces (used by Task 2):
  - `func (s ArtifactObjectSQLStore) ListAmbiguousSummaries(ctx context.Context) ([]AmbiguousObjectSummary, error)`
  - `type AmbiguousObjectSummary struct { ID int64; ArtifactType string; ArtifactID string; ObjectName string; ObjectNameEn string; Confidence float64 }`
  - `func (s ArtifactObjectSQLStore) LoadByID(ctx context.Context, id int64) (ArtifactObject, bool, error)`
  - `func RankAmbiguousCandidates(ctx context.Context, reconciler ObjectReconciler, obj ArtifactObject) ([]ObjectNodeCandidate, string, error)` — candidates sorted by score descending, plus the recommended candidate's `object_id` (empty string if no candidates).

- [ ] **Step 1: Write the failing tests**

Append to `server/api/doc-processing/object_ambiguous_resolution_test.go`:

```go
func TestArtifactObjectSQLStoreListAmbiguousSummariesReadsRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("FROM kb.artifact_objects").
		WithArgs(ObjectReconcileAmbiguous).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "artifact_type", "artifact_id", "object_name", "object_name_en", "confidence",
		}).AddRow(
			int64(7), searchArtifactProvision, "9_prv_1", "pressure regulator", "", 0.85,
		))

	store := ArtifactObjectSQLStore{DB: db}
	rows, err := store.ListAmbiguousSummaries(context.Background())
	if err != nil {
		t.Fatalf("ListAmbiguousSummaries: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != 7 || rows[0].ObjectName != "pressure regulator" {
		t.Fatalf("rows = %+v, unexpected", rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestArtifactObjectSQLStoreLoadByIDReadsRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("FROM kb.artifact_objects").
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "source_record_id", "input_record_id", "artifact_type", "artifact_id",
			"object_name", "object_name_en", "object_name_zh",
			"language", "object_type", "object_role",
			"aliases", "acronyms", "normalized_names",
			"description", "evidence_quote", "source_line_spans", "confidence",
			"object_id", "reconcile_status", "reconcile_confidence",
		}).AddRow(
			int64(7), int64(9), int64(9), searchArtifactProvision, "9_prv_1",
			"pressure regulator", "", "",
			"", "equipment", "regulated_object",
			[]byte(`["reg"]`), []byte(`[]`), []byte(`["pressure regulator"]`),
			"", "", []byte(`["8"]`), 0.85,
			"", ObjectReconcileAmbiguous, 0.0,
		))

	store := ArtifactObjectSQLStore{DB: db}
	obj, found, err := store.LoadByID(context.Background(), 7)
	if err != nil {
		t.Fatalf("LoadByID: %v", err)
	}
	if !found {
		t.Fatalf("found = false, want true")
	}
	if obj.ID != 7 || obj.ObjectName != "pressure regulator" || obj.ReconcileStatus != ObjectReconcileAmbiguous {
		t.Fatalf("obj = %+v, unexpected", obj)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestArtifactObjectSQLStoreLoadByIDReturnsNotFoundForMissingRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("FROM kb.artifact_objects").
		WithArgs(int64(999)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "source_record_id", "input_record_id", "artifact_type", "artifact_id",
			"object_name", "object_name_en", "object_name_zh",
			"language", "object_type", "object_role",
			"aliases", "acronyms", "normalized_names",
			"description", "evidence_quote", "source_line_spans", "confidence",
			"object_id", "reconcile_status", "reconcile_confidence",
		}))

	store := ArtifactObjectSQLStore{DB: db}
	_, found, err := store.LoadByID(context.Background(), 999)
	if err != nil {
		t.Fatalf("LoadByID: %v", err)
	}
	if found {
		t.Fatalf("found = true, want false")
	}
}

func TestRankAmbiguousCandidatesMarksRecommended(t *testing.T) {
	obj := ArtifactObject{NormalizedNames: []string{"pressure regulator", "reg"}}
	nodes := &stubObjectNodeStore{candidates: []ObjectNodeCandidate{
		{Node: ObjectNode{ObjectID: "obj_a", NormalizedNames: []string{"pressure regulator"}}, Score: 0.85, Method: "lexical_name"},
		{Node: ObjectNode{ObjectID: "obj_b", NormalizedNames: []string{"pressure regulator", "reg"}}, Score: 0.85, Method: "lexical_name"},
	}}
	reconciler := ObjectReconciler{Store: nodes}

	candidates, recommended, err := RankAmbiguousCandidates(context.Background(), reconciler, obj)
	if err != nil {
		t.Fatalf("RankAmbiguousCandidates: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("candidates = %+v, want 2", candidates)
	}
	if recommended != "obj_b" {
		t.Fatalf("recommended = %q, want obj_b (more normalized-name overlap)", recommended)
	}
}

func TestRankAmbiguousCandidatesReturnsEmptyRecommendedWhenNoCandidates(t *testing.T) {
	nodes := &stubObjectNodeStore{}
	reconciler := ObjectReconciler{Store: nodes}

	candidates, recommended, err := RankAmbiguousCandidates(context.Background(), reconciler, ArtifactObject{})
	if err != nil {
		t.Fatalf("RankAmbiguousCandidates: %v", err)
	}
	if len(candidates) != 0 || recommended != "" {
		t.Fatalf("candidates = %+v, recommended = %q, want empty", candidates, recommended)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail (functions don't exist yet)**

Run: `cd /Users/cding/Workspace/ChenWeb && go test ./server/api/doc-processing/... -run 'TestArtifactObjectSQLStoreListAmbiguousSummaries|TestArtifactObjectSQLStoreLoadByID|TestRankAmbiguousCandidates' -v`
Expected: FAIL with `undefined: ListAmbiguousSummaries` / `undefined: LoadByID` / `undefined: RankAmbiguousCandidates` (compile error).

- [ ] **Step 3: Implement the store methods and ranking function**

In `server/api/doc-processing/object_ambiguous_resolution.go`, change the import block to add `"database/sql"`:

```go
import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)
```

Append at the end of the file:

```go
// AmbiguousObjectSummary is the lightweight row shape used by the admin
// resolution page's left-panel list.
type AmbiguousObjectSummary struct {
	ID           int64
	ArtifactType string
	ArtifactID   string
	ObjectName   string
	ObjectNameEn string
	Confidence   float64
}

// ListAmbiguousSummaries returns every kb.artifact_objects row still at
// reconcile_status='ambiguous', for the admin resolution page. Unlike
// LoadAmbiguous (used by the automated backfill), this has no limit — the
// ADR-documented backlog is small (~40 rows).
func (s ArtifactObjectSQLStore) ListAmbiguousSummaries(ctx context.Context) ([]AmbiguousObjectSummary, error) {
	if s.DB == nil {
		return nil, fmt.Errorf("db is nil")
	}
	rows, err := s.DB.QueryContext(ctx, `
SELECT id, artifact_type, artifact_id, object_name, COALESCE(object_name_en, ''), confidence
FROM kb.artifact_objects
WHERE reconcile_status = $1
ORDER BY id`, ObjectReconcileAmbiguous)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []AmbiguousObjectSummary
	for rows.Next() {
		var row AmbiguousObjectSummary
		if err := rows.Scan(&row.ID, &row.ArtifactType, &row.ArtifactID, &row.ObjectName, &row.ObjectNameEn, &row.Confidence); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// LoadByID loads one kb.artifact_objects row by primary key, regardless of
// reconcile_status, for the admin resolution page's detail view. The second
// return value is false if no row matches.
func (s ArtifactObjectSQLStore) LoadByID(ctx context.Context, id int64) (ArtifactObject, bool, error) {
	if s.DB == nil {
		return ArtifactObject{}, false, fmt.Errorf("db is nil")
	}
	row := s.DB.QueryRowContext(ctx, `
SELECT id, source_record_id, input_record_id, artifact_type, artifact_id,
       object_name, COALESCE(object_name_en, ''), COALESCE(object_name_zh, ''),
       COALESCE(language, ''), object_type, object_role,
       COALESCE(aliases, '[]'::jsonb), COALESCE(acronyms, '[]'::jsonb), COALESCE(normalized_names, '[]'::jsonb),
       COALESCE(description, ''), COALESCE(evidence_quote, ''), COALESCE(source_line_spans, '[]'::jsonb), confidence,
       COALESCE(object_id, ''), reconcile_status, reconcile_confidence
FROM kb.artifact_objects
WHERE id = $1`, id)

	var (
		obj                                    ArtifactObject
		aliasesRaw, acronymsRaw, normalizedRaw []byte
		spansRaw                               []byte
	)
	err := row.Scan(
		&obj.ID, &obj.SourceRecordID, &obj.InputRecordID,
		&obj.ArtifactType, &obj.ArtifactID,
		&obj.ObjectName, &obj.ObjectNameEn, &obj.ObjectNameZh,
		&obj.Language, &obj.ObjectType, &obj.ObjectRole,
		&aliasesRaw, &acronymsRaw, &normalizedRaw,
		&obj.Description, &obj.EvidenceQuote, &spansRaw, &obj.Confidence,
		&obj.ObjectID, &obj.ReconcileStatus, &obj.ReconcileConfidence,
	)
	if err == sql.ErrNoRows {
		return ArtifactObject{}, false, nil
	}
	if err != nil {
		return ArtifactObject{}, false, err
	}
	obj.Aliases = jsonStringArray(aliasesRaw)
	obj.Acronyms = jsonStringArray(acronymsRaw)
	obj.NormalizedNames = jsonStringArray(normalizedRaw)
	_ = json.Unmarshal(spansRaw, &obj.SourceLineSpans)
	return obj, true, nil
}

// RankAmbiguousCandidates re-runs FindCandidates for obj and returns the
// results sorted by score descending, along with the object_id of the
// deterministic tie-break pick (pickTieBreakCandidate) so callers can mark a
// "recommended" entry. Used by the admin resolution page's detail endpoint.
// Deliberately separate from ResolveAmbiguousArtifactObjects's identical
// sort+pick sequence rather than extracted into it, to avoid touching that
// already-tested automated path for an unrelated feature.
func RankAmbiguousCandidates(ctx context.Context, reconciler ObjectReconciler, obj ArtifactObject) ([]ObjectNodeCandidate, string, error) {
	candidates, err := reconciler.Store.FindCandidates(ctx, obj, reconciler.Options)
	if err != nil {
		return nil, "", err
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Score > candidates[j].Score })
	recommended := ""
	if len(candidates) > 0 {
		recommended = pickTieBreakCandidate(obj, candidates).Node.ObjectID
	}
	return candidates, recommended, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/cding/Workspace/ChenWeb && go test ./server/api/doc-processing/... -run 'TestArtifactObjectSQLStoreListAmbiguousSummaries|TestArtifactObjectSQLStoreLoadByID|TestRankAmbiguousCandidates' -v`
Expected: PASS (5 tests).

- [ ] **Step 5: Run the full doc-processing package test suite to check for regressions**

Run: `cd /Users/cding/Workspace/ChenWeb && go test ./server/api/doc-processing/... -run 'TestArtifactObjectSQLStore|TestResolveAmbiguousArtifactObjects|TestPickTieBreakCandidate|TestRankAmbiguousCandidates'`
Expected: PASS (all pre-existing ambiguous-object tests plus the new ones).

- [ ] **Step 6: Commit**

```bash
cd /Users/cding/Workspace/ChenWeb
jj describe -m "Add LoadByID/ListAmbiguousSummaries/RankAmbiguousCandidates for the ambiguous-objects admin page"
jj new
```

---

### Task 2: kbhandler GET endpoints (list + detail)

**Files:**
- Create: `server/api/kbhandler/ambiguous_objects_handler.go`

**Interfaces:**
- Consumes: `docprocessing.ArtifactObjectSQLStore.ListAmbiguousSummaries` / `.LoadByID`, `docprocessing.RankAmbiguousCandidates`, `docprocessing.ObjectNodeSQLStore`, `docprocessing.ObjectReconciler`, `docprocessing.ObjectReconcileOptionsFromEnv` (all from Task 1 / pre-existing), `errorResponse` (`handler.go:64`), `EchoFactory.NewFromEcho`, `ApiTypes.ProjectDBHandle`.
- Produces (used by Task 3, 4, 5):
  - `func ListAmbiguousObjects(c echo.Context) error`
  - `func GetAmbiguousObjectDetail(c echo.Context) error`
  - `type artifactObjectDTO struct{...}` with `json` tags, and `func toArtifactObjectDTO(obj docprocessing.ArtifactObject) artifactObjectDTO` (reused by Task 3).
  - `type objectNodeCandidateDTO struct{...}` with `json` tags, and `func toObjectNodeCandidateDTO(c docprocessing.ObjectNodeCandidate, recommendedID string) objectNodeCandidateDTO`.
  - `func nonNilStrings(s []string) []string` (local helper — do not use doc-processing's unexported `orEmptySlice`, it isn't visible from this package).

This task has no dedicated handler test file: both handlers are thin wrappers over the already-unit-tested Task 1 functions, matching the existing `ResolveAmbiguousObjects` handler in `resolve_ambiguous_objects_handler.go` (which also has no test file for the same reason — see Global Constraints). Verification is `go build`/`go vet` plus the Task 1 test suite already covering the underlying logic.

- [ ] **Step 1: Write the handler file**

Create `server/api/kbhandler/ambiguous_objects_handler.go`:

```go
package kbhandler

import (
	"net/http"
	"strconv"
	"strings"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// artifactObjectDTO is the wire shape for one kb.artifact_objects row on the
// Resolve Ambiguous Objects admin page.
type artifactObjectDTO struct {
	ID                  int64    `json:"id"`
	SourceRecordID      int64    `json:"source_record_id"`
	ArtifactType        string   `json:"artifact_type"`
	ArtifactID          string   `json:"artifact_id"`
	ObjectName          string   `json:"object_name"`
	ObjectNameEn        string   `json:"object_name_en"`
	ObjectNameZh        string   `json:"object_name_zh"`
	Language            string   `json:"language"`
	ObjectType          string   `json:"object_type"`
	ObjectRole          string   `json:"object_role"`
	Aliases             []string `json:"aliases"`
	Acronyms            []string `json:"acronyms"`
	Description         string   `json:"description"`
	EvidenceQuote       string   `json:"evidence_quote"`
	ObjectID            string   `json:"object_id"`
	ReconcileStatus     string   `json:"reconcile_status"`
	ReconcileConfidence float64  `json:"reconcile_confidence"`
}

// objectNodeCandidateDTO is the wire shape for one candidate kb.object_nodes
// row shown alongside an ambiguous artifact_object.
type objectNodeCandidateDTO struct {
	ObjectID        string   `json:"object_id"`
	CanonicalName   string   `json:"canonical_name"`
	CanonicalNameEn string   `json:"canonical_name_en"`
	CanonicalNameZh string   `json:"canonical_name_zh"`
	PrimaryLanguage string   `json:"primary_language"`
	ObjectType      string   `json:"object_type"`
	Aliases         []string `json:"aliases"`
	Acronyms        []string `json:"acronyms"`
	Description     string   `json:"description"`
	Score           float64  `json:"score"`
	Method          string   `json:"method"`
	Recommended     bool     `json:"recommended"`
}

// ambiguousObjectSummaryDTO is the wire shape for the left-panel list.
type ambiguousObjectSummaryDTO struct {
	ID           int64   `json:"id"`
	ArtifactType string  `json:"artifact_type"`
	ArtifactID   string  `json:"artifact_id"`
	ObjectName   string  `json:"object_name"`
	ObjectNameEn string  `json:"object_name_en"`
	Confidence   float64 `json:"confidence"`
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func toArtifactObjectDTO(obj docprocessing.ArtifactObject) artifactObjectDTO {
	return artifactObjectDTO{
		ID:                  obj.ID,
		SourceRecordID:      obj.SourceRecordID,
		ArtifactType:        obj.ArtifactType,
		ArtifactID:          obj.ArtifactID,
		ObjectName:          obj.ObjectName,
		ObjectNameEn:        obj.ObjectNameEn,
		ObjectNameZh:        obj.ObjectNameZh,
		Language:            obj.Language,
		ObjectType:          obj.ObjectType,
		ObjectRole:          obj.ObjectRole,
		Aliases:             nonNilStrings(obj.Aliases),
		Acronyms:            nonNilStrings(obj.Acronyms),
		Description:         obj.Description,
		EvidenceQuote:       obj.EvidenceQuote,
		ObjectID:            obj.ObjectID,
		ReconcileStatus:     obj.ReconcileStatus,
		ReconcileConfidence: obj.ReconcileConfidence,
	}
}

func toObjectNodeCandidateDTO(c docprocessing.ObjectNodeCandidate, recommendedID string) objectNodeCandidateDTO {
	return objectNodeCandidateDTO{
		ObjectID:        c.Node.ObjectID,
		CanonicalName:   c.Node.CanonicalName,
		CanonicalNameEn: c.Node.CanonicalNameEn,
		CanonicalNameZh: c.Node.CanonicalNameZh,
		PrimaryLanguage: c.Node.PrimaryLanguage,
		ObjectType:      c.Node.ObjectType,
		Aliases:         nonNilStrings(c.Node.Aliases),
		Acronyms:        nonNilStrings(c.Node.Acronyms),
		Description:     c.Node.Description,
		Score:           c.Score,
		Method:          c.Method,
		Recommended:     recommendedID != "" && c.Node.ObjectID == recommendedID,
	}
}

// ListAmbiguousObjects handles GET /api/v1/kb/objects/ambiguous — the
// left-panel list for the Resolve Ambiguous Objects admin page.
func ListAmbiguousObjects(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_AAO_001")
	defer rc.Close()
	logger := rc.GetLogger()

	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "db not initialized (CWB_KB_AAO_010)"})
	}

	store := docprocessing.ArtifactObjectSQLStore{DB: db}
	rows, err := store.ListAmbiguousSummaries(c.Request().Context())
	if err != nil {
		logger.Error("list ambiguous objects failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to list ambiguous objects (CWB_KB_AAO_011)"})
	}

	out := make([]ambiguousObjectSummaryDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, ambiguousObjectSummaryDTO{
			ID:           row.ID,
			ArtifactType: row.ArtifactType,
			ArtifactID:   row.ArtifactID,
			ObjectName:   row.ObjectName,
			ObjectNameEn: row.ObjectNameEn,
			Confidence:   row.Confidence,
		})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "rows": out})
}

// GetAmbiguousObjectDetail handles GET /api/v1/kb/objects/ambiguous/:id — the
// right-panel detail (artifact object + ranked candidate object nodes) for
// one ambiguous row.
func GetAmbiguousObjectDetail(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_AAO_100")
	defer rc.Close()
	logger := rc.GetLogger()

	idStr := strings.TrimSpace(c.Param("id"))
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid id (CWB_KB_AAO_101)"})
	}

	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "db not initialized (CWB_KB_AAO_110)"})
	}

	store := docprocessing.ArtifactObjectSQLStore{DB: db}
	obj, found, err := store.LoadByID(c.Request().Context(), id)
	if err != nil {
		logger.Error("load artifact object failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to load artifact object (CWB_KB_AAO_111)"})
	}
	if !found {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "artifact object not found (CWB_KB_AAO_112)"})
	}

	reconciler := docprocessing.ObjectReconciler{
		Store:   docprocessing.ObjectNodeSQLStore{DB: db},
		Options: docprocessing.ObjectReconcileOptionsFromEnv(),
	}
	candidates, recommendedID, err := docprocessing.RankAmbiguousCandidates(c.Request().Context(), reconciler, obj)
	if err != nil {
		logger.Error("rank ambiguous candidates failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to load candidates (CWB_KB_AAO_113)"})
	}

	candidateDTOs := make([]objectNodeCandidateDTO, 0, len(candidates))
	for _, cand := range candidates {
		candidateDTOs = append(candidateDTOs, toObjectNodeCandidateDTO(cand, recommendedID))
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status":          true,
		"artifact_object": toArtifactObjectDTO(obj),
		"candidates":      candidateDTOs,
	})
}
```

- [ ] **Step 2: Build and vet to verify it compiles**

Run: `cd /Users/cding/Workspace/ChenWeb && go build ./... && go vet ./server/api/kbhandler/... ./server/api/doc-processing/...`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
cd /Users/cding/Workspace/ChenWeb
jj describe -m "Add ListAmbiguousObjects/GetAmbiguousObjectDetail GET handlers for the admin page"
jj new
```

---

### Task 3: kbhandler PATCH endpoint — UpdateArtifactObject

**Files:**
- Modify: `server/api/kbhandler/ambiguous_objects_handler.go`
- Test: `server/api/kbhandler/ambiguous_objects_handler_test.go` (new)

**Interfaces:**
- Consumes: `errorResponse`, `decodeStringValue` (`metrics_handler.go:344`), `compactJSONRaw` (`metrics_handler.go:361`), `decodeFloat64Value` (`metrics_handler.go:422`), `docprocessing.ObjectReconcile*` status constants (`artifact_objects.go:15-21`).
- Produces (used by Task 5 routes wiring): `func UpdateArtifactObject(c echo.Context) error`.

- [ ] **Step 1: Write the failing tests**

Create `server/api/kbhandler/ambiguous_objects_handler_test.go`:

```go
package kbhandler

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func newUpdateArtifactObjectContext(t *testing.T, id string, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/kb/objects/artifact-objects/"+id, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/kb/objects/artifact-objects/:id")
	c.SetParamNames("id")
	c.SetParamValues(id)
	return c, rec
}

func TestUpdateArtifactObjectSuccessStampsExtInfoWhenObjectIDSet(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	updateQuery := regexp.QuoteMeta(
		"UPDATE kb.artifact_objects SET object_id = $1, object_name = $2, reconcile_status = $3, ext_info = COALESCE(ext_info, '{}'::jsonb) || $4::jsonb WHERE id = $5",
	)
	mock.ExpectExec(updateQuery).
		WithArgs("obj_b", "Pressure Regulator", "ambiguous_resolved", `{"reconcile_method":"manual_admin"}`, int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	c, rec := newUpdateArtifactObjectContext(t, "42", `{
		"object_id":"obj_b",
		"object_name":"Pressure Regulator",
		"reconcile_status":"ambiguous_resolved"
	}`)
	if err := UpdateArtifactObject(c); err != nil {
		t.Fatalf("UpdateArtifactObject returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestUpdateArtifactObjectRejectsInvalidReconcileStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	c, rec := newUpdateArtifactObjectContext(t, "42", `{"reconcile_status":"bogus"}`)
	if err := UpdateArtifactObject(c); err != nil {
		t.Fatalf("UpdateArtifactObject returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestUpdateArtifactObjectRejectsNullObjectName(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	c, rec := newUpdateArtifactObjectContext(t, "42", `{"object_name":null}`)
	if err := UpdateArtifactObject(c); err != nil {
		t.Fatalf("UpdateArtifactObject returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestUpdateArtifactObjectNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	updateQuery := regexp.QuoteMeta("UPDATE kb.artifact_objects SET description = $1 WHERE id = $2")
	mock.ExpectExec(updateQuery).
		WithArgs("no such row", int64(999)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	c, rec := newUpdateArtifactObjectContext(t, "999", `{"description":"no such row"}`)
	if err := UpdateArtifactObject(c); err != nil {
		t.Fatalf("UpdateArtifactObject returned error: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/cding/Workspace/ChenWeb && go test ./server/api/kbhandler/... -run TestUpdateArtifactObject -v`
Expected: FAIL with `undefined: UpdateArtifactObject` (compile error).

- [ ] **Step 3: Implement UpdateArtifactObject**

Append to `server/api/kbhandler/ambiguous_objects_handler.go` (add `"encoding/json"`, `"fmt"`, `"sort"` to the existing import block):

```go
import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)
```

```go
var artifactObjectReconcileStatuses = map[string]struct{}{
	docprocessing.ObjectReconcilePending:           {},
	docprocessing.ObjectReconcileMatched:           {},
	docprocessing.ObjectReconcileNew:               {},
	docprocessing.ObjectReconcileAmbiguous:         {},
	docprocessing.ObjectReconcileAmbiguousResolved: {},
	docprocessing.ObjectReconcileRejected:          {},
}

// UpdateArtifactObject handles PATCH /api/v1/kb/objects/artifact-objects/:id
// — partial update of one kb.artifact_objects row from the admin resolution
// page. Setting a non-empty object_id also stamps ext_info.reconcile_method
// = "manual_admin" (merged into existing ext_info, not overwritten) so
// provenance distinguishes manual resolutions from the automated backfill's
// tie_break_deterministic / exact_name / lexical_name / new_node methods.
func UpdateArtifactObject(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_AAO_200")
	defer rc.Close()
	logger := rc.GetLogger()

	idStr := strings.TrimSpace(c.Param("id"))
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid id (CWB_KB_AAO_201)"})
	}

	var payload map[string]json.RawMessage
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_AAO_202)"})
	}

	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "db not initialized (CWB_KB_AAO_210)"})
	}

	sets := make([]string, 0, len(payload)+1)
	args := make([]any, 0, len(payload)+2)
	addSet := func(column string, value any) {
		args = append(args, value)
		sets = append(sets, fmt.Sprintf("%s = $%d", column, len(args)))
	}

	fields := make([]string, 0, len(payload))
	for field := range payload {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	var settingObjectID bool
	for _, field := range fields {
		raw := payload[field]
		switch field {
		case "object_name", "object_type", "object_role", "reconcile_status":
			value, err := decodeStringValue(raw, true)
			if err != nil || value == nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status: false, ErrorMsg: fmt.Sprintf("%s cannot be null (CWB_KB_AAO_211)", field),
				})
			}
			if field == "reconcile_status" {
				if _, ok := artifactObjectReconcileStatuses[*value]; !ok {
					return c.JSON(http.StatusBadRequest, errorResponse{
						Status: false, ErrorMsg: fmt.Sprintf("invalid reconcile_status %q (CWB_KB_AAO_212)", *value),
					})
				}
			}
			addSet(field, *value)

		case "object_name_en", "object_name_zh", "language", "description", "evidence_quote":
			value, err := decodeStringValue(raw, true)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status: false, ErrorMsg: fmt.Sprintf("invalid %s: %v (CWB_KB_AAO_213)", field, err),
				})
			}
			if value == nil || *value == "" {
				addSet(field, nil)
			} else {
				addSet(field, *value)
			}

		case "object_id":
			value, err := decodeStringValue(raw, true)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status: false, ErrorMsg: fmt.Sprintf("invalid object_id: %v (CWB_KB_AAO_214)", err),
				})
			}
			if value == nil || *value == "" {
				addSet("object_id", nil)
			} else {
				addSet("object_id", *value)
				settingObjectID = true
			}

		case "aliases", "acronyms":
			if strings.TrimSpace(string(raw)) == "null" {
				addSet(field, "[]")
				break
			}
			compact, err := compactJSONRaw(raw)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status: false, ErrorMsg: fmt.Sprintf("invalid %s: %v (CWB_KB_AAO_215)", field, err),
				})
			}
			addSet(field, compact)

		case "reconcile_confidence":
			value, err := decodeFloat64Value(raw)
			if err != nil || value == nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status: false, ErrorMsg: fmt.Sprintf("invalid reconcile_confidence: %v (CWB_KB_AAO_216)", err),
				})
			}
			addSet(field, *value)
		}
	}

	if settingObjectID {
		args = append(args, `{"reconcile_method":"manual_admin"}`)
		sets = append(sets, fmt.Sprintf("ext_info = COALESCE(ext_info, '{}'::jsonb) || $%d::jsonb", len(args)))
	}

	if len(sets) == 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "no editable fields in request (CWB_KB_AAO_217)"})
	}

	query := fmt.Sprintf("UPDATE kb.artifact_objects SET %s WHERE id = $%d", strings.Join(sets, ", "), len(args)+1)
	args = append(args, id)
	result, err := db.Exec(query, args...)
	if err != nil {
		logger.Error("update artifact object failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to update artifact object (CWB_KB_AAO_218)"})
	}
	affected, err := result.RowsAffected()
	if err != nil {
		logger.Error("rows affected artifact object failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to verify artifact object update (CWB_KB_AAO_219)"})
	}
	if affected == 0 {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "artifact object not found (CWB_KB_AAO_220)"})
	}

	return c.JSON(http.StatusOK, map[string]any{"status": true})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/cding/Workspace/ChenWeb && go test ./server/api/kbhandler/... -run TestUpdateArtifactObject -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
cd /Users/cding/Workspace/ChenWeb
jj describe -m "Add UpdateArtifactObject PATCH handler for the ambiguous-objects admin page"
jj new
```

---

### Task 4: kbhandler PATCH endpoint — UpdateObjectNode

**Files:**
- Modify: `server/api/kbhandler/ambiguous_objects_handler.go`
- Modify: `server/api/kbhandler/ambiguous_objects_handler_test.go`

**Interfaces:**
- Consumes: same helpers as Task 3 (`decodeStringValue`, `compactJSONRaw`).
- Produces (used by Task 5): `func UpdateObjectNode(c echo.Context) error`.

- [ ] **Step 1: Write the failing tests**

Append to `server/api/kbhandler/ambiguous_objects_handler_test.go`:

```go
func newUpdateObjectNodeContext(t *testing.T, objectID string, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/kb/object-nodes/"+objectID, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/kb/object-nodes/:object_id")
	c.SetParamNames("object_id")
	c.SetParamValues(objectID)
	return c, rec
}

func TestUpdateObjectNodeSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	updateQuery := regexp.QuoteMeta("UPDATE kb.object_nodes SET aliases = $1, canonical_name = $2 WHERE object_id = $3")
	mock.ExpectExec(updateQuery).
		WithArgs(`["reg","regulator"]`, "Pressure Regulator", "obj_a").
		WillReturnResult(sqlmock.NewResult(0, 1))

	c, rec := newUpdateObjectNodeContext(t, "obj_a", `{
		"aliases":["reg","regulator"],
		"canonical_name":"Pressure Regulator"
	}`)
	if err := UpdateObjectNode(c); err != nil {
		t.Fatalf("UpdateObjectNode returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestUpdateObjectNodeRejectsNullCanonicalName(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	c, rec := newUpdateObjectNodeContext(t, "obj_a", `{"canonical_name":null}`)
	if err := UpdateObjectNode(c); err != nil {
		t.Fatalf("UpdateObjectNode returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestUpdateObjectNodeNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	updateQuery := regexp.QuoteMeta("UPDATE kb.object_nodes SET description = $1 WHERE object_id = $2")
	mock.ExpectExec(updateQuery).
		WithArgs("no such node", "obj_missing").
		WillReturnResult(sqlmock.NewResult(0, 0))

	c, rec := newUpdateObjectNodeContext(t, "obj_missing", `{"description":"no such node"}`)
	if err := UpdateObjectNode(c); err != nil {
		t.Fatalf("UpdateObjectNode returned error: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/cding/Workspace/ChenWeb && go test ./server/api/kbhandler/... -run TestUpdateObjectNode -v`
Expected: FAIL with `undefined: UpdateObjectNode` (compile error).

- [ ] **Step 3: Implement UpdateObjectNode**

Append to `server/api/kbhandler/ambiguous_objects_handler.go`:

```go
// UpdateObjectNode handles PATCH /api/v1/kb/object-nodes/:object_id —
// partial update of one kb.object_nodes candidate row from the admin
// resolution page.
func UpdateObjectNode(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_AAO_300")
	defer rc.Close()
	logger := rc.GetLogger()

	objectID := strings.TrimSpace(c.Param("object_id"))
	if objectID == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid object_id (CWB_KB_AAO_301)"})
	}

	var payload map[string]json.RawMessage
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_AAO_302)"})
	}

	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "db not initialized (CWB_KB_AAO_310)"})
	}

	sets := make([]string, 0, len(payload))
	args := make([]any, 0, len(payload)+1)
	addSet := func(column string, value any) {
		args = append(args, value)
		sets = append(sets, fmt.Sprintf("%s = $%d", column, len(args)))
	}

	fields := make([]string, 0, len(payload))
	for field := range payload {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	for _, field := range fields {
		raw := payload[field]
		switch field {
		case "canonical_name", "object_type":
			value, err := decodeStringValue(raw, true)
			if err != nil || value == nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status: false, ErrorMsg: fmt.Sprintf("%s cannot be null (CWB_KB_AAO_311)", field),
				})
			}
			addSet(field, *value)

		case "canonical_name_en", "canonical_name_zh", "primary_language", "description":
			value, err := decodeStringValue(raw, true)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status: false, ErrorMsg: fmt.Sprintf("invalid %s: %v (CWB_KB_AAO_312)", field, err),
				})
			}
			if value == nil || *value == "" {
				addSet(field, nil)
			} else {
				addSet(field, *value)
			}

		case "aliases", "acronyms":
			if strings.TrimSpace(string(raw)) == "null" {
				addSet(field, "[]")
				break
			}
			compact, err := compactJSONRaw(raw)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{
					Status: false, ErrorMsg: fmt.Sprintf("invalid %s: %v (CWB_KB_AAO_313)", field, err),
				})
			}
			addSet(field, compact)
		}
	}

	if len(sets) == 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "no editable fields in request (CWB_KB_AAO_314)"})
	}

	query := fmt.Sprintf("UPDATE kb.object_nodes SET %s WHERE object_id = $%d", strings.Join(sets, ", "), len(args)+1)
	args = append(args, objectID)
	result, err := db.Exec(query, args...)
	if err != nil {
		logger.Error("update object node failed", "object_id", objectID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to update object node (CWB_KB_AAO_315)"})
	}
	affected, err := result.RowsAffected()
	if err != nil {
		logger.Error("rows affected object node failed", "object_id", objectID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to verify object node update (CWB_KB_AAO_316)"})
	}
	if affected == 0 {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "object node not found (CWB_KB_AAO_317)"})
	}

	return c.JSON(http.StatusOK, map[string]any{"status": true})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/cding/Workspace/ChenWeb && go test ./server/api/kbhandler/... -run TestUpdateObjectNode -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Run the full kbhandler + doc-processing test suites to check for regressions**

Run: `cd /Users/cding/Workspace/ChenWeb && go build ./... && go vet ./server/api/... && go test ./server/api/kbhandler/... ./server/api/doc-processing/...`
Expected: PASS, no build/vet errors.

- [ ] **Step 6: Commit**

```bash
cd /Users/cding/Workspace/ChenWeb
jj describe -m "Add UpdateObjectNode PATCH handler for the ambiguous-objects admin page"
jj new
```

---

### Task 5: Route wiring

**Files:**
- Modify: `server/api/routes.go`

**Interfaces:**
- Consumes: `kbhandler.ListAmbiguousObjects`, `kbhandler.GetAmbiguousObjectDetail`, `kbhandler.UpdateArtifactObject`, `kbhandler.UpdateObjectNode` (Tasks 2–4).
- Produces: live routes `GET /api/v1/kb/objects/ambiguous`, `GET /api/v1/kb/objects/ambiguous/:id`, `PATCH /api/v1/kb/objects/artifact-objects/:id`, `PATCH /api/v1/kb/object-nodes/:object_id`.

- [ ] **Step 1: Add the routes**

In `server/api/routes.go`, right after line 331 (`apiGroup.POST("/kb/objects/resolve-ambiguous", kbhandler.ResolveAmbiguousObjects)`):

```go
	apiGroup.GET("/kb/objects/ambiguous", kbhandler.ListAmbiguousObjects)
	apiGroup.GET("/kb/objects/ambiguous/:id", kbhandler.GetAmbiguousObjectDetail)
	apiGroup.PATCH("/kb/objects/artifact-objects/:id", kbhandler.UpdateArtifactObject)
	apiGroup.PATCH("/kb/object-nodes/:object_id", kbhandler.UpdateObjectNode)
```

- [ ] **Step 2: Build to verify routes.go compiles and the binary still starts**

Run: `cd /Users/cding/Workspace/ChenWeb && go build ./...`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
cd /Users/cding/Workspace/ChenWeb
jj describe -m "Wire ambiguous-objects admin page routes into routes.go"
jj new
```

---

### Task 6: Frontend API client + pure logic (resolve-ambiguous-objects-client.ts)

**Files:**
- Create: `web/src/lib/components/home3/resolve-ambiguous-objects-client.ts`
- Test: `web/src/lib/components/home3/resolve-ambiguous-objects-client.test.ts`

**Interfaces:**
- Consumes: nothing beyond the global `fetch`.
- Produces (used by Task 7–9): types `AmbiguousObjectSummary`, `ArtifactObjectDetail`, `ObjectNodeCandidate`, `AmbiguousObjectDetailResponse`; constants `ARTIFACT_OBJECT_EDITABLE_FIELDS`, `OBJECT_NODE_EDITABLE_FIELDS`, `RECONCILE_STATUS_OPTIONS`; functions `listAmbiguousObjects()`, `getAmbiguousObjectDetail(id)`, `updateArtifactObject(id, patch)`, `updateObjectNode(objectId, patch)`, `buildArtifactObjectPatch(original, edited)`, `buildObjectNodePatch(original, edited)`, `neighborAmbiguousId(ids, currentId, direction)`.

- [ ] **Step 1: Write the failing tests**

Create `web/src/lib/components/home3/resolve-ambiguous-objects-client.test.ts`:

```ts
import test from 'node:test';
import assert from 'node:assert/strict';

import {
	buildArtifactObjectPatch,
	buildObjectNodePatch,
	getAmbiguousObjectDetail,
	listAmbiguousObjects,
	neighborAmbiguousId,
	updateArtifactObject,
	updateObjectNode,
	type ArtifactObjectDetail,
	type ObjectNodeCandidate
} from './resolve-ambiguous-objects-client.js';

type FetchCall = { input: string | URL | Request; init?: RequestInit };

function installFetchMock(handler: (call: FetchCall) => Promise<Response>) {
	const originalFetch = globalThis.fetch;
	const calls: FetchCall[] = [];
	globalThis.fetch = (async (input: string | URL | Request, init?: RequestInit) => {
		const call = { input, init };
		calls.push(call);
		return handler(call);
	}) as typeof fetch;
	return {
		calls,
		restore() {
			globalThis.fetch = originalFetch;
		}
	};
}

const baseArtifactObject: ArtifactObjectDetail = {
	id: 101,
	source_record_id: 9,
	artifact_type: 'provision',
	artifact_id: '9_prv_1',
	object_name: 'pressure regulator',
	object_name_en: '',
	object_name_zh: '',
	language: '',
	object_type: 'equipment',
	object_role: 'regulated_object',
	aliases: ['reg'],
	acronyms: [],
	description: '',
	evidence_quote: '',
	object_id: '',
	reconcile_status: 'ambiguous',
	reconcile_confidence: 0
};

const baseCandidate: ObjectNodeCandidate = {
	object_id: 'obj_a',
	canonical_name: 'Pressure Regulator',
	canonical_name_en: '',
	canonical_name_zh: '',
	primary_language: '',
	object_type: 'equipment',
	aliases: [],
	acronyms: [],
	description: '',
	score: 0.85,
	method: 'lexical_name',
	recommended: true
};

test('listAmbiguousObjects loads the ambiguous rows list', async () => {
	const mock = installFetchMock(async () =>
		Response.json({
			status: true,
			rows: [
				{
					id: 101,
					artifact_type: 'provision',
					artifact_id: '9_prv_1',
					object_name: 'pressure regulator',
					object_name_en: '',
					confidence: 0.85
				}
			]
		})
	);
	try {
		const response = await listAmbiguousObjects();
		assert.equal(mock.calls.length, 1);
		assert.equal(String(mock.calls[0].input), '/api/v1/kb/objects/ambiguous');
		assert.equal(response.rows.length, 1);
		assert.equal(response.rows[0].object_name, 'pressure regulator');
	} finally {
		mock.restore();
	}
});

test('getAmbiguousObjectDetail loads the artifact object and candidates for one id', async () => {
	const mock = installFetchMock(async () =>
		Response.json({ status: true, artifact_object: baseArtifactObject, candidates: [baseCandidate] })
	);
	try {
		const response = await getAmbiguousObjectDetail(101);
		assert.equal(String(mock.calls[0].input), '/api/v1/kb/objects/ambiguous/101');
		assert.equal(response.artifact_object.object_name, 'pressure regulator');
		assert.equal(response.candidates[0].recommended, true);
	} finally {
		mock.restore();
	}
});

test('updateArtifactObject PATCHes the artifact-objects endpoint with the given patch', async () => {
	const mock = installFetchMock(async () => Response.json({ status: true }));
	try {
		await updateArtifactObject(101, { object_id: 'obj_a', reconcile_status: 'ambiguous_resolved' });
		assert.equal(String(mock.calls[0].input), '/api/v1/kb/objects/artifact-objects/101');
		assert.equal(mock.calls[0].init?.method, 'PATCH');
		assert.equal(
			mock.calls[0].init?.body,
			JSON.stringify({ object_id: 'obj_a', reconcile_status: 'ambiguous_resolved' })
		);
	} finally {
		mock.restore();
	}
});

test('updateObjectNode PATCHes the object-nodes endpoint keyed by object_id', async () => {
	const mock = installFetchMock(async () => Response.json({ status: true }));
	try {
		await updateObjectNode('obj_a', { canonical_name: 'Pressure Regulator' });
		assert.equal(String(mock.calls[0].input), '/api/v1/kb/object-nodes/obj_a');
		assert.equal(mock.calls[0].init?.method, 'PATCH');
	} finally {
		mock.restore();
	}
});

test('buildArtifactObjectPatch only includes changed editable fields', () => {
	const edited = { ...baseArtifactObject, object_name: 'Pressure Regulator', aliases: ['reg', 'regulator'] };
	const patch = buildArtifactObjectPatch(baseArtifactObject, edited);
	assert.deepEqual(patch, { object_name: 'Pressure Regulator', aliases: ['reg', 'regulator'] });
});

test('buildArtifactObjectPatch returns empty object when nothing changed', () => {
	const patch = buildArtifactObjectPatch(baseArtifactObject, { ...baseArtifactObject });
	assert.deepEqual(patch, {});
});

test('buildObjectNodePatch only includes changed editable fields', () => {
	const edited = { ...baseCandidate, description: 'Regulates line pressure' };
	const patch = buildObjectNodePatch(baseCandidate, edited);
	assert.deepEqual(patch, { description: 'Regulates line pressure' });
});

test('neighborAmbiguousId moves within the id list and returns null past the ends', () => {
	const ids = [101, 102, 103];
	assert.equal(neighborAmbiguousId(ids, 101, 1), 102);
	assert.equal(neighborAmbiguousId(ids, 103, 1), null);
	assert.equal(neighborAmbiguousId(ids, 101, -1), null);
	assert.equal(neighborAmbiguousId(ids, 102, -1), 101);
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/cding/Workspace/ChenWeb/web && bun test src/lib/components/home3/resolve-ambiguous-objects-client.test.ts`
Expected: FAIL — module `./resolve-ambiguous-objects-client.js` not found.

- [ ] **Step 3: Implement the client module**

Create `web/src/lib/components/home3/resolve-ambiguous-objects-client.ts`:

```ts
export type AmbiguousObjectSummary = {
	id: number;
	artifact_type: string;
	artifact_id: string;
	object_name: string;
	object_name_en: string;
	confidence: number;
};

export type ArtifactObjectDetail = {
	id: number;
	source_record_id: number;
	artifact_type: string;
	artifact_id: string;
	object_name: string;
	object_name_en: string;
	object_name_zh: string;
	language: string;
	object_type: string;
	object_role: string;
	aliases: string[];
	acronyms: string[];
	description: string;
	evidence_quote: string;
	object_id: string;
	reconcile_status: string;
	reconcile_confidence: number;
};

export type ObjectNodeCandidate = {
	object_id: string;
	canonical_name: string;
	canonical_name_en: string;
	canonical_name_zh: string;
	primary_language: string;
	object_type: string;
	aliases: string[];
	acronyms: string[];
	description: string;
	score: number;
	method: string;
	recommended: boolean;
};

export type AmbiguousObjectDetailResponse = {
	status: boolean;
	artifact_object: ArtifactObjectDetail;
	candidates: ObjectNodeCandidate[];
};

export const ARTIFACT_OBJECT_EDITABLE_FIELDS = [
	'object_name',
	'object_name_en',
	'object_name_zh',
	'language',
	'object_type',
	'object_role',
	'aliases',
	'acronyms',
	'description',
	'evidence_quote',
	'object_id',
	'reconcile_status',
	'reconcile_confidence'
] as const;

export const OBJECT_NODE_EDITABLE_FIELDS = [
	'canonical_name',
	'canonical_name_en',
	'canonical_name_zh',
	'primary_language',
	'object_type',
	'aliases',
	'acronyms',
	'description'
] as const;

export const RECONCILE_STATUS_OPTIONS = [
	'pending',
	'matched',
	'new',
	'ambiguous',
	'ambiguous_resolved',
	'rejected'
] as const;

async function req<T>(path: string, init?: RequestInit): Promise<T> {
	const res = await fetch(path, { credentials: 'same-origin', ...init });
	const text = await res.text();
	let parsed: unknown = null;
	if (text) {
		try {
			parsed = JSON.parse(text);
		} catch {
			parsed = null;
		}
	}
	if (!res.ok) {
		const msg =
			parsed && typeof parsed === 'object' && parsed !== null && 'error_msg' in parsed
				? String((parsed as { error_msg: unknown }).error_msg)
				: `HTTP ${res.status}`;
		throw new Error(msg);
	}
	return parsed as T;
}

export function listAmbiguousObjects(): Promise<{ status: boolean; rows: AmbiguousObjectSummary[] }> {
	return req('/api/v1/kb/objects/ambiguous');
}

export function getAmbiguousObjectDetail(id: number): Promise<AmbiguousObjectDetailResponse> {
	return req(`/api/v1/kb/objects/ambiguous/${id}`);
}

export function updateArtifactObject(
	id: number,
	patch: Record<string, unknown>
): Promise<{ status: boolean }> {
	return req(`/api/v1/kb/objects/artifact-objects/${id}`, {
		method: 'PATCH',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(patch)
	});
}

export function updateObjectNode(
	objectId: string,
	patch: Record<string, unknown>
): Promise<{ status: boolean }> {
	return req(`/api/v1/kb/object-nodes/${encodeURIComponent(objectId)}`, {
		method: 'PATCH',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(patch)
	});
}

function diffFields<T extends Record<string, unknown>>(
	original: T,
	edited: T,
	fields: readonly string[]
): Record<string, unknown> {
	const patch: Record<string, unknown> = {};
	for (const field of fields) {
		const before = original[field];
		const after = edited[field];
		if (Array.isArray(before) && Array.isArray(after)) {
			if (JSON.stringify(before) !== JSON.stringify(after)) patch[field] = after;
		} else if (before !== after) {
			patch[field] = after;
		}
	}
	return patch;
}

/** Returns only the ARTIFACT_OBJECT_EDITABLE_FIELDS that differ between original and edited. */
export function buildArtifactObjectPatch(
	original: ArtifactObjectDetail,
	edited: ArtifactObjectDetail
): Record<string, unknown> {
	return diffFields(original, edited, ARTIFACT_OBJECT_EDITABLE_FIELDS);
}

/** Returns only the OBJECT_NODE_EDITABLE_FIELDS that differ between original and edited. */
export function buildObjectNodePatch(
	original: ObjectNodeCandidate,
	edited: ObjectNodeCandidate
): Record<string, unknown> {
	return diffFields(original, edited, OBJECT_NODE_EDITABLE_FIELDS);
}

/** Index math for Prev/Next within the left-panel id list. Returns null past either end. */
export function neighborAmbiguousId(ids: number[], currentId: number, direction: 1 | -1): number | null {
	const index = ids.indexOf(currentId);
	if (index === -1) return null;
	const nextIndex = index + direction;
	if (nextIndex < 0 || nextIndex >= ids.length) return null;
	return ids[nextIndex];
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/cding/Workspace/ChenWeb/web && bun test src/lib/components/home3/resolve-ambiguous-objects-client.test.ts`
Expected: PASS (9 tests).

- [ ] **Step 5: Commit**

```bash
cd /Users/cding/Workspace/ChenWeb
jj describe -m "Add resolve-ambiguous-objects-client.ts fetch wrappers and diff/nav helpers"
jj new
```

---

### Task 7: Nav wiring + view skeleton (left panel list)

**Files:**
- Modify: `web/src/lib/components/home3/nav-rail.svelte`
- Modify: `web/src/lib/components/home3/content-panel.svelte`
- Create: `web/src/lib/components/home3/resolve-ambiguous-objects-view.svelte`

**Interfaces:**
- Consumes: `listAmbiguousObjects`, `getAmbiguousObjectDetail` (Task 6); `darkMode` prop convention shared by every sibling view (`db-consistency-view.svelte`, `db-maint-log-view.svelte`).
- Produces (used by Task 8, 9): the mounted `<ResolveAmbiguousObjectsView {darkMode} />` component with its left-panel list rendered and selectable.

- [ ] **Step 1: Add the nav entry**

In `web/src/lib/components/home3/nav-rail.svelte`, change the `sysadmin-db` children (around line 196-201):

```js
				{
					id: 'sysadmin-db', label: 'Database Maintenance',
					children: [
						{ id: 'sysadmin-db-consistency', label: 'Consistency Check' },
						{ id: 'sysadmin-db-maint-log', label: 'Maintenance Log' },
						{ id: 'sysadmin-db-resolve-ambiguous', label: 'Resolve Ambiguous Objects' }
					]
				}
```

- [ ] **Step 2: Wire the dispatch in content-panel.svelte**

Add the import near the other `DbConsistencyView`/`DbMaintLogView` imports (around line 22-23):

```svelte
	import DbConsistencyView from '$lib/components/home3/db-consistency-view.svelte';
	import DbMaintLogView from '$lib/components/home3/db-maint-log-view.svelte';
	import ResolveAmbiguousObjectsView from '$lib/components/home3/resolve-ambiguous-objects-view.svelte';
```

Add the dispatch branch right after the existing `sysadmin-db-maint-log` branch (around line 204-205):

```svelte
		{:else if activeMenu?.childId === 'sysadmin-db-maint-log'}
			<DbMaintLogView {darkMode} />
		{:else if activeMenu?.childId === 'sysadmin-db-resolve-ambiguous'}
			<ResolveAmbiguousObjectsView {darkMode} />
```

- [ ] **Step 3: Write the view skeleton — left panel only**

Create `web/src/lib/components/home3/resolve-ambiguous-objects-view.svelte`:

```svelte
<script lang="ts">
	import { onMount } from 'svelte';
	import {
		listAmbiguousObjects,
		getAmbiguousObjectDetail,
		type AmbiguousObjectSummary,
		type ArtifactObjectDetail,
		type ObjectNodeCandidate
	} from './resolve-ambiguous-objects-client.js';

	let { darkMode = true }: { darkMode?: boolean } = $props();

	// --- Design tokens (matches db-consistency-view.svelte / db-maint-log-view.svelte) ---
	let pageBg        = $derived(darkMode ? '#171B26' : '#F2F4F7');
	let cardBg        = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let borderColor   = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent        = $derived(darkMode ? '#818CF8' : '#6366F1');
	let accentTint    = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)');
	let textPrimary   = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted     = $derived(darkMode ? '#64748B' : '#9CA3AF');

	// --- Left panel state ---
	let rows        = $state<AmbiguousObjectSummary[]>([]);
	let listLoading = $state(false);
	let listError   = $state('');

	// --- Right panel state ---
	let selectedId     = $state<number | null>(null);
	let detailLoading  = $state(false);
	let detailError    = $state('');
	let snapshotObject = $state<ArtifactObjectDetail | null>(null);
	let currentObject  = $state<ArtifactObjectDetail | null>(null);
	let snapshotNodes  = $state<ObjectNodeCandidate[]>([]);
	let currentNodes   = $state<ObjectNodeCandidate[]>([]);

	async function loadList() {
		listLoading = true;
		listError = '';
		try {
			const res = await listAmbiguousObjects();
			rows = res.rows;
			if (rows.length > 0 && selectedId === null) {
				await selectRow(rows[0].id);
			}
		} catch (e) {
			listError = e instanceof Error ? e.message : String(e);
		} finally {
			listLoading = false;
		}
	}

	async function selectRow(id: number) {
		selectedId = id;
		detailLoading = true;
		detailError = '';
		try {
			const detail = await getAmbiguousObjectDetail(id);
			snapshotObject = detail.artifact_object;
			currentObject = { ...detail.artifact_object };
			snapshotNodes = detail.candidates;
			currentNodes = detail.candidates.map((c) => ({ ...c }));
		} catch (e) {
			detailError = e instanceof Error ? e.message : String(e);
		} finally {
			detailLoading = false;
		}
	}

	onMount(() => {
		loadList();
	});
</script>

<div class="flex" style="height:100%; min-height:100%; background:{pageBg};">
	<!-- Left panel -->
	<div
		class="flex-shrink-0 overflow-y-auto"
		style="width:320px; border-right:1px solid {borderColor};"
	>
		<div class="p-4" style="border-bottom:1px solid {borderColor};">
			<h1 style="font-size:16px; font-weight:600; color:{textPrimary}; margin-bottom:2px;">
				Resolve Ambiguous Objects
			</h1>
			<p style="font-size:12px; color:{textSecondary};">
				{rows.length} row{rows.length === 1 ? '' : 's'} at reconcile_status = ambiguous
			</p>
		</div>

		{#if listLoading}
			<div class="p-4" style="color:{textMuted}; font-size:13px;">Loading…</div>
		{:else if listError}
			<div class="p-4" style="color:#F87171; font-size:13px;">Error: {listError}</div>
		{:else if rows.length === 0}
			<div class="p-4" style="color:{textMuted}; font-size:13px;">
				No ambiguous objects — the queue is empty.
			</div>
		{:else}
			{#each rows as row (row.id)}
				<button
					type="button"
					onclick={() => selectRow(row.id)}
					class="w-full text-left p-3 cursor-pointer"
					style="
						border-bottom:1px solid {borderColor};
						background:{selectedId === row.id ? accentTint : 'transparent'};
					"
				>
					<div style="font-size:13px; font-weight:500; color:{selectedId === row.id ? accent : textPrimary};">
						{row.object_name}{row.object_name_en ? ` (${row.object_name_en})` : ''}
					</div>
					<div style="font-size:11px; color:{textMuted}; margin-top:2px;">
						{row.artifact_type} · confidence {row.confidence.toFixed(2)}
					</div>
				</button>
			{/each}
		{/if}
	</div>

	<!-- Right panel -->
	<div class="flex-1 overflow-y-auto p-6">
		{#if detailLoading}
			<div style="color:{textMuted}; font-size:13px;">Loading…</div>
		{:else if detailError}
			<div style="color:#F87171; font-size:13px;">Error: {detailError}</div>
		{:else if !currentObject}
			<div style="color:{textMuted}; font-size:13px;">Select a record on the left to resolve it.</div>
		{:else}
			<div style="color:{textPrimary}; font-size:13px;">
				Loaded: {currentObject.object_name} ({currentObject.id})
			</div>
		{/if}
	</div>
</div>
```

- [ ] **Step 2: Type-check**

Run: `cd /Users/cding/Workspace/ChenWeb/web && bun run check`
Expected: no new type errors from the files touched in this task.

- [ ] **Step 3: Manual smoke check**

Start the dev server and open the page (see Task 10 for the full manual verification pass — a quick check here just confirms the nav entry and left-panel list render): `cd /Users/cding/Workspace/ChenWeb/web && bun run dev` then visit `http://127.0.0.1:5173/home3`, open System Admin → Database Maintenance → Resolve Ambiguous Objects, confirm the left panel lists rows (or the empty-queue message) without console errors. Stop the dev server after checking.

- [ ] **Step 4: Commit**

```bash
cd /Users/cding/Workspace/ChenWeb
jj describe -m "Wire Resolve Ambiguous Objects into System Admin nav with a left-panel list skeleton"
jj new
```

---

### Task 8: Right panel — Artifact Object block + Object Nodes block

**Files:**
- Modify: `web/src/lib/components/home3/resolve-ambiguous-objects-view.svelte`

**Interfaces:**
- Consumes: `RECONCILE_STATUS_OPTIONS` (Task 6); `currentObject`, `currentNodes` state (Task 7).
- Produces (used by Task 9): fully editable right-panel form; `useCandidate(objectId)` function that Task 9's dirty-tracking depends on.

- [ ] **Step 1: Replace the right-panel placeholder with the two editable blocks**

In `resolve-ambiguous-objects-view.svelte`, add to the script block (after the existing state/functions, before `onMount`):

```ts
	function useCandidate(objectId: string) {
		if (!currentObject) return;
		currentObject.object_id = objectId;
	}

	function aliasesText(values: string[]): string {
		return values.join(', ');
	}

	function parseAliasesText(text: string): string[] {
		return text
			.split(',')
			.map((s) => s.trim())
			.filter(Boolean);
	}
```

Update the import line to also bring in `RECONCILE_STATUS_OPTIONS`:

```ts
	import {
		listAmbiguousObjects,
		getAmbiguousObjectDetail,
		RECONCILE_STATUS_OPTIONS,
		type AmbiguousObjectSummary,
		type ArtifactObjectDetail,
		type ObjectNodeCandidate
	} from './resolve-ambiguous-objects-client.js';
```

Replace the right panel's `{:else}` branch (the `Loaded: {currentObject.object_name}...` placeholder) with:

```svelte
		{:else}
			<!-- Artifact Object block -->
			<div class="rounded-xl p-5 mb-5" style="background:{cardBg}; border:1px solid {borderColor};">
				<div class="flex items-center justify-between mb-4">
					<h2 style="font-size:14px; font-weight:600; color:{textPrimary};">Artifact Object</h2>
					<span style="font-size:11px; color:{textMuted};">
						id {currentObject.id} · {currentObject.artifact_type} · {currentObject.artifact_id}
					</span>
				</div>
				<div class="grid grid-cols-2 gap-3">
					<label class="flex flex-col gap-1">
						<span style="font-size:11px; color:{textMuted};">Object Name</span>
						<input bind:value={currentObject.object_name} style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;" />
					</label>
					<label class="flex flex-col gap-1">
						<span style="font-size:11px; color:{textMuted};">Object Name (EN)</span>
						<input bind:value={currentObject.object_name_en} style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;" />
					</label>
					<label class="flex flex-col gap-1">
						<span style="font-size:11px; color:{textMuted};">Object Name (ZH)</span>
						<input bind:value={currentObject.object_name_zh} style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;" />
					</label>
					<label class="flex flex-col gap-1">
						<span style="font-size:11px; color:{textMuted};">Language</span>
						<input bind:value={currentObject.language} style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;" />
					</label>
					<label class="flex flex-col gap-1">
						<span style="font-size:11px; color:{textMuted};">Object Type</span>
						<input bind:value={currentObject.object_type} style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;" />
					</label>
					<label class="flex flex-col gap-1">
						<span style="font-size:11px; color:{textMuted};">Object Role</span>
						<input bind:value={currentObject.object_role} style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;" />
					</label>
					<label class="flex flex-col gap-1 col-span-2">
						<span style="font-size:11px; color:{textMuted};">Aliases (comma-separated)</span>
						<input
							value={aliasesText(currentObject.aliases)}
							oninput={(e) => { if (currentObject) currentObject.aliases = parseAliasesText((e.currentTarget as HTMLInputElement).value); }}
							style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;"
						/>
					</label>
					<label class="flex flex-col gap-1 col-span-2">
						<span style="font-size:11px; color:{textMuted};">Acronyms (comma-separated)</span>
						<input
							value={aliasesText(currentObject.acronyms)}
							oninput={(e) => { if (currentObject) currentObject.acronyms = parseAliasesText((e.currentTarget as HTMLInputElement).value); }}
							style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;"
						/>
					</label>
					<label class="flex flex-col gap-1 col-span-2">
						<span style="font-size:11px; color:{textMuted};">Description</span>
						<textarea bind:value={currentObject.description} rows="2" style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;"></textarea>
					</label>
					<label class="flex flex-col gap-1 col-span-2">
						<span style="font-size:11px; color:{textMuted};">Evidence Quote</span>
						<textarea bind:value={currentObject.evidence_quote} rows="2" style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;"></textarea>
					</label>
					<label class="flex flex-col gap-1">
						<span style="font-size:11px; color:{textMuted};">Object ID</span>
						<input bind:value={currentObject.object_id} placeholder="(unresolved)" style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;" />
					</label>
					<label class="flex flex-col gap-1">
						<span style="font-size:11px; color:{textMuted};">Reconcile Status</span>
						<select bind:value={currentObject.reconcile_status} style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;">
							{#each RECONCILE_STATUS_OPTIONS as opt}
								<option value={opt}>{opt}</option>
							{/each}
						</select>
					</label>
					<label class="flex flex-col gap-1">
						<span style="font-size:11px; color:{textMuted};">Reconcile Confidence</span>
						<input type="number" min="0" max="1" step="0.01" bind:value={currentObject.reconcile_confidence} style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;" />
					</label>
				</div>
			</div>

			<!-- Related Object Nodes block -->
			<div class="rounded-xl p-5" style="background:{cardBg}; border:1px solid {borderColor};">
				<h2 style="font-size:14px; font-weight:600; color:{textPrimary}; margin-bottom:12px;">
					Related Object Nodes
				</h2>
				{#if currentNodes.length === 0}
					<div style="font-size:13px; color:{textMuted};">No candidate object nodes found for this artifact object.</div>
				{/if}
				{#each currentNodes as node, i (node.object_id)}
					<div class="rounded-lg p-4 mb-3" style="border:1px solid {borderColor}; background:{pageBg};">
						<div class="flex items-center justify-between mb-3">
							<div class="flex items-center gap-2">
								<span style="font-size:12px; font-family:monospace; color:{textSecondary};">{node.object_id}</span>
								{#if node.recommended}
									<span style="font-size:10px; font-weight:600; padding:2px 6px; border-radius:4px; background:{accentTint}; color:{accent};">Recommended</span>
								{/if}
							</div>
							<div class="flex items-center gap-3">
								<span style="font-size:11px; color:{textMuted};">score {node.score.toFixed(2)} · {node.method}</span>
								<button
									type="button"
									onclick={() => useCandidate(node.object_id)}
									style="font-size:11px; font-weight:500; padding:4px 10px; border-radius:6px; border:none; cursor:pointer; background:{accent}; color:white;"
								>
									Use this
								</button>
							</div>
						</div>
						<div class="grid grid-cols-2 gap-3">
							<label class="flex flex-col gap-1">
								<span style="font-size:11px; color:{textMuted};">Canonical Name</span>
								<input bind:value={currentNodes[i].canonical_name} style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;" />
							</label>
							<label class="flex flex-col gap-1">
								<span style="font-size:11px; color:{textMuted};">Object Type</span>
								<input bind:value={currentNodes[i].object_type} style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;" />
							</label>
							<label class="flex flex-col gap-1">
								<span style="font-size:11px; color:{textMuted};">Canonical Name (EN)</span>
								<input bind:value={currentNodes[i].canonical_name_en} style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;" />
							</label>
							<label class="flex flex-col gap-1">
								<span style="font-size:11px; color:{textMuted};">Canonical Name (ZH)</span>
								<input bind:value={currentNodes[i].canonical_name_zh} style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;" />
							</label>
							<label class="flex flex-col gap-1">
								<span style="font-size:11px; color:{textMuted};">Primary Language</span>
								<input bind:value={currentNodes[i].primary_language} style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;" />
							</label>
							<label class="flex flex-col gap-1">
								<span style="font-size:11px; color:{textMuted};">Aliases (comma-separated)</span>
								<input
									value={aliasesText(node.aliases)}
									oninput={(e) => { currentNodes[i].aliases = parseAliasesText((e.currentTarget as HTMLInputElement).value); }}
									style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;"
								/>
							</label>
							<label class="flex flex-col gap-1">
								<span style="font-size:11px; color:{textMuted};">Acronyms (comma-separated)</span>
								<input
									value={aliasesText(node.acronyms)}
									oninput={(e) => { currentNodes[i].acronyms = parseAliasesText((e.currentTarget as HTMLInputElement).value); }}
									style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;"
								/>
							</label>
							<label class="flex flex-col gap-1 col-span-2">
								<span style="font-size:11px; color:{textMuted};">Description</span>
								<textarea bind:value={currentNodes[i].description} rows="2" style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px;"></textarea>
							</label>
						</div>
					</div>
				{/each}
			</div>
		{/if}
```

- [ ] **Step 2: Type-check**

Run: `cd /Users/cding/Workspace/ChenWeb/web && bun run check`
Expected: no new type errors.

- [ ] **Step 3: Manual smoke check**

`cd /Users/cding/Workspace/ChenWeb/web && bun run dev`, open the page, select a row, confirm both blocks render with editable inputs, "Use this" copies an object_id into the Object ID field, edits to any field are reflected live. Stop the dev server after checking.

- [ ] **Step 4: Commit**

```bash
cd /Users/cding/Workspace/ChenWeb
jj describe -m "Add editable Artifact Object and Related Object Nodes blocks to the resolution view"
jj new
```

---

### Task 9: Dirty tracking, Save/Cancel/Prev/Next/Help

**Files:**
- Modify: `web/src/lib/components/home3/resolve-ambiguous-objects-view.svelte`

**Interfaces:**
- Consumes: `updateArtifactObject`, `updateObjectNode`, `buildArtifactObjectPatch`, `buildObjectNodePatch`, `neighborAmbiguousId` (Task 6).
- Produces: complete, spec-compliant page behavior.

- [ ] **Step 1: Add dirty tracking, navigation, save, cancel, and help state/functions**

Update the imports:

```ts
	import {
		listAmbiguousObjects,
		getAmbiguousObjectDetail,
		updateArtifactObject,
		updateObjectNode,
		buildArtifactObjectPatch,
		buildObjectNodePatch,
		neighborAmbiguousId,
		RECONCILE_STATUS_OPTIONS,
		type AmbiguousObjectSummary,
		type ArtifactObjectDetail,
		type ObjectNodeCandidate
	} from './resolve-ambiguous-objects-client.js';
```

Add after the existing state declarations (after `let currentNodes = $state<ObjectNodeCandidate[]>([]);`):

```ts
	let saving      = $state(false);
	let saveError   = $state('');
	let helpOpen    = $state(false);

	let navConfirm      = $state<{ kind: 'prev' | 'next' | 'switch' } | null>(null);
	let pendingSelectId = $state<number | null>(null);
	let cancelConfirm   = $state(false);

	let isDirty = $derived.by(() => {
		if (!snapshotObject || !currentObject) return false;
		if (JSON.stringify(snapshotObject) !== JSON.stringify(currentObject)) return true;
		return JSON.stringify(snapshotNodes) !== JSON.stringify(currentNodes);
	});

	// Derived (not inlined in the template) so `selectedId`'s `number | null`
	// type is narrowed once here, rather than needing a `number` at every
	// neighborAmbiguousId call site in the markup.
	let prevId = $derived.by(() =>
		selectedId === null ? null : neighborAmbiguousId(rows.map((r) => r.id), selectedId, -1)
	);
	let nextId = $derived.by(() =>
		selectedId === null ? null : neighborAmbiguousId(rows.map((r) => r.id), selectedId, 1)
	);

	function requestNav(kind: 'prev' | 'next' | 'switch', id: number) {
		if (isDirty) {
			navConfirm = { kind };
			pendingSelectId = id;
			return;
		}
		selectRow(id);
	}

	function goPrev() {
		if (prevId !== null) requestNav('prev', prevId);
	}

	function goNext() {
		if (nextId !== null) requestNav('next', nextId);
	}

	function clickCard(id: number) {
		if (id === selectedId) return;
		requestNav('switch', id);
	}

	async function confirmNavSave() {
		const target = pendingSelectId;
		navConfirm = null;
		pendingSelectId = null;
		await doSave();
		if (target !== null) await selectRow(target);
	}

	function confirmNavDiscard() {
		const target = pendingSelectId;
		navConfirm = null;
		pendingSelectId = null;
		if (target !== null) selectRow(target);
	}

	function stayOnNav() {
		navConfirm = null;
		pendingSelectId = null;
	}

	function requestCancel() {
		if (!isDirty) return;
		cancelConfirm = true;
	}

	function confirmCancel() {
		if (snapshotObject) currentObject = { ...snapshotObject };
		currentNodes = snapshotNodes.map((c) => ({ ...c }));
		cancelConfirm = false;
	}

	function dismissCancel() {
		cancelConfirm = false;
	}

	async function doSave() {
		if (!currentObject || !snapshotObject || selectedId === null) return;
		saving = true;
		saveError = '';
		try {
			const objectPatch = buildArtifactObjectPatch(snapshotObject, currentObject);
			if (Object.keys(objectPatch).length > 0) {
				await updateArtifactObject(selectedId, objectPatch);
			}
			for (let i = 0; i < currentNodes.length; i++) {
				const nodePatch = buildObjectNodePatch(snapshotNodes[i], currentNodes[i]);
				if (Object.keys(nodePatch).length > 0) {
					await updateObjectNode(currentNodes[i].object_id, nodePatch);
				}
			}
			const resolved = currentObject.reconcile_status !== 'ambiguous';
			snapshotObject = { ...currentObject };
			snapshotNodes = currentNodes.map((c) => ({ ...c }));
			if (resolved) {
				const resolvedId = selectedId;
				rows = rows.filter((r) => r.id !== resolvedId);
				if (rows.length > 0) {
					await selectRow(rows[0].id);
				} else {
					selectedId = null;
					currentObject = null;
					snapshotObject = null;
					currentNodes = [];
					snapshotNodes = [];
				}
			}
		} catch (e) {
			saveError = e instanceof Error ? e.message : String(e);
		} finally {
			saving = false;
		}
	}
```

Update `selectRow` to clear `saveError` on entry (change its first line from `detailLoading = true;` to also reset `saveError`):

```ts
	async function selectRow(id: number) {
		selectedId = id;
		detailLoading = true;
		detailError = '';
		saveError = '';
		try {
```

Update the left-panel card button's `onclick` to go through `clickCard` instead of `selectRow` directly:

```svelte
					onclick={() => clickCard(row.id)}
```

- [ ] **Step 2: Add the button row and modals to the markup**

Add a button row above the right panel's `{#if detailLoading}` block (i.e. right after the opening `<div class="flex-1 overflow-y-auto p-6">` tag):

```svelte
		<div class="flex items-center gap-2 mb-4">
			<button
				type="button"
				onclick={goPrev}
				disabled={prevId === null}
				style="font-size:12px; font-weight:500; padding:6px 12px; border-radius:6px; border:1px solid {borderColor}; cursor:pointer; background:{cardBg}; color:{textPrimary}; opacity:{prevId === null ? 0.5 : 1};"
			>
				Prev
			</button>
			<button
				type="button"
				onclick={goNext}
				disabled={nextId === null}
				style="font-size:12px; font-weight:500; padding:6px 12px; border-radius:6px; border:1px solid {borderColor}; cursor:pointer; background:{cardBg}; color:{textPrimary}; opacity:{nextId === null ? 0.5 : 1};"
			>
				Next
			</button>
			<button
				type="button"
				onclick={requestCancel}
				disabled={!isDirty}
				style="font-size:12px; font-weight:500; padding:6px 12px; border-radius:6px; border:1px solid {borderColor}; cursor:pointer; background:{cardBg}; color:{textPrimary}; opacity:{isDirty ? 1 : 0.5};"
			>
				Cancel
			</button>
			<button
				type="button"
				onclick={doSave}
				disabled={!isDirty || saving}
				style="font-size:12px; font-weight:600; padding:6px 14px; border-radius:6px; border:none; cursor:pointer; background:{accent}; color:white; opacity:{!isDirty || saving ? 0.5 : 1};"
			>
				{saving ? 'Saving…' : 'Save'}
			</button>
			<button
				type="button"
				onclick={() => (helpOpen = true)}
				style="font-size:12px; font-weight:500; padding:6px 12px; border-radius:6px; border:1px solid {borderColor}; cursor:pointer; background:{cardBg}; color:{textSecondary}; margin-left:auto;"
			>
				Help
			</button>
		</div>
		{#if saveError}
			<div class="mb-4" style="font-size:12px; color:#F87171;">Save failed: {saveError}</div>
		{/if}
```

Add the three modals at the end of the file, right before the closing `</div>` of the outermost `<div class="flex" ...>` wrapper:

```svelte
	{#if navConfirm}
		<div
			class="overlay"
			role="presentation"
			tabindex="-1"
			onclick={stayOnNav}
			onkeydown={(e) => { if (e.key === 'Escape') stayOnNav(); }}
		>
			<div class="modal" role="dialog" aria-modal="true" tabindex="0" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()}>
				<p style="font-size:13px; color:{textPrimary}; margin-bottom:16px;">
					{navConfirm.kind === 'prev'
						? 'Save changes before moving to the previous record?'
						: navConfirm.kind === 'next'
							? 'Save changes before moving to the next record?'
							: 'Save changes before switching records?'}
				</p>
				<div class="flex justify-end gap-2">
					<button type="button" onclick={stayOnNav} style="font-size:12px; padding:6px 12px; border-radius:6px; border:1px solid {borderColor}; cursor:pointer; background:{cardBg}; color:{textPrimary};">Stay</button>
					<button type="button" onclick={confirmNavDiscard} style="font-size:12px; padding:6px 12px; border-radius:6px; border:1px solid {borderColor}; cursor:pointer; background:{cardBg}; color:{textPrimary};">Discard &amp; Continue</button>
					<button type="button" onclick={confirmNavSave} style="font-size:12px; font-weight:600; padding:6px 12px; border-radius:6px; border:none; cursor:pointer; background:{accent}; color:white;">Save &amp; Continue</button>
				</div>
			</div>
		</div>
	{/if}

	{#if cancelConfirm}
		<div
			class="overlay"
			role="presentation"
			tabindex="-1"
			onclick={dismissCancel}
			onkeydown={(e) => { if (e.key === 'Escape') dismissCancel(); }}
		>
			<div class="modal" role="dialog" aria-modal="true" tabindex="0" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()}>
				<p style="font-size:13px; color:{textPrimary}; margin-bottom:16px;">Discard your edits to this record?</p>
				<div class="flex justify-end gap-2">
					<button type="button" onclick={dismissCancel} style="font-size:12px; padding:6px 12px; border-radius:6px; border:1px solid {borderColor}; cursor:pointer; background:{cardBg}; color:{textPrimary};">Keep Editing</button>
					<button type="button" onclick={confirmCancel} style="font-size:12px; font-weight:600; padding:6px 12px; border-radius:6px; border:none; cursor:pointer; background:#DC2626; color:white;">Discard</button>
				</div>
			</div>
		</div>
	{/if}

	{#if helpOpen}
		<div
			class="overlay"
			role="presentation"
			tabindex="-1"
			onclick={() => (helpOpen = false)}
			onkeydown={(e) => { if (e.key === 'Escape') helpOpen = false; }}
		>
			<div class="modal" role="dialog" aria-modal="true" tabindex="0" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()} style="width:min(560px, 100%);">
				<h3 style="font-size:14px; font-weight:600; color:{textPrimary}; margin-bottom:10px;">About This Page</h3>
				<div style="font-size:12px; color:{textSecondary}; line-height:1.7;">
					<p style="margin-bottom:10px;">
						A row is <strong>ambiguous</strong> when reconciliation found two or more equally-scored
						candidate object nodes and could not pick one automatically. This page lets you review
						the artifact object and its candidates side by side and resolve the tie by hand.
					</p>
					<p style="margin-bottom:10px;">
						The <strong>Recommended</strong> badge marks the same deterministic tie-break pick used by
						the automated backfill (most shared normalized names, falling back to the
						lexicographically smallest object_id). Click <strong>Use this</strong> on any candidate to
						copy its object_id into the Object ID field above.
					</p>
					<p style="margin-bottom:10px;">
						<strong>reconcile_status</strong> values: <code>pending</code> (not yet attempted),
						<code>ambiguous</code> (still tied), <code>ambiguous_resolved</code> (tie broken by a
						human or the automated tie-break), <code>matched</code> (confident automatic match),
						<code>new</code> (a new object node was created), <code>rejected</code> (no valid object
						— leave object_id empty).
					</p>
					<p>
						<strong>Save</strong> writes your edits to both the artifact object and any edited
						candidate nodes. Once you set an object_id and a non-<code>ambiguous</code> status, the
						row leaves this queue.
					</p>
				</div>
				<div class="flex justify-end mt-4">
					<button type="button" onclick={() => (helpOpen = false)} style="font-size:12px; font-weight:600; padding:6px 14px; border-radius:6px; border:none; cursor:pointer; background:{accent}; color:white;">Close</button>
				</div>
			</div>
		</div>
	{/if}
```

Add the modal CSS (mirroring `summary-node-dialog.svelte`'s `.overlay`/`.dialog` pattern) in a `<style>` block at the end of the file:

```svelte
<style>
	.overlay {
		position: fixed;
		inset: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 1.5rem;
		background: rgba(2, 6, 23, 0.62);
		z-index: 30;
	}

	.modal {
		width: min(420px, 100%);
		border-radius: 12px;
		border: 1px solid rgba(148, 163, 184, 0.16);
		background: #1f2333;
		padding: 1.25rem;
	}
</style>
```

- [ ] **Step 3: Type-check**

Run: `cd /Users/cding/Workspace/ChenWeb/web && bun run check`
Expected: no new type errors.

- [ ] **Step 4: Manual smoke check**

`cd /Users/cding/Workspace/ChenWeb/web && bun run dev`, then walk through:
1. Select a row, edit a field, click **Prev**/**Next** — confirm the 3-button modal appears with the right wording and each button behaves as specified.
2. Edit a field, click **Cancel** — confirm the discard modal appears and Discard reverts the form.
3. Edit a field, click **Save** — confirm the PATCH calls fire (check Network tab), and if `reconcile_status` was changed away from `ambiguous`, confirm the row disappears from the left panel and the next row auto-selects.
4. Click **Help** — confirm the modal opens/closes without a network call.
Stop the dev server after checking.

- [ ] **Step 5: Commit**

```bash
cd /Users/cding/Workspace/ChenWeb
jj describe -m "Add dirty tracking, Save/Cancel/Prev/Next/Help behavior to the resolution view"
jj new
```

---

### Task 10: Full verification pass

**Files:** none (verification only).

- [ ] **Step 1: Backend — full build, vet, and test suite**

Run:
```bash
cd /Users/cding/Workspace/ChenWeb
go build ./...
go vet ./server/api/...
go test ./server/api/doc-processing/... ./server/api/kbhandler/...
```
Expected: all pass, no build/vet errors.

- [ ] **Step 2: Frontend — full client test suite and type-check**

Run:
```bash
cd /Users/cding/Workspace/ChenWeb/web
bun test src/lib/components/home3/resolve-ambiguous-objects-client.test.ts
bun run check
```
Expected: all pass, no type errors.

- [ ] **Step 3: Manual end-to-end walkthrough on the running app**

Start both backend and frontend per the project's normal dev workflow (`tax/CLAUDE.md`-equivalent for ChenWeb, or `cd ChenWeb && mise dev` if configured), open `http://127.0.0.1:5173/home3`, navigate to System Admin → Database Maintenance → Resolve Ambiguous Objects, and confirm against a real (or seeded) ambiguous row:
- left panel lists ambiguous rows;
- selecting a row populates both blocks;
- "Use this" fills the Object ID field;
- Prev/Next/Cancel/Save/Help all behave as specified end-to-end, including a row leaving the queue after a resolving Save.

- [ ] **Step 4: Final commit (if any cleanup was needed)**

If Steps 1–3 required fixes, commit them:
```bash
cd /Users/cding/Workspace/ChenWeb
jj describe -m "Fix issues found during Resolve Ambiguous Objects verification pass"
jj new
```
If no fixes were needed, skip this step — the feature is complete as of Task 9's commit.
