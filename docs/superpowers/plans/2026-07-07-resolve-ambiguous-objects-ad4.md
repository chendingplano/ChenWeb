# Resolve Ambiguous Objects — AD4 Refinements Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Apply the four AD4 refinements to the existing Resolve Ambiguous Objects admin page: a two-column right panel (quick-reference candidate table + detailed candidate cards), auto-setting `reconcile_status = 'matched'` from "Use this", red dirty-field highlighting, and an audit trail (`kb.object_audit_log`) for every write to `kb.artifact_objects` / `kb.object_nodes` made through this page.

**Architecture:** Backend adds one new table + a small best-effort `logObjectAudit` helper wired into the two existing PATCH handlers (`UpdateArtifactObject`, `UpdateObjectNode`) right after each confirms a row was actually updated. Frontend adds one pure `fieldDirty` comparator to the existing client module (unit-testable, mirroring `buildArtifactObjectPatch`) and restructures the Svelte view's right panel into a CSS grid with the candidate list rendered twice — a compact picker table and the existing detailed editable cards — sharing the same `currentNodes` state so they can never disagree.

**Tech Stack:** Go + Echo + `database/sql` (Postgres) on the backend; Svelte 5 (runes) + TypeScript on the frontend; `go test` with `sqlmock`; `bun test` with `node:test`/`node:assert`.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-07-07-resolve-ambiguous-objects-ad4-design.md` — every requirement below traces back to this file.
- `table_name` and `action` columns on `kb.object_audit_log` are `TEXT` + `CHECK` constraints, not a Postgres `ENUM` and not integer codes.
- `action` values: `resolve_object_id` (artifact-object PATCH sets a non-empty `object_id`) or `edit_fields` (everything else, on either table).
- Audit logging is best-effort: an insert failure logs `Warn` via the existing `ApiTypes.JimoLogger` and never fails the PATCH response, matching the `docactivity.Log` precedent in `server/api/docactivity/activity.go`.
- Actor resolution reuses the existing unexported `structureActor(rc ApiTypes.RequestContext) string` helper already defined in `server/api/kbhandler/doc_structure_handler.go` (same package) — do not duplicate it.
- `changes` column stores the raw client-submitted PATCH payload (`map[string]json.RawMessage`, marshaled), not before/after values and not the internal `ext_info.reconcile_method` stamp.
- "Use this" pre-fills `object_id` and `reconcile_status = 'matched'` into component state only — still just a form edit, not a save; the admin can still change the dropdown before Save.
- Top Region (new compact candidate table) is the only remaining "Use this" control; the existing per-card button in the detailed candidate cards (Bottom Region) is removed.
- Dirty-field highlighting reuses the existing `#F87171` red token already used for `saveError`/`listError`/`detailError` text in this file — no new color token.
- Follow existing conventions exactly: no new shared components, no new CSS framework usage beyond what's already in the file (plain inline `style="..."` + Tailwind utility classes, hand-rolled `<table>` for the new Top Region — matches this file's existing hand-rolled-markup style).
- Go module: `github.com/chendingplano/deepdoc`. kbhandler package: `github.com/chendingplano/deepdoc/server/api/kbhandler` (implicit — files live in that dir).
- Go tests use `github.com/DATA-DOG/go-sqlmock`; run with `go test ./server/api/kbhandler/...`.
- Frontend tests use `node:test` + `node:assert/strict`, run with `bun test <path>` from `web/` (NOT `node --test` — this repo has no compiled-JS step; `bun test` resolves the `.js`-suffixed imports back to sibling `.ts` files).
- Frontend type-check: `bun run check` (runs `svelte-kit sync && svelte-check`) from `web/`.
- Commit with `jj describe -m "..."` then `jj new` after each task (per Workspace `CLAUDE.md`: "Always use jj"). This repo is jj-colocated (`.jj` alongside `.git`).
- All file paths below are relative to the `ChenWeb/` repo root.

---

### Task 1: Migration — `kb.object_audit_log`

**Files:**
- Create: `project_migrations/20260707000002_create_kb_object_audit_log.sql`

**Interfaces:**
- Consumes: nothing (new table).
- Produces (used by Tasks 2-4): table `kb.object_audit_log` with columns `id, table_name, row_key, action, changes, actor, create_time`.

- [ ] **Step 1: Write the migration**

```sql
-- +goose Up
CREATE TABLE IF NOT EXISTS kb.object_audit_log (
    id          BIGSERIAL    PRIMARY KEY,
    table_name  TEXT         NOT NULL,
    row_key     TEXT         NOT NULL,
    action      TEXT         NOT NULL,
    changes     JSONB        NOT NULL,
    actor       TEXT,
    create_time TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_kb_object_audit_log_table_name
        CHECK (table_name IN ('kb.artifact_objects', 'kb.object_nodes')),
    CONSTRAINT chk_kb_object_audit_log_action
        CHECK (action IN ('resolve_object_id', 'edit_fields'))
);

CREATE INDEX IF NOT EXISTS idx_object_audit_log_row  ON kb.object_audit_log (table_name, row_key);
CREATE INDEX IF NOT EXISTS idx_object_audit_log_time ON kb.object_audit_log (create_time);

-- +goose Down
DROP TABLE IF EXISTS kb.object_audit_log;
```

- [ ] **Step 2: Verify the migration directory is picked up with no code changes needed**

Migrations in this repo are discovered by directory scan (`server/cmd/config/config.go`, `resolveMigrationDir`), not an explicit registration list — dropping the file in `project_migrations/` is sufficient. Confirm no other file needs updating:

Run: `grep -rn "20260707000001" server/ --include="*.go"`
Expected: no output (the sibling migration this one follows is never referenced by filename in Go code either — confirms no registration list exists to update).

- [ ] **Step 3: Sanity build**

Run: `go build ./...`
Expected: exits 0 (migration file is inert to the Go build; this just confirms nothing else in the tree is broken before starting).

Note: this migration is not applied to any live database by this plan — per this repo's convention (`CLAUDE.md`: "Upon system start, make sure it creates all the tables and runs the database migration"), it applies automatically the next time the server starts against a real Postgres instance. This mirrors the precedent already noted in ADR 2026070701 DR5/DR6 for the prior migration in this same feature.

- [ ] **Step 4: Commit**

```bash
jj describe -m "Add kb.object_audit_log migration for AD4 audit logging"
jj new
```

---

### Task 2: Backend — `logObjectAudit` helper

**Files:**
- Create: `server/api/kbhandler/object_audit_log.go`
- Test: `server/api/kbhandler/object_audit_log_test.go`

**Interfaces:**
- Consumes: `database/sql.DB`, `ApiTypes.JimoLogger` (`github.com/chendingplano/shared/go/api/ApiTypes`), `context.Context`.
- Produces (used by Tasks 3-4):
  - `const objectAuditActionResolveObjectID = "resolve_object_id"`
  - `const objectAuditActionEditFields = "edit_fields"`
  - `func logObjectAudit(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, tableName, rowKey, action, actor string, payload map[string]json.RawMessage)`

- [ ] **Step 1: Write the failing tests**

Create `server/api/kbhandler/object_audit_log_test.go`:

```go
package kbhandler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

func testAuditLogger(t *testing.T) ApiTypes.JimoLogger {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	rc := EchoFactory.NewFromEcho(c, "TEST_OAL_001")
	return rc.GetLogger()
}

func TestLogObjectAuditInsertsRowWithActor(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	payload := map[string]json.RawMessage{"object_id": json.RawMessage(`"obj_b"`)}

	insertQuery := regexp.QuoteMeta(
		"INSERT INTO kb.object_audit_log (table_name, row_key, action, changes, actor) VALUES ($1,$2,$3,$4,$5)",
	)
	mock.ExpectExec(insertQuery).
		WithArgs("kb.artifact_objects", "42", "resolve_object_id", `{"object_id":"obj_b"}`, "alice").
		WillReturnResult(sqlmock.NewResult(1, 1))

	logObjectAudit(context.Background(), db, testAuditLogger(t), "kb.artifact_objects", "42", "resolve_object_id", "alice", payload)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestLogObjectAuditInsertsNullActorWhenUnauthenticated(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	payload := map[string]json.RawMessage{"description": json.RawMessage(`"fixed typo"`)}

	insertQuery := regexp.QuoteMeta(
		"INSERT INTO kb.object_audit_log (table_name, row_key, action, changes, actor) VALUES ($1,$2,$3,$4,$5)",
	)
	mock.ExpectExec(insertQuery).
		WithArgs("kb.object_nodes", "obj_a", "edit_fields", `{"description":"fixed typo"}`, nil).
		WillReturnResult(sqlmock.NewResult(1, 1))

	logObjectAudit(context.Background(), db, testAuditLogger(t), "kb.object_nodes", "obj_a", "edit_fields", "", payload)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestLogObjectAuditSwallowsInsertError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	payload := map[string]json.RawMessage{"description": json.RawMessage(`"x"`)}

	insertQuery := regexp.QuoteMeta(
		"INSERT INTO kb.object_audit_log (table_name, row_key, action, changes, actor) VALUES ($1,$2,$3,$4,$5)",
	)
	mock.ExpectExec(insertQuery).WillReturnError(fmt.Errorf("boom"))

	// Must not panic and must not return an error (best-effort, fire-and-forget).
	logObjectAudit(context.Background(), db, testAuditLogger(t), "kb.artifact_objects", "1", "edit_fields", "", payload)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./server/api/kbhandler/... -run TestLogObjectAudit -v`
Expected: FAIL — `undefined: logObjectAudit` (compile error; the function doesn't exist yet).

- [ ] **Step 3: Write the implementation**

Create `server/api/kbhandler/object_audit_log.go`:

```go
package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

const (
	objectAuditActionResolveObjectID = "resolve_object_id"
	objectAuditActionEditFields      = "edit_fields"
)

// logObjectAudit is a best-effort insert into kb.object_audit_log recording
// one successful PATCH against kb.artifact_objects or kb.object_nodes made
// from the Resolve Ambiguous Objects admin page. Failures are logged and
// swallowed so a logging problem never fails the caller's PATCH response,
// matching the docactivity.Log precedent used by the doc-structure editor
// (server/api/docactivity/activity.go).
func logObjectAudit(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger,
	tableName, rowKey, action, actor string, payload map[string]json.RawMessage) {
	changes, err := json.Marshal(payload)
	if err != nil {
		logger.Warn("marshal object audit changes failed", "table_name", tableName, "row_key", rowKey, "err", err)
		return
	}
	var actorArg any
	if actor != "" {
		actorArg = actor
	}
	_, err = db.ExecContext(ctx,
		`INSERT INTO kb.object_audit_log (table_name, row_key, action, changes, actor) VALUES ($1,$2,$3,$4,$5)`,
		tableName, rowKey, action, string(changes), actorArg)
	if err != nil {
		logger.Warn("insert object audit log failed", "table_name", tableName, "row_key", rowKey, "err", err)
		return
	}
	logger.Info("object audit logged", "table_name", tableName, "row_key", rowKey, "action", action)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./server/api/kbhandler/... -run TestLogObjectAudit -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
jj describe -m "Add best-effort logObjectAudit helper for kb.object_audit_log"
jj new
```

---

### Task 3: Wire audit logging into `UpdateArtifactObject`

**Files:**
- Modify: `server/api/kbhandler/ambiguous_objects_handler.go`
- Modify: `server/api/kbhandler/ambiguous_objects_handler_test.go`

**Interfaces:**
- Consumes: `logObjectAudit`, `objectAuditActionResolveObjectID`, `objectAuditActionEditFields` (Task 2); `structureActor(rc ApiTypes.RequestContext) string` (existing, `doc_structure_handler.go`); the existing `settingObjectID bool` local and `payload map[string]json.RawMessage` local already computed inside `UpdateArtifactObject`.
- Produces: no new exported symbols — behavior change only (existing endpoint now also writes an audit row on success).

- [ ] **Step 1: Update the existing success test and add two new tests (failing first)**

In `server/api/kbhandler/ambiguous_objects_handler_test.go`, modify `TestUpdateArtifactObjectSuccessStampsExtInfoWhenObjectIDSet` to add the audit expectation (insert immediately after the existing `mock.ExpectExec(updateQuery)...` block, before the `c, rec := newUpdateArtifactObjectContext(...)` line):

```go
	auditQuery := regexp.QuoteMeta(
		"INSERT INTO kb.object_audit_log (table_name, row_key, action, changes, actor) VALUES ($1,$2,$3,$4,$5)",
	)
	mock.ExpectExec(auditQuery).
		WithArgs("kb.artifact_objects", "42", "resolve_object_id",
			`{"object_id":"obj_b","object_name":"Pressure Regulator","reconcile_status":"ambiguous_resolved"}`, nil).
		WillReturnResult(sqlmock.NewResult(1, 1))
```

Add a new test, appended after `TestUpdateArtifactObjectNotFound`:

```go
func TestUpdateArtifactObjectAuditLogsEditFieldsWhenNoObjectIDChange(t *testing.T) {
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
		WithArgs("fixed typo", int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	auditQuery := regexp.QuoteMeta(
		"INSERT INTO kb.object_audit_log (table_name, row_key, action, changes, actor) VALUES ($1,$2,$3,$4,$5)",
	)
	mock.ExpectExec(auditQuery).
		WithArgs("kb.artifact_objects", "42", "edit_fields", `{"description":"fixed typo"}`, nil).
		WillReturnResult(sqlmock.NewResult(1, 1))

	c, rec := newUpdateArtifactObjectContext(t, "42", `{"description":"fixed typo"}`)
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./server/api/kbhandler/... -run TestUpdateArtifactObject -v`
Expected: FAIL — `TestUpdateArtifactObjectSuccessStampsExtInfoWhenObjectIDSet` and the new test fail with "unmet db expectations" / "there is a remaining expectation which was not matched" (the handler doesn't issue the audit INSERT yet).

- [ ] **Step 3: Wire the call into the handler**

In `server/api/kbhandler/ambiguous_objects_handler.go`, in `UpdateArtifactObject`, replace:

```go
	if affected == 0 {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "artifact object not found (CWB_KB_AAO_220)"})
	}

	return c.JSON(http.StatusOK, map[string]any{"status": true})
}
```

with:

```go
	if affected == 0 {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "artifact object not found (CWB_KB_AAO_220)"})
	}

	action := objectAuditActionEditFields
	if settingObjectID {
		action = objectAuditActionResolveObjectID
	}
	logObjectAudit(c.Request().Context(), db, logger, "kb.artifact_objects", idStr, action, structureActor(rc), payload)

	return c.JSON(http.StatusOK, map[string]any{"status": true})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./server/api/kbhandler/... -run TestUpdateArtifactObject -v`
Expected: PASS (5 tests: the 4 existing plus the new one).

- [ ] **Step 5: Commit**

```bash
jj describe -m "Audit-log kb.artifact_objects PATCHes from the ambiguous-objects admin page"
jj new
```

---

### Task 4: Wire audit logging into `UpdateObjectNode`

**Files:**
- Modify: `server/api/kbhandler/ambiguous_objects_handler.go`
- Modify: `server/api/kbhandler/ambiguous_objects_handler_test.go`

**Interfaces:**
- Consumes: same as Task 3 (`logObjectAudit`, `objectAuditActionEditFields`, `structureActor`), plus the existing `payload` local in `UpdateObjectNode`.
- Produces: no new exported symbols — behavior change only.

- [ ] **Step 1: Update the existing success test (failing first)**

In `server/api/kbhandler/ambiguous_objects_handler_test.go`, modify `TestUpdateObjectNodeSuccess` to add the audit expectation immediately after the existing `mock.ExpectExec(updateQuery)...` block, before `c, rec := newUpdateObjectNodeContext(...)`:

```go
	auditQuery := regexp.QuoteMeta(
		"INSERT INTO kb.object_audit_log (table_name, row_key, action, changes, actor) VALUES ($1,$2,$3,$4,$5)",
	)
	mock.ExpectExec(auditQuery).
		WithArgs("kb.object_nodes", "obj_a", "edit_fields",
			`{"aliases":["reg","regulator"],"canonical_name":"Pressure Regulator"}`, nil).
		WillReturnResult(sqlmock.NewResult(1, 1))
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/api/kbhandler/... -run TestUpdateObjectNodeSuccess -v`
Expected: FAIL — "there is a remaining expectation which was not matched" (the audit INSERT).

- [ ] **Step 3: Wire the call into the handler**

In `server/api/kbhandler/ambiguous_objects_handler.go`, in `UpdateObjectNode`, replace:

```go
	if affected == 0 {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "object node not found (CWB_KB_AAO_317)"})
	}

	return c.JSON(http.StatusOK, map[string]any{"status": true})
}
```

with:

```go
	if affected == 0 {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "object node not found (CWB_KB_AAO_317)"})
	}

	logObjectAudit(c.Request().Context(), db, logger, "kb.object_nodes", objectID, objectAuditActionEditFields, structureActor(rc), payload)

	return c.JSON(http.StatusOK, map[string]any{"status": true})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./server/api/kbhandler/... -run TestUpdateObjectNodeSuccess -v`
Expected: PASS.

- [ ] **Step 5: Full backend regression check, then commit**

Run: `go build ./... && go vet ./server/api/kbhandler/... && go test ./server/api/kbhandler/...`
Expected: build clean, vet clean, all tests in the package pass (including the untouched `TestUpdateObjectNodeRejectsNullCanonicalName` / `TestUpdateObjectNodeNotFound`, which issue no audit insert and must still pass with no new expectation added to them).

```bash
jj describe -m "Audit-log kb.object_nodes PATCHes from the ambiguous-objects admin page"
jj new
```

---

### Task 5: Frontend — `fieldDirty` comparator

**Files:**
- Modify: `web/src/lib/components/home3/resolve-ambiguous-objects-client.ts`
- Modify: `web/src/lib/components/home3/resolve-ambiguous-objects-client.test.ts`

**Interfaces:**
- Consumes: nothing new.
- Produces (used by Task 6): `export function fieldDirty(current: unknown, snapshot: unknown): boolean`.

- [ ] **Step 1: Write the failing tests**

Append to `web/src/lib/components/home3/resolve-ambiguous-objects-client.test.ts` (add `fieldDirty` to the existing import block at the top of the file first):

```ts
import {
	buildArtifactObjectPatch,
	buildObjectNodePatch,
	fieldDirty,
	getAmbiguousObjectDetail,
	listAmbiguousObjects,
	neighborAmbiguousId,
	updateArtifactObject,
	updateObjectNode,
	type ArtifactObjectDetail,
	type ObjectNodeCandidate
} from './resolve-ambiguous-objects-client.js';
```

Then append these tests at the end of the file:

```ts
test('fieldDirty is false for equal primitives and true for different primitives', () => {
	assert.equal(fieldDirty('a', 'a'), false);
	assert.equal(fieldDirty('a', 'b'), true);
	assert.equal(fieldDirty(0.85, 0.85), false);
	assert.equal(fieldDirty(0.85, 0.9), true);
});

test('fieldDirty compares arrays by content and order, not reference', () => {
	assert.equal(fieldDirty(['reg', 'regulator'], ['reg', 'regulator']), false);
	assert.equal(fieldDirty(['reg', 'regulator'], ['reg']), true);
	assert.equal(fieldDirty(['reg', 'regulator'], ['regulator', 'reg']), true);
});

test('fieldDirty treats a missing snapshot as dirty only when current differs from it', () => {
	assert.equal(fieldDirty('Pressure Regulator', undefined), true);
	assert.equal(fieldDirty(undefined, undefined), false);
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run (from `web/`): `bun test src/lib/components/home3/resolve-ambiguous-objects-client.test.ts`
Expected: FAIL — `fieldDirty is not a function` / import error (not exported yet).

- [ ] **Step 3: Write the implementation**

In `web/src/lib/components/home3/resolve-ambiguous-objects-client.ts`, append after `neighborAmbiguousId`:

```ts
/**
 * True when `current` differs from `snapshot`. Array-aware and order-sensitive
 * (same comparison semantics as diffFields above), used to highlight
 * unsaved-edit state per field in the admin view.
 */
export function fieldDirty(current: unknown, snapshot: unknown): boolean {
	if (Array.isArray(current) && Array.isArray(snapshot)) {
		return JSON.stringify(current) !== JSON.stringify(snapshot);
	}
	return current !== snapshot;
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run (from `web/`): `bun test src/lib/components/home3/resolve-ambiguous-objects-client.test.ts`
Expected: PASS (all tests in the file, 11 total: 8 existing + 3 new).

- [ ] **Step 5: Commit**

```bash
jj describe -m "Add fieldDirty comparator for per-field dirty highlighting"
jj new
```

---

### Task 6: Frontend — two-column layout, Use-this sets matched, dirty highlighting

**Files:**
- Modify: `web/src/lib/components/home3/resolve-ambiguous-objects-view.svelte`

**Interfaces:**
- Consumes: `fieldDirty` (Task 5), all existing exports from `resolve-ambiguous-objects-client.ts` (unchanged), existing component state (`currentObject`, `snapshotObject`, `currentNodes`, `snapshotNodes`, `aliasesText`, `parseAliasesText`, design tokens).
- Produces: no new exports (this is a leaf `.svelte` component) — visual/behavioral change only.

- [ ] **Step 1: Import `fieldDirty` and add local helpers**

In the `<script>` block, change the import block (currently `import { listAmbiguousObjects, getAmbiguousObjectDetail, updateArtifactObject, updateObjectNode, buildArtifactObjectPatch, buildObjectNodePatch, neighborAmbiguousId, RECONCILE_STATUS_OPTIONS, type AmbiguousObjectSummary, type ArtifactObjectDetail, type ObjectNodeCandidate } from './resolve-ambiguous-objects-client.js';`) to:

```ts
	import {
		listAmbiguousObjects,
		getAmbiguousObjectDetail,
		updateArtifactObject,
		updateObjectNode,
		buildArtifactObjectPatch,
		buildObjectNodePatch,
		neighborAmbiguousId,
		fieldDirty,
		RECONCILE_STATUS_OPTIONS,
		type AmbiguousObjectSummary,
		type ArtifactObjectDetail,
		type ObjectNodeCandidate
	} from './resolve-ambiguous-objects-client.js';
```

Immediately after the existing `isDirty` `$derived.by` block (the one ending `return JSON.stringify(snapshotNodes) !== JSON.stringify(currentNodes); });`), add:

```ts
	function objDirty(field: keyof ArtifactObjectDetail): boolean {
		if (!currentObject || !snapshotObject) return false;
		return fieldDirty(currentObject[field], snapshotObject[field]);
	}

	function nodeDirty(index: number, field: keyof ObjectNodeCandidate): boolean {
		if (!currentNodes[index] || !snapshotNodes[index]) return false;
		return fieldDirty(currentNodes[index][field], snapshotNodes[index][field]);
	}

	function dirtyStyle(dirty: boolean): string {
		return dirty ? 'border-color:#F87171; color:#F87171;' : '';
	}
```

- [ ] **Step 2: Update `useCandidate` to also set `reconcile_status`**

Replace:

```ts
	function useCandidate(objectId: string) {
		if (!currentObject) return;
		currentObject.object_id = objectId;
	}
```

with:

```ts
	function useCandidate(objectId: string) {
		if (!currentObject) return;
		currentObject.object_id = objectId;
		currentObject.reconcile_status = 'matched';
	}
```

- [ ] **Step 3: Replace the right-panel content block**

In the template, the block from the `{:else}` (following `{:else if !currentObject}`) through its matching `{/if}` currently contains the "Artifact Object" card followed by the "Related Object Nodes" card. Replace that entire `{:else} ... {/if}` block's contents (keep the `{:else}` and `{/if}` lines themselves) with:

```svelte
			<div class="grid grid-cols-1 lg:grid-cols-2 gap-5" style="align-items:start;">
				<!-- Left column: Artifact Object -->
				<div class="rounded-xl p-5" style="background:{cardBg}; border:1px solid {borderColor};">
					<div class="flex items-center justify-between mb-4">
						<h2 style="font-size:14px; font-weight:600; color:{textPrimary};">Artifact Object</h2>
						<span style="font-size:11px; color:{textMuted};">
							id {currentObject.id} · {currentObject.artifact_type} · {currentObject.artifact_id}
						</span>
					</div>
					<div class="grid grid-cols-2 gap-3">
						<label class="flex flex-col gap-1">
							<span style="font-size:11px; color:{textMuted};">Object Name</span>
							<input bind:value={currentObject.object_name} style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(objDirty('object_name'))}" />
						</label>
						<label class="flex flex-col gap-1">
							<span style="font-size:11px; color:{textMuted};">Object Name (EN)</span>
							<input bind:value={currentObject.object_name_en} style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(objDirty('object_name_en'))}" />
						</label>
						<label class="flex flex-col gap-1">
							<span style="font-size:11px; color:{textMuted};">Object Name (ZH)</span>
							<input bind:value={currentObject.object_name_zh} style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(objDirty('object_name_zh'))}" />
						</label>
						<label class="flex flex-col gap-1">
							<span style="font-size:11px; color:{textMuted};">Language</span>
							<input bind:value={currentObject.language} style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(objDirty('language'))}" />
						</label>
						<label class="flex flex-col gap-1">
							<span style="font-size:11px; color:{textMuted};">Object Type</span>
							<input bind:value={currentObject.object_type} style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(objDirty('object_type'))}" />
						</label>
						<label class="flex flex-col gap-1">
							<span style="font-size:11px; color:{textMuted};">Object Role</span>
							<input bind:value={currentObject.object_role} style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(objDirty('object_role'))}" />
						</label>
						<label class="flex flex-col gap-1 col-span-2">
							<span style="font-size:11px; color:{textMuted};">Aliases (comma-separated)</span>
							<input
								value={aliasesText(currentObject.aliases)}
								oninput={(e) => { if (currentObject) currentObject.aliases = parseAliasesText((e.currentTarget as HTMLInputElement).value); }}
								style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(objDirty('aliases'))}"
							/>
						</label>
						<label class="flex flex-col gap-1 col-span-2">
							<span style="font-size:11px; color:{textMuted};">Acronyms (comma-separated)</span>
							<input
								value={aliasesText(currentObject.acronyms)}
								oninput={(e) => { if (currentObject) currentObject.acronyms = parseAliasesText((e.currentTarget as HTMLInputElement).value); }}
								style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(objDirty('acronyms'))}"
							/>
						</label>
						<label class="flex flex-col gap-1 col-span-2">
							<span style="font-size:11px; color:{textMuted};">Description</span>
							<textarea bind:value={currentObject.description} rows="2" style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(objDirty('description'))}"></textarea>
						</label>
						<label class="flex flex-col gap-1 col-span-2">
							<span style="font-size:11px; color:{textMuted};">Evidence Quote</span>
							<textarea bind:value={currentObject.evidence_quote} rows="2" style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(objDirty('evidence_quote'))}"></textarea>
						</label>
						<label class="flex flex-col gap-1">
							<span style="font-size:11px; color:{textMuted};">Object ID</span>
							<input bind:value={currentObject.object_id} placeholder="(unresolved)" style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(objDirty('object_id'))}" />
						</label>
						<label class="flex flex-col gap-1">
							<span style="font-size:11px; color:{textMuted};">Reconcile Status</span>
							<select bind:value={currentObject.reconcile_status} style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(objDirty('reconcile_status'))}">
								{#each RECONCILE_STATUS_OPTIONS as opt}
									<option value={opt}>{opt}</option>
								{/each}
							</select>
						</label>
						<label class="flex flex-col gap-1">
							<span style="font-size:11px; color:{textMuted};">Reconcile Confidence</span>
							<input type="number" min="0" max="1" step="0.01" bind:value={currentObject.reconcile_confidence} style="background:{pageBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(objDirty('reconcile_confidence'))}" />
						</label>
					</div>
				</div>

				<!-- Right column: Top Region (quick reference) + Bottom Region (editable candidates) -->
				<div class="flex flex-col gap-5">
					<div class="rounded-xl p-5" style="background:{cardBg}; border:1px solid {borderColor};">
						<h2 style="font-size:14px; font-weight:600; color:{textPrimary}; margin-bottom:12px;">Candidates</h2>
						{#if currentNodes.length === 0}
							<div style="font-size:13px; color:{textMuted};">No candidate object nodes found for this artifact object.</div>
						{:else}
							<table style="width:100%; border-collapse:collapse; font-size:12px;">
								<thead>
									<tr>
										<th style="text-align:left; padding:6px 8px; color:{textMuted}; font-weight:500; border-bottom:1px solid {borderColor};">Canonical Name</th>
										<th style="text-align:left; padding:6px 8px; color:{textMuted}; font-weight:500; border-bottom:1px solid {borderColor};">Description</th>
										<th style="text-align:right; padding:6px 8px; color:{textMuted}; font-weight:500; border-bottom:1px solid {borderColor};">Action</th>
									</tr>
								</thead>
								<tbody>
									{#each currentNodes as node (node.object_id)}
										<tr>
											<td style="padding:6px 8px; color:{textPrimary}; border-bottom:1px solid {borderColor}; vertical-align:top;">
												{node.canonical_name}
												{#if node.recommended}
													<span style="display:inline-block; margin-left:6px; font-size:10px; font-weight:600; padding:2px 6px; border-radius:4px; background:{accentTint}; color:{accent};">Recommended</span>
												{/if}
											</td>
											<td style="padding:6px 8px; color:{textSecondary}; border-bottom:1px solid {borderColor}; max-width:260px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; vertical-align:top;">
												{node.description}
											</td>
											<td style="padding:6px 8px; border-bottom:1px solid {borderColor}; text-align:right; vertical-align:top;">
												<button
													type="button"
													onclick={() => useCandidate(node.object_id)}
													style="font-size:11px; font-weight:500; padding:4px 10px; border-radius:6px; border:none; cursor:pointer; background:{accent}; color:white;"
												>
													Use this
												</button>
											</td>
										</tr>
									{/each}
								</tbody>
							</table>
						{/if}
					</div>

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
									<span style="font-size:11px; color:{textMuted};">score {node.score.toFixed(2)} · {node.method}</span>
								</div>
								<div class="grid grid-cols-2 gap-3">
									<label class="flex flex-col gap-1">
										<span style="font-size:11px; color:{textMuted};">Canonical Name</span>
										<input bind:value={currentNodes[i].canonical_name} style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(nodeDirty(i, 'canonical_name'))}" />
									</label>
									<label class="flex flex-col gap-1">
										<span style="font-size:11px; color:{textMuted};">Object Type</span>
										<input bind:value={currentNodes[i].object_type} style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(nodeDirty(i, 'object_type'))}" />
									</label>
									<label class="flex flex-col gap-1">
										<span style="font-size:11px; color:{textMuted};">Canonical Name (EN)</span>
										<input bind:value={currentNodes[i].canonical_name_en} style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(nodeDirty(i, 'canonical_name_en'))}" />
									</label>
									<label class="flex flex-col gap-1">
										<span style="font-size:11px; color:{textMuted};">Canonical Name (ZH)</span>
										<input bind:value={currentNodes[i].canonical_name_zh} style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(nodeDirty(i, 'canonical_name_zh'))}" />
									</label>
									<label class="flex flex-col gap-1">
										<span style="font-size:11px; color:{textMuted};">Primary Language</span>
										<input bind:value={currentNodes[i].primary_language} style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(nodeDirty(i, 'primary_language'))}" />
									</label>
									<label class="flex flex-col gap-1">
										<span style="font-size:11px; color:{textMuted};">Aliases (comma-separated)</span>
										<input
											value={aliasesText(node.aliases)}
											oninput={(e) => { currentNodes[i].aliases = parseAliasesText((e.currentTarget as HTMLInputElement).value); }}
											style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(nodeDirty(i, 'aliases'))}"
										/>
									</label>
									<label class="flex flex-col gap-1">
										<span style="font-size:11px; color:{textMuted};">Acronyms (comma-separated)</span>
										<input
											value={aliasesText(node.acronyms)}
											oninput={(e) => { currentNodes[i].acronyms = parseAliasesText((e.currentTarget as HTMLInputElement).value); }}
											style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(nodeDirty(i, 'acronyms'))}"
										/>
									</label>
									<label class="flex flex-col gap-1 col-span-2">
										<span style="font-size:11px; color:{textMuted};">Description</span>
										<textarea bind:value={currentNodes[i].description} rows="2" style="background:{cardBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:6px 8px; font-size:13px; {dirtyStyle(nodeDirty(i, 'description'))}"></textarea>
									</label>
								</div>
							</div>
						{/each}
					</div>
				</div>
			</div>
```

- [ ] **Step 4: Type-check**

Run (from `web/`): `bun run check`
Expected: zero new errors introduced by this file (pre-existing unrelated errors elsewhere in the project, if any, are out of scope — compare against a `bun run check` baseline captured before this task if unsure).

- [ ] **Step 5: Manual verification in the browser**

Start the dev server (per this repo's normal dev workflow, e.g. `mise dev` from the project root or `bun run dev` from `web/`) and open the Resolve Ambiguous Objects page (System Admin → Database Maintenance → Resolve Ambiguous Objects). Confirm, for a row that has at least one candidate:
1. The right panel renders as two columns: Artifact Object on the left; a compact "Candidates" table (Canonical Name / Description / Action) above a "Related Object Nodes" detailed-cards block on the right.
2. The detailed candidate cards no longer have their own "Use this" button.
3. Clicking "Use this" in the compact table fills the Artifact Object's Object ID field and changes Reconcile Status to `matched`, and both fields immediately render with a red border/text (dirty), while Save becomes enabled.
4. Editing any other field (e.g. Object Name, or a candidate's Canonical Name) also turns that specific field red; fields you have not touched stay in their normal color.
5. Clicking Save persists the change, the row leaves the left-panel queue (since `reconcile_status` is no longer `ambiguous`), and the next row loads with no fields marked dirty.
6. Clicking Cancel after an edit reverts the field(s) and clears the red styling.

- [ ] **Step 6: Commit**

```bash
jj describe -m "Two-column layout, Use-this sets matched, dirty-field highlighting for Resolve Ambiguous Objects"
jj new
```

---

### Task 7: Final verification

**Files:** none (verification only).

- [ ] **Step 1: Full backend suite**

Run: `go build ./... && go vet ./server/api/kbhandler/... && go test ./server/api/kbhandler/...`
Expected: build clean, vet clean, all tests pass (existing `ambiguous_objects_handler_test.go` tests + new `object_audit_log_test.go` tests).

- [ ] **Step 2: Full frontend suite**

Run (from `web/`): `bun test src/lib/components/home3/resolve-ambiguous-objects-client.test.ts && bun run check`
Expected: all tests pass; `check` reports no new errors from files this plan touched.

- [ ] **Step 3: Update the parent ADR's status line**

In `KnowledgeStore/doc-repo/adrs/202607/2026070701-adr-object-reconciliation-ambiguous-tie-resolution.md`, add a Change Log entry noting AD4 was implemented (date, one line), consistent with this ADR's existing Change Log convention. This is documentation only — no code.

- [ ] **Step 4: Commit**

```bash
jj describe -m "AD4: verify Resolve Ambiguous Objects refinements end-to-end"
jj new
```
