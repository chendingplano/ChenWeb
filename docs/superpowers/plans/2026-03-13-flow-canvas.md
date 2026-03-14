# Flow Canvas Editor Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a full-screen flow/canvas editor to home3 — users build node-based workflows, save flows to PostgreSQL, and manage templates.

**Architecture:** `canvas-01.svelte` mounts as a full-screen overlay inside `content-panel.svelte` when `activeMenu.itemId === 'flow'`. The canvas uses `@xyflow/svelte` for rendering; all 10 REST endpoints are backend stubs (real routing + auth, no DB). The nav-rail collapses to icon-only while the canvas is open and restores on close.

**Tech Stack:** Svelte 5 (runes), SvelteKit 2, `@xyflow/svelte`, Tailwind CSS v4, Lucide Svelte icons; Go 1.25, Echo v4, PostgreSQL (migration only — stubs don't query DB)

**Spec:** `docs/superpowers/specs/2026-03-13-flow-canvas-design.md`

---

## Chunk 1: Foundation — Types, Service, Backend Stubs, Migration

### Task 1: Install `@xyflow/svelte`

**Files:**
- Modify: `web/package.json`

- [ ] **Step 1: Install the package**

```bash
cd /Users/cding/Workspace/ChenWeb/web
bun add @xyflow/svelte
```

- [ ] **Step 2: Verify import resolves**

```bash
bun run check
```
Expected: no "cannot find module @xyflow/svelte" errors

- [ ] **Step 3: Commit**

```bash
git add web/package.json web/bun.lockb
git commit -m "chore: install @xyflow/svelte"
```

---

### Task 2: TypeScript types — `flow.ts`

**Files:**
- Create: `web/src/lib/types/flow.ts`

- [ ] **Step 1: Create the types file**

```typescript
// web/src/lib/types/flow.ts

export interface Connector {
  id: string;       // e.g. "out-text", "in-context"
  label: string;    // display name
  direction: 'input' | 'output';
}

export interface NodeType {
  id: string;           // e.g. "ai-assistant"
  label: string;
  category: string;     // "AI" | "Data" | "Actions"
  icon: string;         // lucide icon name
  inputs: string[];     // connector labels
  outputs: string[];
  defaultData: Record<string, unknown>;
}

export interface FlowNode {
  id: string;
  type: string;         // matches NodeType.id
  position: { x: number; y: number };
  data: Record<string, unknown>;
}

export interface FlowEdge {
  id: string;
  source: string;
  sourceHandle: string;
  target: string;
  targetHandle: string;
}

export interface FlowData {
  nodes: FlowNode[];
  edges: FlowEdge[];
}

export interface Flow {
  flow_id: number;
  user_id: number;
  flow_name: string;
  flow_desc: string;
  is_default: boolean;
  is_shared: boolean;
  is_template: boolean;
  template_category: string;
  flow_data: FlowData;
  thumbnail_svg: string | null;
  created_at: string;
  updated_at: string;
}

export interface Snapshot {
  nodes: FlowNode[];
  edges: FlowEdge[];
}

export interface ApiError {
  error: { code: string; message: string };
}
```

- [ ] **Step 2: Verify types compile**

```bash
cd /Users/cding/Workspace/ChenWeb/web && bun run check
```
Expected: 0 errors

- [ ] **Step 3: Commit**

```bash
git add web/src/lib/types/flow.ts
git commit -m "feat(flow): add TypeScript types"
```

---

### Task 3: Frontend service — `flowService.ts`

**Files:**
- Create: `web/src/lib/services/flowService.ts`

- [ ] **Step 1: Create the service**

```typescript
// web/src/lib/services/flowService.ts
import type { Flow, NodeType } from '$lib/types/flow';

const BASE = '/api/v1';

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : {},
    body: body ? JSON.stringify(body) : undefined,
    credentials: 'same-origin',
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: { code: 'UNKNOWN', message: res.statusText } }));
    throw err;
  }
  return res.json();
}

export type FlowScope = 'mine' | 'shared' | 'templates';

export const flowService = {
  list: (scope: FlowScope): Promise<{ flows: Flow[] }> =>
    request('GET', `/flows?scope=${scope}`),

  create: (payload: Pick<Flow, 'flow_name' | 'flow_data' | 'is_shared'> & { thumbnail_svg?: string | null }): Promise<{ flow: Flow }> =>
    request('POST', '/flows', payload),

  getDefault: (): Promise<{ flow: Flow }> =>
    request('GET', '/flows/default'),

  get: (id: number): Promise<{ flow: Flow }> =>
    request('GET', `/flows/${id}`),

  update: (id: number, payload: Partial<Pick<Flow, 'flow_name' | 'flow_desc' | 'flow_data' | 'is_shared' | 'thumbnail_svg'>>): Promise<{ flow: Flow }> =>
    request('PUT', `/flows/${id}`, payload),

  delete: (id: number): Promise<void> =>
    request('DELETE', `/flows/${id}`),

  setDefault: (id: number): Promise<void> =>
    request('PUT', `/flows/${id}/default`),

  fork: (id: number): Promise<{ flow: Flow }> =>
    request('POST', `/flows/${id}/fork`),

  saveAsTemplate: (id: number): Promise<{ flow: Flow }> =>
    request('POST', `/flows/${id}/template`),

  getNodeTypes: (): Promise<{ nodeTypes: NodeType[] }> =>
    request('GET', '/flow-node-types'),
};
```

- [ ] **Step 2: Type-check**

```bash
cd /Users/cding/Workspace/ChenWeb/web && bun run check
```
Expected: 0 errors

- [ ] **Step 3: Commit**

```bash
git add web/src/lib/services/flowService.ts
git commit -m "feat(flow): add flowService"
```

---

### Task 4: Backend — table definition

**Files:**
- Create: `server/api/appdatastores/table-flows.go`

Read `server/api/appdatastores/table-documents.go` first to match patterns exactly (package name, imports, function signature).

- [ ] **Step 1: Create table-flows.go**

```go
// server/api/appdatastores/table-flows.go
package appdatastores

import (
	"database/sql"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/databaseutil"
)

// CreateFlowsTable creates the flows table and its supporting index.
// Pre-condition: confirm that users(user_id) exists in the schema
// before running this migration in production.
func CreateFlowsTable(logger ApiTypes.JimoLogger) error {
	var db *sql.DB = ApiTypes.ProjectDBHandle

	stmt := `
	CREATE TABLE IF NOT EXISTS flows (
		flow_id           BIGSERIAL PRIMARY KEY,
		user_id           BIGINT NOT NULL,
		flow_name         VARCHAR(255) NOT NULL,
		flow_desc         TEXT,
		is_default        BOOLEAN NOT NULL DEFAULT FALSE,
		is_shared         BOOLEAN NOT NULL DEFAULT FALSE,
		is_template       BOOLEAN NOT NULL DEFAULT FALSE,
		template_category VARCHAR(100),
		flow_data         JSONB NOT NULL DEFAULT '{"nodes":[],"edges":[]}',
		thumbnail_svg     TEXT,
		created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);`

	if err := databaseutil.ExecuteStatement(db, stmt); err != nil {
		logger.Error("CreateFlowsTable: failed to create table: " + err.Error())
		return err
	}

	// Partial unique index: only one default flow per user.
	indexStmt := `
	CREATE UNIQUE INDEX IF NOT EXISTS flows_user_default_idx
		ON flows (user_id) WHERE is_default = TRUE;`

	if err := databaseutil.ExecuteStatement(db, indexStmt); err != nil {
		logger.Error("CreateFlowsTable: failed to create index: " + err.Error())
		return err
	}

	logger.Info("CreateFlowsTable: ok")
	return nil
}
```

- [ ] **Step 2: Register in CreateTables**

Open `server/api/database/createtables.go` (or wherever `CreateDocumentsTable` is called). Add:
```go
if err := appdatastores.CreateFlowsTable(logger); err != nil {
    return err
}
```

- [ ] **Step 3: Build**

```bash
cd /Users/cding/Workspace/ChenWeb && go build ./...
```
Expected: 0 errors

- [ ] **Step 4: Commit**

```bash
git add server/api/appdatastores/table-flows.go server/api/database/
git commit -m "feat(flow): add flows table definition"
```

---

### Task 5: Backend — Goose migration

**Files:**
- Create: `server/migrations/20260313120000_create_flows.sql`

- [ ] **Step 1: Create migration file**

```sql
-- server/migrations/20260313120000_create_flows.sql
-- +goose Up
CREATE TABLE IF NOT EXISTS flows (
    flow_id           BIGSERIAL PRIMARY KEY,
    user_id           BIGINT NOT NULL,
    flow_name         VARCHAR(255) NOT NULL,
    flow_desc         TEXT,
    is_default        BOOLEAN NOT NULL DEFAULT FALSE,
    is_shared         BOOLEAN NOT NULL DEFAULT FALSE,
    is_template       BOOLEAN NOT NULL DEFAULT FALSE,
    template_category VARCHAR(100),
    flow_data         JSONB NOT NULL DEFAULT '{"nodes":[],"edges":[]}',
    thumbnail_svg     TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS flows_user_default_idx
    ON flows (user_id) WHERE is_default = TRUE;

-- +goose Down
DROP INDEX IF EXISTS flows_user_default_idx;
DROP TABLE IF EXISTS flows;
```

- [ ] **Step 2: Commit**

```bash
git add server/migrations/20260313120000_create_flows.sql
git commit -m "feat(flow): add flows migration"
```

---

### Task 6: Backend — CRUD stub handlers

**Files:**
- Create: `server/api/flowhandler/flowhandler.go`

Read `server/api/aiassistanthandler/handler.go` first to match the exact stub pattern (log codes format `CWB_XXX_NNN`, import paths, JSON response shape).

- [ ] **Step 1: Write handler test**

Create `server/api/flowhandler/flowhandler_test.go`:

```go
package flowhandler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/flowhandler"
	"github.com/labstack/echo/v4"
)

func newEcho() *echo.Echo { return echo.New() }

func TestListFlows_ReturnsOK(t *testing.T) {
	e := newEcho()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/flows?scope=mine", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := flowhandler.ListFlows(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"flows"`) {
		t.Fatalf("expected flows key in response, got: %s", rec.Body.String())
	}
}

func TestCreateFlow_ReturnsCreated(t *testing.T) {
	e := newEcho()
	body := `{"flow_name":"Test","flow_data":{"nodes":[],"edges":[]},"is_shared":false}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/flows", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := flowhandler.CreateFlow(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"flow"`) {
		t.Fatalf("expected flow key in response, got: %s", rec.Body.String())
	}
}

func TestGetFlow_ReturnsOK(t *testing.T) {
	e := newEcho()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/flows/1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("1")
	if err := flowhandler.GetFlow(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestDeleteFlow_ReturnsOK(t *testing.T) {
	e := newEcho()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/flows/1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("1")
	if err := flowhandler.DeleteFlow(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run test — expect FAIL (package doesn't exist)**

```bash
cd /Users/cding/Workspace/ChenWeb && go test ./server/api/flowhandler/... 2>&1 | head -5
```
Expected: `cannot find package` or `build failed`

- [ ] **Step 3: Create flowhandler.go**

```go
// server/api/flowhandler/flowhandler.go
// Package flowhandler provides stub HTTP handlers for flow management.
// All handlers return placeholder data. Replace with real DB calls in a future phase.
package flowhandler

import (
	"net/http"
	"time"

	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
)

var stubFlow = map[string]any{
	"flow_id":           1,
	"user_id":           1,
	"flow_name":         "My First Flow",
	"flow_desc":         "",
	"is_default":        false,
	"is_shared":         false,
	"is_template":       false,
	"template_category": "",
	"flow_data":         map[string]any{"nodes": []any{}, "edges": []any{}},
	"thumbnail_svg":     nil,
	"created_at":        time.Now().Format(time.RFC3339),
	"updated_at":        time.Now().Format(time.RFC3339),
}

// ListFlows returns the list of flows filtered by scope.
// GET /api/v1/flows?scope=mine|shared|templates
func ListFlows(c echo.Context) error {
	logger := loggerutil.CreateDefaultLogger("CWB_FLW_010")
	scope := c.QueryParam("scope")
	logger.Info("ListFlows called scope=" + scope)
	return c.JSON(http.StatusOK, map[string]any{"flows": []any{stubFlow}})
}

// CreateFlow creates a new flow.
// POST /api/v1/flows
func CreateFlow(c echo.Context) error {
	logger := loggerutil.CreateDefaultLogger("CWB_FLW_020")
	logger.Info("CreateFlow called")
	return c.JSON(http.StatusCreated, map[string]any{"flow": stubFlow})
}

// GetFlow returns a single flow by ID.
// GET /api/v1/flows/:id
func GetFlow(c echo.Context) error {
	logger := loggerutil.CreateDefaultLogger("CWB_FLW_030")
	id := c.Param("id")
	logger.Info("GetFlow called id=" + id)
	return c.JSON(http.StatusOK, map[string]any{"flow": stubFlow})
}

// UpdateFlow updates a flow by ID.
// PUT /api/v1/flows/:id
func UpdateFlow(c echo.Context) error {
	logger := loggerutil.CreateDefaultLogger("CWB_FLW_040")
	id := c.Param("id")
	logger.Info("UpdateFlow called id=" + id)
	updated := map[string]any{}
	for k, v := range stubFlow {
		updated[k] = v
	}
	updated["updated_at"] = time.Now().Format(time.RFC3339)
	return c.JSON(http.StatusOK, map[string]any{"flow": updated})
}

// DeleteFlow deletes a flow by ID.
// DELETE /api/v1/flows/:id
func DeleteFlow(c echo.Context) error {
	logger := loggerutil.CreateDefaultLogger("CWB_FLW_050")
	id := c.Param("id")
	logger.Info("DeleteFlow called id=" + id)
	return c.JSON(http.StatusOK, map[string]any{"message": "flow deleted", "flow_id": id})
}
```

- [ ] **Step 4: Run tests — expect PASS**

```bash
cd /Users/cding/Workspace/ChenWeb && go test ./server/api/flowhandler/... -v
```
Expected: all 4 tests PASS

- [ ] **Step 5: Commit**

```bash
git add server/api/flowhandler/
git commit -m "feat(flow): add flow CRUD stub handlers"
```

---

### Task 7: Backend — action stub handlers

**Files:**
- Create: `server/api/flowhandler/flowhandler_actions.go`

- [ ] **Step 1: Add tests for actions** — append to `flowhandler_test.go`:

```go
func TestGetDefaultFlow_Returns200(t *testing.T) {
	e := newEcho()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/flows/default", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := flowhandler.GetDefaultFlow(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestSetDefaultFlow_Returns200(t *testing.T) {
	e := newEcho()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/flows/1/default", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("1")
	if err := flowhandler.SetDefaultFlow(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestForkFlow_Returns201(t *testing.T) {
	e := newEcho()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/flows/1/fork", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("1")
	if err := flowhandler.ForkFlow(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
}

func TestSaveAsTemplate_Returns201(t *testing.T) {
	e := newEcho()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/flows/1/template", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("1")
	if err := flowhandler.SaveAsTemplate(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run tests — expect FAIL (functions undefined)**

```bash
cd /Users/cding/Workspace/ChenWeb && go test ./server/api/flowhandler/... 2>&1 | grep "undefined"
```

- [ ] **Step 3: Create flowhandler_actions.go**

```go
// server/api/flowhandler/flowhandler_actions.go
package flowhandler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
)

// GetDefaultFlow returns the user's default flow.
// GET /api/v1/flows/default — returns 404 if none configured.
func GetDefaultFlow(c echo.Context) error {
	logger := loggerutil.CreateDefaultLogger("CWB_FLW_060")
	logger.Info("GetDefaultFlow called")
	// TODO: query DB for user's default flow
	return c.JSON(http.StatusOK, map[string]any{"flow": stubFlow})
}

// SetDefaultFlow marks a flow as the user's default (clears previous default).
// PUT /api/v1/flows/:id/default
func SetDefaultFlow(c echo.Context) error {
	logger := loggerutil.CreateDefaultLogger("CWB_FLW_070")
	id := c.Param("id")
	logger.Info("SetDefaultFlow called id=" + id)
	return c.JSON(http.StatusOK, map[string]any{"message": "default flow set", "flow_id": id})
}

// ForkFlow creates a new private flow forked from an existing one.
// POST /api/v1/flows/:id/fork
func ForkFlow(c echo.Context) error {
	logger := loggerutil.CreateDefaultLogger("CWB_FLW_080")
	id := c.Param("id")
	logger.Info("ForkFlow called source_id=" + id)
	forked := map[string]any{}
	for k, v := range stubFlow {
		forked[k] = v
	}
	forked["flow_id"] = 99
	forked["flow_name"] = fmt.Sprintf("Copy of %v", stubFlow["flow_name"])
	forked["is_default"] = false
	forked["is_template"] = false
	forked["is_shared"] = false
	forked["created_at"] = time.Now().Format(time.RFC3339)
	forked["updated_at"] = time.Now().Format(time.RFC3339)
	return c.JSON(http.StatusCreated, map[string]any{"flow": forked})
}

// SaveAsTemplate copies a flow as a public template.
// POST /api/v1/flows/:id/template
func SaveAsTemplate(c echo.Context) error {
	logger := loggerutil.CreateDefaultLogger("CWB_FLW_090")
	id := c.Param("id")
	logger.Info("SaveAsTemplate called source_id=" + id)
	tmpl := map[string]any{}
	for k, v := range stubFlow {
		tmpl[k] = v
	}
	tmpl["flow_id"] = 100
	tmpl["is_template"] = true
	tmpl["is_shared"] = true
	tmpl["created_at"] = time.Now().Format(time.RFC3339)
	tmpl["updated_at"] = time.Now().Format(time.RFC3339)
	return c.JSON(http.StatusCreated, map[string]any{"flow": tmpl})
}
```

- [ ] **Step 4: Run all handler tests — expect PASS**

```bash
cd /Users/cding/Workspace/ChenWeb && go test ./server/api/flowhandler/... -v
```
Expected: all 8 tests PASS

- [ ] **Step 5: Commit**

```bash
git add server/api/flowhandler/flowhandler_actions.go server/api/flowhandler/flowhandler_test.go
git commit -m "feat(flow): add flow action stub handlers"
```

---

### Task 8: Backend — node types handler

**Files:**
- Create: `server/api/flowhandler/nodetypes.go`

- [ ] **Step 1: Add test** — append to `flowhandler_test.go`:

```go
func TestGetNodeTypes_Returns11Types(t *testing.T) {
	e := newEcho()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/flow-node-types", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := flowhandler.GetNodeTypes(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"nodeTypes"`) {
		t.Fatalf("missing nodeTypes key: %s", rec.Body.String())
	}
	// Count occurrences of "id" to verify 11 node types
	count := strings.Count(rec.Body.String(), `"id":"`)
	if count != 11 {
		t.Fatalf("expected 11 node types, found %d", count)
	}
}
```

- [ ] **Step 2: Run test — expect FAIL**

```bash
cd /Users/cding/Workspace/ChenWeb && go test ./server/api/flowhandler/... -run TestGetNodeTypes -v 2>&1 | head -10
```

- [ ] **Step 3: Create nodetypes.go**

```go
// server/api/flowhandler/nodetypes.go
package flowhandler

import (
	"net/http"

	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
)

// GetNodeTypes returns the static list of 11 node type definitions.
// GET /api/v1/flow-node-types
func GetNodeTypes(c echo.Context) error {
	logger := loggerutil.CreateDefaultLogger("CWB_FLW_100")
	logger.Info("GetNodeTypes called")
	return c.JSON(http.StatusOK, map[string]any{
		"nodeTypes": []map[string]any{
			{
				"id": "ai-assistant", "label": "AI Assistant", "category": "AI", "icon": "Bot",
				"inputs": []string{"text", "context"}, "outputs": []string{"response"},
				"defaultData": map[string]any{"model": "gpt-4o", "system_prompt": "", "temperature": 0.7},
			},
			{
				"id": "coding-assistant", "label": "Coding Assistant", "category": "AI", "icon": "Terminal",
				"inputs": []string{"code", "context"}, "outputs": []string{"code", "explanation"},
				"defaultData": map[string]any{"language": "typescript", "model": "gpt-4o"},
			},
			{
				"id": "text", "label": "Text", "category": "Data", "icon": "Type",
				"inputs": []string{}, "outputs": []string{"text"},
				"defaultData": map[string]any{"content": ""},
			},
			{
				"id": "document", "label": "Document", "category": "Data", "icon": "FileText",
				"inputs": []string{}, "outputs": []string{"document"},
				"defaultData": map[string]any{"doc_id": "", "doc_source": ""},
			},
			{
				"id": "file", "label": "File", "category": "Data", "icon": "File",
				"inputs": []string{}, "outputs": []string{"file"},
				"defaultData": map[string]any{"file_path": "", "file_type": "txt"},
			},
			{
				"id": "media", "label": "Media", "category": "Data", "icon": "Image",
				"inputs": []string{}, "outputs": []string{"media"},
				"defaultData": map[string]any{"media_url": "", "media_type": "image"},
			},
			{
				"id": "tool", "label": "Tool", "category": "Actions", "icon": "Wrench",
				"inputs": []string{"args"}, "outputs": []string{"result"},
				"defaultData": map[string]any{"tool_name": "", "tool_config": "{}"},
			},
			{
				"id": "mcp", "label": "MCP", "category": "Actions", "icon": "Plug",
				"inputs": []string{"request"}, "outputs": []string{"response"},
				"defaultData": map[string]any{"server_url": "", "auth_token": ""},
			},
			{
				"id": "http-request", "label": "HTTP Request", "category": "Actions", "icon": "Globe",
				"inputs": []string{"body"}, "outputs": []string{"response"},
				"defaultData": map[string]any{"url": "", "method": "GET", "headers": "{}"},
			},
			{
				"id": "rule", "label": "Rule", "category": "Actions", "icon": "Filter",
				"inputs": []string{"data"}, "outputs": []string{"pass", "fail"},
				"defaultData": map[string]any{"rule_expression": "", "description": ""},
			},
			{
				"id": "git", "label": "GIT", "category": "Actions", "icon": "GitBranch",
				"inputs": []string{"files"}, "outputs": []string{"result"},
				"defaultData": map[string]any{"repo_url": "", "branch": "main", "operation": "status"},
			},
		},
	})
}
```

- [ ] **Step 4: Run all tests — expect PASS**

```bash
cd /Users/cding/Workspace/ChenWeb && go test ./server/api/flowhandler/... -v
```
Expected: all 9 tests PASS

- [ ] **Step 5: Commit**

```bash
git add server/api/flowhandler/nodetypes.go server/api/flowhandler/flowhandler_test.go
git commit -m "feat(flow): add GetNodeTypes handler"
```

---

### Task 9: Register flow routes in `routes.go`

**Files:**
- Modify: `server/api/routes.go`

- [ ] **Step 1: Read routes.go** — look at lines around the dspy handler block to know exactly where to insert

- [ ] **Step 2: Add import and routes**

In the import block, add:
```go
"github.com/chendingplano/deepdoc/server/api/flowhandler"
```

In `RegisterRoutes`, after the dspy block, add:
```go
// Flow Canvas
apiGroup.GET("/flows", flowhandler.ListFlows)
apiGroup.POST("/flows", flowhandler.CreateFlow)
apiGroup.GET("/flows/default", flowhandler.GetDefaultFlow)
apiGroup.GET("/flows/:id", flowhandler.GetFlow)
apiGroup.PUT("/flows/:id", flowhandler.UpdateFlow)
apiGroup.DELETE("/flows/:id", flowhandler.DeleteFlow)
apiGroup.PUT("/flows/:id/default", flowhandler.SetDefaultFlow)
apiGroup.POST("/flows/:id/fork", flowhandler.ForkFlow)
apiGroup.POST("/flows/:id/template", flowhandler.SaveAsTemplate)
apiGroup.GET("/flow-node-types", flowhandler.GetNodeTypes)
```

- [ ] **Step 3: Build**

```bash
cd /Users/cding/Workspace/ChenWeb && go build ./...
```
Expected: 0 errors

- [ ] **Step 4: Smoke-test routes** — start server and check two endpoints (requires PostgreSQL running and valid `.env`/`config.toml`; skip if DB unavailable):

```bash
# Start server in background (check config for port)
cd /Users/cding/Workspace/ChenWeb && go run server/cmd/deepdoc/main.go &
sleep 2
curl -s http://localhost:8080/api/v1/flow-node-types | python3 -m json.tool | head -10
curl -s http://localhost:8080/api/v1/flows?scope=mine | python3 -m json.tool | head -5
kill %1
```
Expected: valid JSON with `nodeTypes` array (11 items) and `flows` array

- [ ] **Step 5: Commit**

```bash
git add server/api/routes.go
git commit -m "feat(flow): register flow routes"
```

---

## Chunk 2: Home3 Wiring + Canvas Shell + Flow Picker

### Task 10: Add Tools → Flow to nav-rail

**Files:**
- Modify: `web/src/lib/components/home3/nav-rail.svelte`

- [ ] **Step 1: Read the file** — look at lines 1-50 (imports) and 85-140 (mainNav definition)

- [ ] **Step 2: Add Workflow import and Tools nav item**

At the top of the `<script>` block, add one import:
```typescript
import WorkflowIcon from '@lucide/svelte/icons/workflow';
```

In `mainNav`, after the `knowledge` item, add:
```typescript
{
    id: 'tools', label: 'Tools', icon: WorkflowIcon,
    children: [
        { id: 'flow', label: 'Flow' }
    ]
},
```

- [ ] **Step 3: Verify type-check passes**

```bash
cd /Users/cding/Workspace/ChenWeb/web && bun run check
```
Expected: 0 errors

- [ ] **Step 4: Commit**

```bash
git add web/src/lib/components/home3/nav-rail.svelte
git commit -m "feat(flow): add Tools→Flow to nav-rail"
```

---

### Task 11: Update content-panel to render Canvas01

**Files:**
- Modify: `web/src/lib/components/home3/content-panel.svelte`
- Create: `web/src/lib/components/shared-ui/canvas-01.svelte` (skeleton only — full implementation in later tasks)

First, create the skeleton so content-panel can import it:

- [ ] **Step 1: Create canvas-01.svelte skeleton**

```svelte
<!-- web/src/lib/components/shared-ui/canvas-01.svelte -->
<script lang="ts">
  import type { Flow } from '$lib/types/flow';

  let {
    darkMode = true,
    onClose,
    onCollapseRail,
    onRestoreRail,
  }: {
    darkMode?: boolean;
    onClose: () => void;
    onCollapseRail: () => void;
    onRestoreRail: () => void;
  } = $props();

  import { onMount, onDestroy } from 'svelte';

  onMount(() => {
    onCollapseRail();
  });

  onDestroy(() => {
    onRestoreRail();
  });
</script>

<div class="fixed inset-0 z-50 flex items-center justify-center"
  style="background: rgba(0,0,0,0.5);">
  <div style="color:white; font-size:18px;">
    Flow Canvas — coming soon
    <button onclick={onClose} style="margin-left:16px; padding:4px 8px; background:#6366f1; border:none; border-radius:4px; color:white; cursor:pointer;">
      Close
    </button>
  </div>
</div>
```

- [ ] **Step 2: Read content-panel.svelte** — check the imports block and the `sectionId` derived variable (lines 1–80)

- [ ] **Step 3: Modify content-panel.svelte**

Add import at top of `<script>`:
```typescript
import Canvas01 from '$lib/components/shared-ui/canvas-01.svelte';
```

Add a prop for rail collapse/restore callbacks (these come from the page):
```typescript
let {
  ..., // existing props
  onCollapseRail = () => {},
  onRestoreRail  = () => {},
}: {
  ...,
  onCollapseRail?: () => void;
  onRestoreRail?:  () => void;
} = $props();
```

In the `<main>` block, before the existing `{#if isDashboard}` block, add:
```svelte
{#if sectionId === 'flow'}
  <Canvas01
    {darkMode}
    onClose={() => { /* handled by parent */ }}
    {onCollapseRail}
    {onRestoreRail}
  />
{:else}
```
And close with `{/if}` after the existing closing `</div>` of the main content block.

> **Note:** The `onClose` for Canvas01 should navigate the active menu back. This is wired in Task 12 when canvas-01 has proper state. For now, pass a no-op.

- [ ] **Step 4: Read `home3/+page.svelte`** — find where `railWidth`, `railPinned`, `railExpanded` are defined and how they're passed to components

- [ ] **Step 5: Add rail collapse/restore to home3/+page.svelte**

In `home3/+page.svelte`, add state variables for storing previous rail state, and add callback functions:
```typescript
// Store previous rail state when canvas opens
let prevRailWidth   = $state(240);
let prevRailPinned  = $state(false);

function handleCollapseRail() {
  prevRailWidth  = railWidth;
  prevRailPinned = railPinned;
  railWidth    = 56;
  railPinned   = false;
  railExpanded = false;
}

function handleRestoreRail() {
  railWidth    = prevRailWidth;
  railPinned   = prevRailPinned;
  railExpanded = prevRailPinned;
}
```

Pass these to `<ContentPanel>`:
```svelte
<ContentPanel
  ...
  onCollapseRail={handleCollapseRail}
  onRestoreRail={handleRestoreRail}
/>
```

- [ ] **Step 6: Type-check**

```bash
cd /Users/cding/Workspace/ChenWeb/web && bun run check
```
Expected: 0 errors

- [ ] **Step 7: Manual smoke test** — start the dev server, navigate to home3, click Tools → Flow

```bash
cd /Users/cding/Workspace/ChenWeb/web && bun run dev
```
Expected: clicking Flow shows "Flow Canvas — coming soon" overlay; clicking Close dismisses it; nav-rail should collapse to 56px while canvas is open

- [ ] **Step 8: Commit**

```bash
git add web/src/lib/components/shared-ui/canvas-01.svelte \
        web/src/lib/components/home3/content-panel.svelte \
        web/src/routes/home3/+page.svelte
git commit -m "feat(flow): wire canvas-01 skeleton into home3"
```

---

### Task 12: FlowCard component

**Files:**
- Create: `web/src/lib/components/shared-ui/canvas/FlowCard.svelte`

- [ ] **Step 1: Create FlowCard.svelte**

```svelte
<!-- web/src/lib/components/shared-ui/canvas/FlowCard.svelte -->
<script lang="ts">
  import type { Flow } from '$lib/types/flow';

  let {
    flow,
    selected   = false,
    currentUserId,
    onOpen,
    onSetDefault,
    onSaveAsTemplate,
    onDuplicate,
    onDelete,
  }: {
    flow: Flow;
    selected?: boolean;
    currentUserId: number;
    onOpen:          (f: Flow) => void;
    onSetDefault:    (f: Flow) => void;
    onSaveAsTemplate:(f: Flow) => void;
    onDuplicate:     (f: Flow) => void;
    onDelete:        (f: Flow) => void;
  } = $props();

  const isOwner = $derived(flow.user_id === currentUserId);

  let showMenu = $state(false);
  let menuX    = $state(0);
  let menuY    = $state(0);

  function openContextMenu(e: MouseEvent) {
    e.preventDefault();
    menuX = e.clientX;
    menuY = e.clientY;
    showMenu = true;
  }
</script>

<svelte:window onclick={() => { showMenu = false; }} />

<!-- Card -->
<button
  class="w-full text-left rounded-lg border transition-all duration-150 cursor-pointer overflow-hidden"
  style="background: {selected ? '#1e2535' : '#161b27'}; border-color: {selected ? '#6366f1' : '#1e2a3a'}; padding:0;"
  onclick={() => onOpen(flow)}
  oncontextmenu={openContextMenu}
  aria-label="Open flow {flow.flow_name}"
>
  <!-- Thumbnail -->
  <div class="w-full flex items-center justify-center overflow-hidden"
    style="height:64px; background:#0d1117; border-bottom:1px solid #1e2a3a;">
    {#if flow.thumbnail_svg}
      {@html flow.thumbnail_svg}
    {:else}
      <span style="font-size:11px; color:#4b5563;">No preview</span>
    {/if}
  </div>
  <!-- Meta -->
  <div style="padding:8px 10px;">
    <div style="font-size:12px; font-weight:500; color:#e2e8f0; margin-bottom:3px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">
      {flow.flow_name}
      {#if flow.is_default}<span style="font-size:9px; color:#fbbf24; margin-left:4px;">★ default</span>{/if}
    </div>
    <div style="font-size:10px; color:#6b7280;">
      {flow.flow_data?.nodes?.length ?? 0} nodes ·
      {flow.is_shared ? '🌐 Shared' : '🔒 Private'}
    </div>
  </div>
</button>

<!-- Context menu -->
{#if showMenu}
  <div
    class="fixed z-50 rounded-lg shadow-xl overflow-hidden"
    style="left:{menuX}px; top:{menuY}px; background:#1e2535; border:1px solid #374151; min-width:160px;"
    role="menu"
  >
    <button class="w-full text-left px-3 py-2 text-sm hover:bg-white/5" style="color:#e2e8f0; border:none; cursor:pointer;" onclick={() => { onOpen(flow); showMenu=false; }}>Open</button>
    {#if isOwner}
      <button class="w-full text-left px-3 py-2 text-sm hover:bg-white/5" style="color:#e2e8f0; border:none; cursor:pointer;" onclick={() => { onSetDefault(flow); showMenu=false; }}>Set as default</button>
      <button class="w-full text-left px-3 py-2 text-sm hover:bg-white/5" style="color:#e2e8f0; border:none; cursor:pointer;" onclick={() => { onSaveAsTemplate(flow); showMenu=false; }}>Save as template</button>
    {/if}
    <button class="w-full text-left px-3 py-2 text-sm hover:bg-white/5" style="color:#e2e8f0; border:none; cursor:pointer;" onclick={() => { onDuplicate(flow); showMenu=false; }}>Duplicate</button>
    {#if isOwner}
      <button class="w-full text-left px-3 py-2 text-sm hover:bg-white/5" style="color:#ef4444; border:none; cursor:pointer;" onclick={() => { onDelete(flow); showMenu=false; }}>Delete</button>
    {/if}
  </div>
{/if}
```

- [ ] **Step 2: Type-check**

```bash
cd /Users/cding/Workspace/ChenWeb/web && bun run check
```
Expected: 0 errors

- [ ] **Step 3: Commit**

```bash
git add web/src/lib/components/shared-ui/canvas/FlowCard.svelte
git commit -m "feat(flow): add FlowCard component"
```

---

### Task 13: FlowPicker modal

**Files:**
- Create: `web/src/lib/components/shared-ui/canvas/FlowPicker.svelte`

- [ ] **Step 1: Create FlowPicker.svelte**

```svelte
<!-- web/src/lib/components/shared-ui/canvas/FlowPicker.svelte -->
<script lang="ts">
  import { flowService, type FlowScope } from '$lib/services/flowService';
  import type { Flow } from '$lib/types/flow';
  import FlowCard from './FlowCard.svelte';

  let {
    darkMode       = true,
    currentUserId,
    onOpen,
    onNewEmpty,
    onClose,
  }: {
    darkMode?:     boolean;
    currentUserId: number;
    onOpen:     (flow: Flow) => void;
    onNewEmpty: () => void;
    onClose:    () => void;
  } = $props();

  type Tab = 'mine' | 'shared' | 'templates';
  let activeTab = $state<Tab>('mine');
  let flows     = $state<Flow[]>([]);
  let loading   = $state(false);
  let search    = $state('');
  let error     = $state('');

  const filtered = $derived(
    flows.filter(f =>
      f.flow_name.toLowerCase().includes(search.toLowerCase())
    )
  );

  async function loadFlows(scope: Tab) {
    loading = true;
    error   = '';
    try {
      const res = await flowService.list(scope as FlowScope);
      flows = res.flows ?? [];
    } catch (e: any) {
      error = e?.error?.message ?? 'Failed to load flows';
      flows = [];
    } finally {
      loading = false;
    }
  }

  $effect(() => { loadFlows(activeTab); });

  async function handleSetDefault(flow: Flow) {
    try { await flowService.setDefault(flow.flow_id); await loadFlows(activeTab); }
    catch (e: any) { error = e?.error?.message ?? 'Failed to set default'; }
  }

  async function handleSaveAsTemplate(flow: Flow) {
    try { await flowService.saveAsTemplate(flow.flow_id); await loadFlows(activeTab); }
    catch (e: any) { error = e?.error?.message ?? 'Failed to save as template'; }
  }

  async function handleDuplicate(flow: Flow) {
    try {
      const res = await flowService.fork(flow.flow_id);
      onOpen(res.flow);
    } catch (e: any) { error = e?.error?.message ?? 'Failed to duplicate'; }
  }

  async function handleDelete(flow: Flow) {
    if (!confirm(`Delete "${flow.flow_name}"?`)) return;
    try { await flowService.delete(flow.flow_id); await loadFlows(activeTab); }
    catch (e: any) { error = e?.error?.message ?? 'Failed to delete'; }
  }
</script>

<!-- Backdrop -->
<div class="fixed inset-0 z-50 flex items-center justify-center"
  style="background: rgba(0,0,0,0.7);"
  onclick={(e) => { if (e.target === e.currentTarget) onClose(); }}
  role="dialog" aria-modal="true" aria-label="Open a Flow"
>
  <div class="rounded-xl overflow-hidden shadow-2xl"
    style="background:#111827; border:1px solid #1e2a3a; width:680px; max-width:95vw; max-height:80vh; display:flex; flex-direction:column;">

    <!-- Header -->
    <div style="padding:16px 20px; border-bottom:1px solid #1e2a3a; display:flex; justify-content:space-between; align-items:flex-start;">
      <div>
        <div style="font-size:15px; font-weight:600; color:#e2e8f0;">Open a Flow</div>
        <div style="font-size:11px; color:#6b7280; margin-top:2px;">Select a flow, start from a template, or open a blank canvas</div>
      </div>
      <button onclick={onClose} style="color:#6b7280; font-size:16px; background:none; border:none; cursor:pointer;" aria-label="Close">✕</button>
    </div>

    <!-- Tabs -->
    <div style="display:flex; border-bottom:1px solid #1e2a3a; padding:0 20px;">
      {#each (['mine','shared','templates'] as const) as tab}
        <button
          style="padding:10px 16px; font-size:11px; border:none; background:none; cursor:pointer;
            color:{activeTab===tab ? '#6366f1' : '#6b7280'};
            border-bottom:{activeTab===tab ? '2px solid #6366f1' : '2px solid transparent'};"
          onclick={() => { activeTab = tab; search = ''; }}
        >
          {tab === 'mine' ? 'My Flows' : tab === 'shared' ? 'Shared Flows' : 'Templates'}
        </button>
      {/each}
    </div>

    <!-- Search + New -->
    <div style="padding:12px 20px; display:flex; gap:8px; align-items:center;">
      <input
        bind:value={search}
        placeholder="Search flows..."
        style="flex:1; background:#1e2535; border:1px solid #374151; border-radius:6px; padding:6px 12px; font-size:11px; color:#94a3b8;"
      />
      <button onclick={onNewEmpty}
        style="background:#6366f1; border-radius:6px; padding:6px 12px; font-size:11px; color:white; border:none; cursor:pointer; white-space:nowrap;">
        + New Empty Flow
      </button>
    </div>

    <!-- Error -->
    {#if error}
      <div style="margin:0 20px 8px; padding:8px 12px; background:#1f1313; border:1px solid #ef4444; border-radius:6px; font-size:11px; color:#ef4444;">
        {error}
      </div>
    {/if}

    <!-- Grid -->
    <div style="flex:1; overflow-y:auto; padding:0 20px 20px;">
      {#if loading}
        <div style="text-align:center; padding:40px; color:#6b7280; font-size:12px;">Loading...</div>
      {:else if filtered.length === 0}
        <div style="text-align:center; padding:40px; color:#6b7280; font-size:12px;">
          {search ? 'No flows match your search.' : 'No flows yet.'}
        </div>
      {:else}
        <div style="display:grid; grid-template-columns:repeat(3,1fr); gap:10px; padding-top:4px;">
          {#each filtered as flow (flow.flow_id)}
            <FlowCard
              {flow}
              {currentUserId}
              onOpen={(f) => onOpen(f)}
              onSetDefault={handleSetDefault}
              onSaveAsTemplate={handleSaveAsTemplate}
              onDuplicate={handleDuplicate}
              onDelete={handleDelete}
            />
          {/each}
        </div>
      {/if}
    </div>

    <!-- Footer -->
    <div style="padding:10px 20px; border-top:1px solid #1e2a3a; display:flex; justify-content:space-between; align-items:center;">
      <span style="font-size:10px; color:#4b5563;">Right-click a card for more options</span>
      <button onclick={onClose} style="background:#1e2535; border-radius:6px; padding:5px 12px; font-size:11px; color:#94a3b8; border:none; cursor:pointer;">Cancel</button>
    </div>
  </div>
</div>
```

- [ ] **Step 2: Type-check**

```bash
cd /Users/cding/Workspace/ChenWeb/web && bun run check
```
Expected: 0 errors

- [ ] **Step 3: Commit**

```bash
git add web/src/lib/components/shared-ui/canvas/FlowPicker.svelte
git commit -m "feat(flow): add FlowPicker modal"
```

---

### Task 14: FlowToolbar component

**Files:**
- Create: `web/src/lib/components/shared-ui/canvas/FlowToolbar.svelte`

- [ ] **Step 1: Create FlowToolbar.svelte**

```svelte
<!-- web/src/lib/components/shared-ui/canvas/FlowToolbar.svelte -->
<script lang="ts">
  import type { Flow } from '$lib/types/flow';
  import Undo2Icon        from '@lucide/svelte/icons/undo-2';
  import Redo2Icon        from '@lucide/svelte/icons/redo-2';
  import ZoomInIcon       from '@lucide/svelte/icons/zoom-in';
  import MaximizeIcon     from '@lucide/svelte/icons/maximize';
  import SaveIcon         from '@lucide/svelte/icons/save';
  import PlayIcon         from '@lucide/svelte/icons/play';
  import XIcon            from '@lucide/svelte/icons/x';
  import ChevronDownIcon  from '@lucide/svelte/icons/chevron-down';
  import BookmarkPlusIcon from '@lucide/svelte/icons/bookmark-plus';

  let {
    activeFlow,
    isDirty,
    canUndo,
    canRedo,
    darkMode = true,
    onPickerOpen,
    onRename,
    onUndo,
    onRedo,
    onFitView,
    onSave,
    onSaveAsTemplate,
    onClose,
  }: {
    activeFlow:       import('$lib/types/flow').Flow | null;
    isDirty:          boolean;
    canUndo:          boolean;
    canRedo:          boolean;
    darkMode?:        boolean;
    onPickerOpen:     () => void;
    onRename:         (name: string) => void;
    onUndo:           () => void;
    onRedo:           () => void;
    onFitView:        () => void;
    onSave:           () => void;
    onSaveAsTemplate: () => void;
    onClose:          () => void;
  } = $props();

  let editingName = $state(false);
  let nameInput   = $state('');

  function startRename() {
    nameInput   = activeFlow?.flow_name ?? '';
    editingName = true;
  }
  function commitRename() {
    const trimmed = nameInput.trim().slice(0, 255);
    if (trimmed && trimmed !== activeFlow?.flow_name) onRename(trimmed);
    editingName = false;
  }
</script>

<div class="flex items-center gap-2 px-3 flex-shrink-0"
  style="height:44px; background:#161b27; border-bottom:1px solid #1e2a3a; z-index:10;">

  <!-- Flow name + picker toggle -->
  <div class="flex items-center gap-1 flex-1 min-w-0">
    <div class="w-5 h-5 rounded flex-shrink-0" style="background:#6366f1;"></div>
    {#if editingName}
      <input
        bind:value={nameInput}
        onblur={commitRename}
        onkeydown={(e) => { if (e.key === 'Enter') commitRename(); if (e.key === 'Escape') editingName=false; }}
        class="rounded px-2 py-1"
        style="background:#0d1117; border:1px solid #6366f1; color:#e2e8f0; font-size:12px; max-width:200px;"
        autofocus
        maxlength="255"
      />
    {:else}
      <button
        onclick={startRename}
        class="rounded px-2 py-1 flex items-center gap-1 hover:bg-white/5 transition-colors"
        style="background:#1e2535; color:#e2e8f0; font-size:12px; border:none; cursor:pointer; max-width:200px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;"
        title="Click to rename"
      >
        {activeFlow?.flow_name ?? 'Untitled Flow'}
        {#if isDirty}<span style="color:#f59e0b; font-size:10px;">•</span>{/if}
      </button>
    {/if}
    <button
      onclick={onPickerOpen}
      title="Open flow picker"
      style="background:#1e2535; border:none; border-radius:4px; padding:3px 5px; cursor:pointer; color:#6b7280; display:flex; align-items:center;"
    >
      <ChevronDownIcon class="w-3 h-3" />
    </button>
  </div>

  <!-- Divider -->
  <div style="width:1px; height:20px; background:#374151;"></div>

  <!-- Undo / Redo -->
  <button onclick={onUndo} disabled={!canUndo} title="Undo" class="toolbar-btn" aria-label="Undo">
    <Undo2Icon class="w-4 h-4" />
  </button>
  <button onclick={onRedo} disabled={!canRedo} title="Redo" class="toolbar-btn" aria-label="Redo">
    <Redo2Icon class="w-4 h-4" />
  </button>

  <div style="width:1px; height:20px; background:#374151;"></div>

  <!-- Zoom / Fit -->
  <button onclick={onFitView} title="Fit view" class="toolbar-btn" aria-label="Fit view">
    <MaximizeIcon class="w-4 h-4" />
  </button>

  <div style="width:1px; height:20px; background:#374151;"></div>

  <!-- Save / Template / Run -->
  <button onclick={onSave} title="Save" class="toolbar-btn" style="color:{isDirty ? '#fbbf24' : ''}" aria-label="Save">
    <SaveIcon class="w-4 h-4" />
    <span style="font-size:10px; margin-left:3px;">Save</span>
  </button>
  <button onclick={onSaveAsTemplate} title="Save as template" class="toolbar-btn" aria-label="Save as template">
    <BookmarkPlusIcon class="w-4 h-4" />
  </button>
  <button title="Run (not yet implemented)" class="toolbar-btn" style="background:#6366f1; color:white; border-radius:4px; padding:4px 10px;" aria-label="Run">
    <PlayIcon class="w-4 h-4" />
    <span style="font-size:10px; margin-left:3px;">Run</span>
  </button>

  <div style="width:1px; height:20px; background:#374151;"></div>

  <!-- Close -->
  <button onclick={onClose} title="Close canvas" class="toolbar-btn" aria-label="Close">
    <XIcon class="w-4 h-4" />
  </button>
</div>

<style>
  .toolbar-btn {
    display: flex; align-items: center;
    background: none; border: none; cursor: pointer;
    color: #94a3b8; border-radius: 4px; padding: 4px 6px;
    transition: background 0.1s;
  }
  .toolbar-btn:hover:not(:disabled) { background: rgba(255,255,255,0.05); }
  .toolbar-btn:disabled { opacity: 0.35; cursor: not-allowed; }
</style>
```

- [ ] **Step 2: Type-check**

```bash
cd /Users/cding/Workspace/ChenWeb/web && bun run check
```
Expected: 0 errors

- [ ] **Step 3: Commit**

```bash
git add web/src/lib/components/shared-ui/canvas/FlowToolbar.svelte
git commit -m "feat(flow): add FlowToolbar component"
```

---

## Chunk 3: Palette, Properties Panel, Node Components, Full Integration

### Task 15: NodePalette component

**Files:**
- Create: `web/src/lib/components/shared-ui/canvas/NodePalette.svelte`

- [ ] **Step 1: Create NodePalette.svelte**

```svelte
<!-- web/src/lib/components/shared-ui/canvas/NodePalette.svelte -->
<script lang="ts">
  import type { NodeType } from '$lib/types/flow';

  let {
    nodeTypes  = [],
    darkMode   = true,
    onDragStart,
  }: {
    nodeTypes:   NodeType[];
    darkMode?:   boolean;
    onDragStart: (event: DragEvent, nodeType: NodeType) => void;
  } = $props();

  let search = $state('');

  const categories = $derived(
    [...new Set(nodeTypes.map(n => n.category))]
  );

  function filteredByCategory(cat: string) {
    return nodeTypes.filter(n =>
      n.category === cat &&
      n.label.toLowerCase().includes(search.toLowerCase())
    );
  }
</script>

<aside
  class="flex flex-col h-full overflow-hidden flex-shrink-0"
  style="width:200px; background:#111827; border-right:1px solid #1e2a3a;"
>
  <div style="padding:8px; border-bottom:1px solid #1e2a3a;">
    <div style="font-size:9px; color:#6366f1; letter-spacing:1px; margin-bottom:6px;">NODE PALETTE</div>
    <input
      bind:value={search}
      placeholder="Search nodes..."
      style="width:100%; background:#1e2535; border:1px solid #374151; border-radius:4px; padding:4px 8px; font-size:10px; color:#94a3b8; box-sizing:border-box;"
    />
  </div>

  <div class="flex-1 overflow-y-auto" style="padding:8px; scrollbar-width:thin; scrollbar-color:#374151 transparent;">
    {#each categories as cat}
      {@const items = filteredByCategory(cat)}
      {#if items.length > 0}
        <div style="font-size:8px; color:#4b5563; letter-spacing:1px; margin:8px 0 4px 4px;">{cat.toUpperCase()}</div>
        {#each items as nodeType (nodeType.id)}
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <div
            draggable="true"
            ondragstart={(e) => onDragStart(e, nodeType)}
            class="flex items-center gap-2 rounded px-2 py-1.5 mb-1 cursor-grab hover:bg-white/5 transition-colors select-none"
            style="background:#1e2535; border:1px solid transparent; font-size:10px; color:#e2e8f0;"
            title="Drag onto canvas"
          >
            <span style="font-size:14px; flex-shrink:0;">{getIcon(nodeType.icon)}</span>
            {nodeType.label}
          </div>
        {/each}
      {/if}
    {/each}
  </div>
</aside>

<script module lang="ts">
  // Simple icon emoji map — replace with lucide icons if desired
  const ICON_MAP: Record<string, string> = {
    Bot: '🤖', Terminal: '💻', Type: '📝', FileText: '📄',
    File: '📁', Image: '🎬', Wrench: '🔧', Plug: '🔌',
    Globe: '📡', Filter: '📋', GitBranch: '🗂',
  };
  export function getIcon(name: string) { return ICON_MAP[name] ?? '⬡'; }
</script>
```

- [ ] **Step 2: Type-check**

```bash
cd /Users/cding/Workspace/ChenWeb/web && bun run check
```
Expected: 0 errors

- [ ] **Step 3: Commit**

```bash
git add web/src/lib/components/shared-ui/canvas/NodePalette.svelte
git commit -m "feat(flow): add NodePalette component"
```

---

### Task 16: PropertiesPanel component

**Files:**
- Create: `web/src/lib/components/shared-ui/canvas/PropertiesPanel.svelte`

- [ ] **Step 1: Create PropertiesPanel.svelte**

```svelte
<!-- web/src/lib/components/shared-ui/canvas/PropertiesPanel.svelte -->
<script lang="ts">
  import type { FlowNode, NodeType } from '$lib/types/flow';

  let {
    node,
    nodeType,
    darkMode = true,
    onUpdate,
  }: {
    node:      FlowNode | null;
    nodeType:  NodeType | null;
    darkMode?: boolean;
    onUpdate:  (nodeId: string, data: Record<string, unknown>) => void;
  } = $props();

  // Map of attribute → UI control type
  const SELECT_ATTRS: Record<string, string[]> = {
    model:      ['gpt-4o', 'gpt-4o-mini', 'claude-opus-4-6', 'claude-sonnet-4-6', 'claude-haiku-4-5'],
    language:   ['typescript', 'python', 'go', 'rust', 'java', 'c++', 'bash'],
    file_type:  ['txt', 'csv', 'json', 'pdf', 'md', 'html'],
    media_type: ['image', 'audio', 'video'],
    method:     ['GET', 'POST', 'PUT', 'DELETE', 'PATCH'],
    operation:  ['status', 'clone', 'pull', 'push', 'commit', 'diff'],
  };
  const MASKED_ATTRS  = new Set(['auth_token']);
  const TEXTAREA_ATTRS = new Set(['system_prompt', 'content', 'tool_config', 'headers', 'rule_expression']);

  function update(key: string, value: unknown) {
    if (!node) return;
    onUpdate(node.id, { ...node.data, [key]: value });
  }
</script>

<aside
  class="flex flex-col h-full overflow-hidden flex-shrink-0"
  style="width:240px; background:#111827; border-left:1px solid #1e2a3a;"
>
  {#if !node || !nodeType}
    <div class="flex-1 flex items-center justify-center" style="color:#4b5563; font-size:12px;">
      Select a node to edit its properties
    </div>
  {:else}
    <div style="padding:12px 12px 0; border-bottom:1px solid #1e2a3a;">
      <div style="font-size:9px; color:#6366f1; letter-spacing:1px; margin-bottom:8px;">PROPERTIES</div>
      <!-- Node name -->
      <div style="margin-bottom:10px;">
        <label style="font-size:9px; color:#6b7280; display:block; margin-bottom:3px;">Name</label>
        <input
          type="text"
          maxlength="100"
          value={String(node.data.label ?? nodeType.label)}
          oninput={(e) => update('label', (e.target as HTMLInputElement).value)}
          style="width:100%; background:#1e2535; border:1px solid #374151; border-radius:4px; padding:4px 8px; font-size:10px; color:#e2e8f0; box-sizing:border-box;"
        />
      </div>
    </div>

    <div class="flex-1 overflow-y-auto" style="padding:10px 12px; scrollbar-width:thin; scrollbar-color:#374151 transparent;">
      <!-- Attributes -->
      {#each Object.entries(nodeType.defaultData) as [key]}
        <div style="margin-bottom:10px;">
          <label style="font-size:9px; color:#6b7280; display:block; margin-bottom:3px; text-transform:capitalize;">
            {key.replace(/_/g, ' ')}
          </label>
          {#if SELECT_ATTRS[key]}
            <select
              value={String(node.data[key] ?? nodeType.defaultData[key])}
              onchange={(e) => update(key, (e.target as HTMLSelectElement).value)}
              style="width:100%; background:#1e2535; border:1px solid #374151; border-radius:4px; padding:4px 8px; font-size:10px; color:#e2e8f0;"
            >
              {#each SELECT_ATTRS[key] as opt}
                <option value={opt}>{opt}</option>
              {/each}
            </select>
          {:else if key === 'temperature'}
            <div class="flex items-center gap-2">
              <input type="range" min="0" max="2" step="0.1"
                value={Number(node.data[key] ?? nodeType.defaultData[key])}
                oninput={(e) => update(key, parseFloat((e.target as HTMLInputElement).value))}
                style="flex:1;"
              />
              <span style="font-size:10px; color:#94a3b8; min-width:24px;">{node.data[key] ?? nodeType.defaultData[key]}</span>
            </div>
          {:else if TEXTAREA_ATTRS.has(key)}
            <textarea
              rows="4"
              value={String(node.data[key] ?? nodeType.defaultData[key] ?? '')}
              oninput={(e) => update(key, (e.target as HTMLTextAreaElement).value)}
              style="width:100%; background:#1e2535; border:1px solid #374151; border-radius:4px; padding:4px 8px; font-size:10px; color:#e2e8f0; resize:vertical; box-sizing:border-box;"
            ></textarea>
          {:else if MASKED_ATTRS.has(key)}
            <input type="password"
              value={String(node.data[key] ?? '')}
              oninput={(e) => update(key, (e.target as HTMLInputElement).value)}
              style="width:100%; background:#1e2535; border:1px solid #374151; border-radius:4px; padding:4px 8px; font-size:10px; color:#e2e8f0; box-sizing:border-box;"
            />
          {:else}
            <input type="text"
              value={String(node.data[key] ?? '')}
              oninput={(e) => update(key, (e.target as HTMLInputElement).value)}
              style="width:100%; background:#1e2535; border:1px solid #374151; border-radius:4px; padding:4px 8px; font-size:10px; color:#e2e8f0; box-sizing:border-box;"
            />
          {/if}
        </div>
      {/each}

      <!-- Connectors (read-only) -->
      <div style="margin-top:8px;">
        <div style="font-size:9px; color:#6b7280; margin-bottom:6px;">CONNECTORS</div>
        {#each nodeType.inputs as inp}
          <div style="background:#1e2535; border-radius:4px; padding:4px 8px; margin-bottom:3px; font-size:9px; color:#818cf8;">← in: {inp}</div>
        {/each}
        {#each nodeType.outputs as out}
          <div style="background:#1e2535; border-radius:4px; padding:4px 8px; margin-bottom:3px; font-size:9px; color:#6366f1;">→ out: {out}</div>
        {/each}
      </div>
    </div>
  {/if}
</aside>
```

- [ ] **Step 2: Type-check**

```bash
cd /Users/cding/Workspace/ChenWeb/web && bun run check
```
Expected: 0 errors

- [ ] **Step 3: Commit**

```bash
git add web/src/lib/components/shared-ui/canvas/PropertiesPanel.svelte
git commit -m "feat(flow): add PropertiesPanel component"
```

---

### Task 17: BaseNode component

**Files:**
- Create: `web/src/lib/components/shared-ui/canvas/nodes/BaseNode.svelte`

- [ ] **Step 1: Create BaseNode.svelte**

```svelte
<!-- web/src/lib/components/shared-ui/canvas/nodes/BaseNode.svelte -->
<script lang="ts">
  import { Handle, Position } from '@xyflow/svelte';
  import CopyIcon   from '@lucide/svelte/icons/copy';
  import Trash2Icon from '@lucide/svelte/icons/trash-2';
  import SlidersIcon from '@lucide/svelte/icons/sliders-horizontal';

  let {
    id,
    selected = false,
    label,
    icon         = '⬡',
    inputHandles = [],
    outputHandles= [],
    children,
    onConfigure,
    onDuplicate,
    onDelete,
  }: {
    id:            string;
    selected?:     boolean;
    label:         string;
    icon?:         string;
    inputHandles:  string[];
    outputHandles: string[];
    children?:     import('svelte').Snippet;
    onConfigure:   () => void;
    onDuplicate:   () => void;
    onDelete:      () => void;
  } = $props();
</script>

<div
  class="relative rounded-lg overflow-visible"
  style="background:#161b27; border:2px solid {selected ? '#6366f1' : '#374151'};
    box-shadow:{selected ? '0 0 0 3px rgba(99,102,241,0.2)' : 'none'};
    min-width:160px;"
>
  <!-- Mini toolbar (only when selected) -->
  {#if selected}
    <div
      class="absolute flex gap-1"
      style="top:-30px; left:50%; transform:translateX(-50%); background:#1e2535; border:1px solid #374151; border-radius:6px; padding:3px 6px; white-space:nowrap; z-index:10;"
    >
      <button onclick={onConfigure} title="Configure" style="background:none; border:none; cursor:pointer; color:#94a3b8; padding:1px 3px; border-radius:3px; display:flex; align-items:center;" aria-label="Configure">
        <SlidersIcon class="w-3 h-3" />
      </button>
      <button onclick={onDuplicate} title="Duplicate" style="background:none; border:none; cursor:pointer; color:#94a3b8; padding:1px 3px; border-radius:3px; display:flex; align-items:center;" aria-label="Duplicate">
        <CopyIcon class="w-3 h-3" />
      </button>
      <button onclick={onDelete} title="Delete" style="background:none; border:none; cursor:pointer; color:#ef4444; padding:1px 3px; border-radius:3px; display:flex; align-items:center;" aria-label="Delete">
        <Trash2Icon class="w-3 h-3" />
      </button>
    </div>
  {/if}

  <!-- Input handles -->
  {#each inputHandles as handleId, i}
    <Handle
      type="target"
      position={Position.Left}
      id={`in-${handleId}`}
      style="top:{inputHandles.length === 1 ? '50%' : `${(i + 1) / (inputHandles.length + 1) * 100}%`}; background:#818cf8; border:2px solid #161b27; width:14px; height:14px;"
    />
  {/each}

  <!-- Header -->
  <div style="padding:6px 10px; border-bottom:1px solid #1e2a3a; display:flex; align-items:center; gap:6px; font-size:10px; color:{selected ? '#e2e8f0' : '#94a3b8'};">
    <span style="font-size:13px;">{icon}</span>
    <span style="font-weight:500; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">{label}</span>
  </div>

  <!-- Body (slot for node-specific content) -->
  {#if children}
    {@render children()}
  {/if}

  <!-- Output handles -->
  {#each outputHandles as handleId, i}
    <Handle
      type="source"
      position={Position.Right}
      id={`out-${handleId}`}
      style="top:{outputHandles.length === 1 ? '50%' : `${(i + 1) / (outputHandles.length + 1) * 100}%`}; background:#6366f1; border:2px solid #161b27; width:14px; height:14px;"
    />
  {/each}
</div>
```

- [ ] **Step 2: Type-check**

```bash
cd /Users/cding/Workspace/ChenWeb/web && bun run check
```
Expected: 0 errors

- [ ] **Step 3: Commit**

```bash
git add web/src/lib/components/shared-ui/canvas/nodes/BaseNode.svelte
git commit -m "feat(flow): add BaseNode component"
```

---

### Task 18: All 11 node components

Each node component is a thin wrapper around BaseNode. They all follow this pattern — the only differences are the icon emoji, input/output handle names, and the body content preview.

**Files to create** (one step each):

- `nodes/AiAssistantNode.svelte`
- `nodes/TextNode.svelte`
- `nodes/FileNode.svelte`
- `nodes/DocumentNode.svelte`
- `nodes/MediaNode.svelte`
- `nodes/ToolNode.svelte`
- `nodes/McpNode.svelte`
- `nodes/HttpRequestNode.svelte`
- `nodes/RuleNode.svelte`
- `nodes/CodingAssistantNode.svelte`
- `nodes/GitNode.svelte`

- [ ] **Step 1: Create AiAssistantNode.svelte**

```svelte
<!-- web/src/lib/components/shared-ui/canvas/nodes/AiAssistantNode.svelte -->
<script lang="ts">
  import BaseNode from './BaseNode.svelte';
  let { id, data, selected }: { id: string; data: Record<string,any>; selected?: boolean } = $props();
</script>
<BaseNode {id} {selected} label={String(data.label ?? 'AI Assistant')} icon="🤖"
  inputHandles={['text','context']} outputHandles={['response']}
  onConfigure={() => {}} onDuplicate={() => {}} onDelete={() => {}}
>
  <div style="padding:6px 10px; font-size:9px; color:#6b7280;">
    {data.model ?? 'gpt-4o'}
    {#if data.system_prompt}<span style="margin-left:4px;color:#4b5563;">· prompt set</span>{/if}
  </div>
</BaseNode>
```

- [ ] **Step 2: Create TextNode.svelte**

```svelte
<!-- web/src/lib/components/shared-ui/canvas/nodes/TextNode.svelte -->
<script lang="ts">
  import BaseNode from './BaseNode.svelte';
  let { id, data, selected }: { id: string; data: Record<string,any>; selected?: boolean } = $props();
</script>
<BaseNode {id} {selected} label={String(data.label ?? 'Text')} icon="📝"
  inputHandles={[]} outputHandles={['text']}
  onConfigure={() => {}} onDuplicate={() => {}} onDelete={() => {}}
>
  <div style="padding:6px 10px; font-size:9px; color:#6b7280; max-width:180px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">
    {data.content ? String(data.content).slice(0,40) : 'No content yet'}
  </div>
</BaseNode>
```

- [ ] **Step 3: Create FileNode.svelte**

```svelte
<!-- web/src/lib/components/shared-ui/canvas/nodes/FileNode.svelte -->
<script lang="ts">
  import BaseNode from './BaseNode.svelte';
  let { id, data, selected }: { id: string; data: Record<string,any>; selected?: boolean } = $props();
</script>
<BaseNode {id} {selected} label={String(data.label ?? 'File')} icon="📁"
  inputHandles={[]} outputHandles={['file']}
  onConfigure={() => {}} onDuplicate={() => {}} onDelete={() => {}}
>
  <div style="padding:6px 10px; font-size:9px; color:#6b7280;">
    {data.file_type ?? 'txt'} · {data.file_path ? String(data.file_path).split('/').pop() : 'no file'}
  </div>
</BaseNode>
```

- [ ] **Step 4: Create DocumentNode.svelte**

```svelte
<!-- web/src/lib/components/shared-ui/canvas/nodes/DocumentNode.svelte -->
<script lang="ts">
  import BaseNode from './BaseNode.svelte';
  let { id, data, selected }: { id: string; data: Record<string,any>; selected?: boolean } = $props();
</script>
<BaseNode {id} {selected} label={String(data.label ?? 'Document')} icon="📄"
  inputHandles={[]} outputHandles={['document']}
  onConfigure={() => {}} onDuplicate={() => {}} onDelete={() => {}}
>
  <div style="padding:6px 10px; font-size:9px; color:#6b7280;">
    {data.doc_source ?? 'no source'}
  </div>
</BaseNode>
```

- [ ] **Step 5: Create MediaNode.svelte**

```svelte
<!-- web/src/lib/components/shared-ui/canvas/nodes/MediaNode.svelte -->
<script lang="ts">
  import BaseNode from './BaseNode.svelte';
  let { id, data, selected }: { id: string; data: Record<string,any>; selected?: boolean } = $props();
</script>
<BaseNode {id} {selected} label={String(data.label ?? 'Media')} icon="🎬"
  inputHandles={[]} outputHandles={['media']}
  onConfigure={() => {}} onDuplicate={() => {}} onDelete={() => {}}
>
  <div style="padding:6px 10px; font-size:9px; color:#6b7280;">
    {data.media_type ?? 'image'}
  </div>
</BaseNode>
```

- [ ] **Step 6: Create ToolNode.svelte**

```svelte
<!-- web/src/lib/components/shared-ui/canvas/nodes/ToolNode.svelte -->
<script lang="ts">
  import BaseNode from './BaseNode.svelte';
  let { id, data, selected }: { id: string; data: Record<string,any>; selected?: boolean } = $props();
</script>
<BaseNode {id} {selected} label={String(data.label ?? 'Tool')} icon="🔧"
  inputHandles={['args']} outputHandles={['result']}
  onConfigure={() => {}} onDuplicate={() => {}} onDelete={() => {}}
>
  <div style="padding:6px 10px; font-size:9px; color:#6b7280;">
    {data.tool_name ?? 'unnamed tool'}
  </div>
</BaseNode>
```

- [ ] **Step 7: Create McpNode.svelte**

```svelte
<!-- web/src/lib/components/shared-ui/canvas/nodes/McpNode.svelte -->
<script lang="ts">
  import BaseNode from './BaseNode.svelte';
  let { id, data, selected }: { id: string; data: Record<string,any>; selected?: boolean } = $props();
</script>
<BaseNode {id} {selected} label={String(data.label ?? 'MCP')} icon="🔌"
  inputHandles={['request']} outputHandles={['response']}
  onConfigure={() => {}} onDuplicate={() => {}} onDelete={() => {}}
>
  <div style="padding:6px 10px; font-size:9px; color:#6b7280; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; max-width:160px;">
    {data.server_url ?? 'no server'}
  </div>
</BaseNode>
```

- [ ] **Step 8: Create HttpRequestNode.svelte**

```svelte
<!-- web/src/lib/components/shared-ui/canvas/nodes/HttpRequestNode.svelte -->
<script lang="ts">
  import BaseNode from './BaseNode.svelte';
  let { id, data, selected }: { id: string; data: Record<string,any>; selected?: boolean } = $props();
</script>
<BaseNode {id} {selected} label={String(data.label ?? 'HTTP Request')} icon="📡"
  inputHandles={['body']} outputHandles={['response']}
  onConfigure={() => {}} onDuplicate={() => {}} onDelete={() => {}}
>
  <div style="padding:6px 10px; font-size:9px; color:#6b7280; display:flex; gap:4px; align-items:center;">
    <span style="background:#1e2535; border-radius:3px; padding:1px 5px; color:#94a3b8;">{data.method ?? 'GET'}</span>
    <span style="overflow:hidden; text-overflow:ellipsis; white-space:nowrap; max-width:110px;">{data.url ?? 'no URL'}</span>
  </div>
</BaseNode>
```

- [ ] **Step 9: Create RuleNode.svelte**

```svelte
<!-- web/src/lib/components/shared-ui/canvas/nodes/RuleNode.svelte -->
<script lang="ts">
  import BaseNode from './BaseNode.svelte';
  let { id, data, selected }: { id: string; data: Record<string,any>; selected?: boolean } = $props();
</script>
<BaseNode {id} {selected} label={String(data.label ?? 'Rule')} icon="📋"
  inputHandles={['data']} outputHandles={['pass','fail']}
  onConfigure={() => {}} onDuplicate={() => {}} onDelete={() => {}}
>
  <div style="padding:6px 10px; font-size:9px; color:#6b7280; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; max-width:160px;">
    {data.description || data.rule_expression || 'no rule'}
  </div>
</BaseNode>
```

- [ ] **Step 10: Create CodingAssistantNode.svelte**

```svelte
<!-- web/src/lib/components/shared-ui/canvas/nodes/CodingAssistantNode.svelte -->
<script lang="ts">
  import BaseNode from './BaseNode.svelte';
  let { id, data, selected }: { id: string; data: Record<string,any>; selected?: boolean } = $props();
</script>
<BaseNode {id} {selected} label={String(data.label ?? 'Coding Assistant')} icon="💻"
  inputHandles={['code','context']} outputHandles={['code','explanation']}
  onConfigure={() => {}} onDuplicate={() => {}} onDelete={() => {}}
>
  <div style="padding:6px 10px; font-size:9px; color:#6b7280;">
    {data.language ?? 'typescript'} · {data.model ?? 'gpt-4o'}
  </div>
</BaseNode>
```

- [ ] **Step 11: Create GitNode.svelte**

```svelte
<!-- web/src/lib/components/shared-ui/canvas/nodes/GitNode.svelte -->
<script lang="ts">
  import BaseNode from './BaseNode.svelte';
  let { id, data, selected }: { id: string; data: Record<string,any>; selected?: boolean } = $props();
</script>
<BaseNode {id} {selected} label={String(data.label ?? 'GIT')} icon="🗂"
  inputHandles={['files']} outputHandles={['result']}
  onConfigure={() => {}} onDuplicate={() => {}} onDelete={() => {}}
>
  <div style="padding:6px 10px; font-size:9px; color:#6b7280;">
    {data.operation ?? 'status'} · {data.branch ?? 'main'}
  </div>
</BaseNode>
```

- [ ] **Step 12: Type-check all nodes**

```bash
cd /Users/cding/Workspace/ChenWeb/web && bun run check
```
Expected: 0 errors

- [ ] **Step 13: Commit**

```bash
git add web/src/lib/components/shared-ui/canvas/nodes/
git commit -m "feat(flow): add all 11 node components"
```

---

### Task 19: Full canvas-01.svelte integration

**Files:**
- Modify: `web/src/lib/components/shared-ui/canvas-01.svelte`

This task replaces the skeleton with the complete implementation. Read the existing skeleton first.

- [ ] **Step 1: Replace canvas-01.svelte with full implementation**

```svelte
<!-- web/src/lib/components/shared-ui/canvas-01.svelte -->
<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import {
    SvelteFlow, Controls, MiniMap, Background, BackgroundVariant,
    type Node, type Edge,
  } from '@xyflow/svelte';
  import '@xyflow/svelte/dist/style.css';

  import { flowService }     from '$lib/services/flowService';
  import type { Flow, FlowData, NodeType, Snapshot } from '$lib/types/flow';

  import FlowToolbar      from './canvas/FlowToolbar.svelte';
  import FlowPicker       from './canvas/FlowPicker.svelte';
  import NodePalette      from './canvas/NodePalette.svelte';
  import PropertiesPanel  from './canvas/PropertiesPanel.svelte';

  // Node type components
  import AiAssistantNode    from './canvas/nodes/AiAssistantNode.svelte';
  import TextNode           from './canvas/nodes/TextNode.svelte';
  import FileNode           from './canvas/nodes/FileNode.svelte';
  import DocumentNode       from './canvas/nodes/DocumentNode.svelte';
  import MediaNode          from './canvas/nodes/MediaNode.svelte';
  import ToolNode           from './canvas/nodes/ToolNode.svelte';
  import McpNode            from './canvas/nodes/McpNode.svelte';
  import HttpRequestNode    from './canvas/nodes/HttpRequestNode.svelte';
  import RuleNode           from './canvas/nodes/RuleNode.svelte';
  import CodingAssistantNode from './canvas/nodes/CodingAssistantNode.svelte';
  import GitNode            from './canvas/nodes/GitNode.svelte';

  const NODE_TYPES = {
    'ai-assistant':     AiAssistantNode,
    'text':             TextNode,
    'file':             FileNode,
    'document':         DocumentNode,
    'media':            MediaNode,
    'tool':             ToolNode,
    'mcp':              McpNode,
    'http-request':     HttpRequestNode,
    'rule':             RuleNode,
    'coding-assistant': CodingAssistantNode,
    'git':              GitNode,
  };

  let {
    darkMode = true,
    onClose,
    onCollapseRail,
    onRestoreRail,
  }: {
    darkMode?:      boolean;
    onClose:        () => void;
    onCollapseRail: () => void;
    onRestoreRail:  () => void;
  } = $props();

  // ── Core state ────────────────────────────────────────────────────────
  let activeFlow        = $state<Flow | null>(null);
  let nodes             = $state<Node[]>([]);
  let edges             = $state<Edge[]>([]);
  let selectedNodeId    = $state<string | null>(null);
  let showPicker        = $state(false);
  let nodeTypes         = $state<NodeType[]>([]);
  let toastMsg          = $state('');
  let toastTimeout: ReturnType<typeof setTimeout> | null = null;

  // Undo / redo
  let undoStack         = $state<Snapshot[]>([]);
  let redoStack         = $state<Snapshot[]>([]);
  let lastSavedSnapshot = $state<Snapshot | null>(null);
  // Threshold warning flags (fire once)
  let warnedNodes       = $state(false);
  let warnedEdges       = $state(false);

  const MAX_UNDO = 50;

  const isDirty = $derived(
    lastSavedSnapshot !== null &&
    JSON.stringify({ nodes, edges }) !== JSON.stringify(lastSavedSnapshot)
  );
  const canUndo = $derived(undoStack.length > 0);
  const canRedo = $derived(redoStack.length > 0);

  const selectedNode = $derived(nodes.find(n => n.id === selectedNodeId) ?? null);
  const selectedNodeType = $derived(
    selectedNode ? (nodeTypes.find(t => t.id === selectedNode.type) ?? null) : null
  );

  // ── Toast ──────────────────────────────────────────────────────────────
  function toast(msg: string) {
    toastMsg = msg;
    if (toastTimeout) clearTimeout(toastTimeout);
    toastTimeout = setTimeout(() => { toastMsg = ''; }, 4000);
  }

  // ── Snapshot helpers ────────────────────────────────────────────────────
  function takeSnapshot() {
    const snap: Snapshot = { nodes: JSON.parse(JSON.stringify(nodes)), edges: JSON.parse(JSON.stringify(edges)) };
    undoStack = [snap, ...undoStack].slice(0, MAX_UNDO);
    redoStack = [];
  }

  function undo() {
    if (!undoStack.length) return;
    const [top, ...rest] = undoStack;
    redoStack = [{ nodes: JSON.parse(JSON.stringify(nodes)), edges: JSON.parse(JSON.stringify(edges)) }, ...redoStack];
    undoStack = rest;
    nodes = top.nodes;
    edges = top.edges;
  }

  function redo() {
    if (!redoStack.length) return;
    const [top, ...rest] = redoStack;
    undoStack = [{ nodes: JSON.parse(JSON.stringify(nodes)), edges: JSON.parse(JSON.stringify(edges)) }, ...undoStack].slice(0, MAX_UNDO);
    redoStack = rest;
    nodes = top.nodes;
    edges = top.edges;
  }

  // ── Load flow ───────────────────────────────────────────────────────────
  async function loadFlow(flow: Flow) {
    activeFlow = flow;
    nodes = (flow.flow_data?.nodes ?? []) as Node[];
    edges = (flow.flow_data?.edges ?? []) as Edge[];
    lastSavedSnapshot = { nodes: JSON.parse(JSON.stringify(nodes)), edges: JSON.parse(JSON.stringify(edges)) };
    undoStack = [];
    redoStack = [];
    warnedNodes = false;
    warnedEdges = false;
    showPicker = false;
  }

  async function loadDefaultOrPicker() {
    try {
      const res = await flowService.getDefault();
      await loadFlow(res.flow);
    } catch {
      showPicker = true;
    }
  }

  // ── Save ────────────────────────────────────────────────────────────────
  async function save() {
    if (!activeFlow) return;
    try {
      const res = await flowService.update(activeFlow.flow_id, {
        flow_name:     activeFlow.flow_name,
        flow_data:     { nodes: nodes as any, edges: edges as any },
        thumbnail_svg: generateThumbnail(),
      });
      activeFlow = res.flow;
      lastSavedSnapshot = { nodes: JSON.parse(JSON.stringify(nodes)), edges: JSON.parse(JSON.stringify(edges)) };
      toast('Saved');
    } catch {
      toast('Failed to save — your changes are preserved');
    }
  }

  // ── Thumbnail (simple SVG preview) ─────────────────────────────────────
  function generateThumbnail(): string {
    const W = 200, H = 120;
    const rects = nodes.slice(0, 8).map(n => {
      const x = Math.round((n.position.x / 2000) * W);
      const y = Math.round((n.position.y / 1200) * H);
      return `<rect x="${x}" y="${y}" width="28" height="16" rx="3" fill="#1e2535" stroke="#374151" stroke-width="0.5"/>`;
    }).join('');
    return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${W} ${H}" width="${W}" height="${H}" style="background:#0d1117;">${rects}</svg>`;
  }

  // ── Drag-and-drop from palette ──────────────────────────────────────────
  function onPaletteDragStart(event: DragEvent, nodeType: NodeType) {
    event.dataTransfer?.setData('application/xyflow-node-type', JSON.stringify(nodeType));
  }

  function onCanvasDrop(event: DragEvent) {
    event.preventDefault();
    const raw = event.dataTransfer?.getData('application/xyflow-node-type');
    if (!raw) return;
    const nodeType: NodeType = JSON.parse(raw);
    // @xyflow/svelte provides a way to convert screen coords — use a simple offset for now
    const bounds = (event.currentTarget as HTMLElement).getBoundingClientRect();
    const position = { x: event.clientX - bounds.left, y: event.clientY - bounds.top };
    const id = `${nodeType.id}-${Date.now()}`;
    takeSnapshot();
    nodes = [...nodes, { id, type: nodeType.id, position, data: { ...nodeType.defaultData, label: nodeType.label } }];
    selectedNodeId = id;
    checkLimits();
  }

  function checkLimits() {
    if (!warnedNodes && nodes.length > 500) { toast('Warning: over 500 nodes — performance may degrade'); warnedNodes = true; }
    if (!warnedEdges && edges.length > 1000) { toast('Warning: over 1000 edges — performance may degrade'); warnedEdges = true; }
  }

  // ── Close with guard ────────────────────────────────────────────────────
  function requestClose() {
    if (isDirty) {
      if (!confirm('You have unsaved changes. Discard and continue?')) return;
    }
    onRestoreRail();
    onClose();
  }

  // ── Node update from properties panel ───────────────────────────────────
  function updateNodeData(nodeId: string, data: Record<string, unknown>) {
    nodes = nodes.map(n => n.id === nodeId ? { ...n, data } : n);
  }

  // ── New empty flow ───────────────────────────────────────────────────────
  async function createNewEmpty() {
    const emptySnap: Snapshot = { nodes: [], edges: [] };
    lastSavedSnapshot = emptySnap;
    nodes = [];
    edges = [];
    undoStack = [];
    redoStack = [];
    try {
      const res = await flowService.create({
        flow_name: 'Untitled Flow',
        flow_data: { nodes: [], edges: [] },
        is_shared: false,
        thumbnail_svg: null,
      });
      activeFlow = res.flow;
      lastSavedSnapshot = { nodes: [], edges: [] };
    } catch {
      toast('Could not create flow on server — working offline');
      activeFlow = {
        flow_id: -1, user_id: 0, flow_name: 'Untitled Flow',
        flow_desc: '', is_default: false, is_shared: false,
        is_template: false, template_category: '',
        flow_data: { nodes: [], edges: [] }, thumbnail_svg: null,
        created_at: new Date().toISOString(), updated_at: new Date().toISOString(),
      };
    }
    showPicker = false;
  }

  // ── Save as template ────────────────────────────────────────────────────
  async function saveCurrentAsTemplate() {
    if (!activeFlow || activeFlow.flow_id < 0) { toast('Save the flow first'); return; }
    try {
      await flowService.saveAsTemplate(activeFlow.flow_id);
      toast('Saved as template');
    } catch { toast('Failed to save as template'); }
  }

  // ── Keyboard shortcuts ───────────────────────────────────────────────────
  function onKeydown(e: KeyboardEvent) {
    const mod = e.ctrlKey || e.metaKey;
    if (mod && e.key === 'z' && !e.shiftKey) { e.preventDefault(); undo(); }
    if (mod && (e.key === 'y' || (e.key === 'z' && e.shiftKey))) { e.preventDefault(); redo(); }
    if (mod && e.key === 's') { e.preventDefault(); save(); }
  }

  // ── Lifecycle ────────────────────────────────────────────────────────────
  onMount(async () => {
    onCollapseRail();
    const res = await flowService.getNodeTypes();
    nodeTypes = res.nodeTypes ?? [];
    await loadDefaultOrPicker();
  });

  onDestroy(() => {
    if (toastTimeout) clearTimeout(toastTimeout);
  });
</script>

<svelte:window onkeydown={onKeydown} />

<div class="fixed inset-0 flex flex-col" style="background:#0d1117; z-index:40; left:56px;">

  <!-- Toolbar -->
  <FlowToolbar
    {activeFlow}
    {isDirty}
    {canUndo}
    {canRedo}
    {darkMode}
    onPickerOpen={() => { if (isDirty) { if (!confirm('You have unsaved changes. Discard and continue?')) return; } showPicker = true; }}
    onRename={(name) => { if (activeFlow) { activeFlow = { ...activeFlow, flow_name: name }; } }}
    onUndo={undo}
    onRedo={redo}
    onFitView={() => {}}
    onSave={save}
    onSaveAsTemplate={saveCurrentAsTemplate}
    onClose={requestClose}
  />

  <!-- 3-panel body -->
  <div class="flex flex-1 overflow-hidden">

    <!-- Node Palette -->
    <NodePalette {nodeTypes} {darkMode} onDragStart={onPaletteDragStart} />

    <!-- Canvas -->
    <div
      class="flex-1 overflow-hidden"
      ondragover={(e) => e.preventDefault()}
      ondrop={onCanvasDrop}
      role="application"
      aria-label="Flow canvas"
    >
      <SvelteFlow
        bind:nodes
        bind:edges
        nodeTypes={NODE_TYPES}
        fitView
        onSelectionChange={(params) => {
          selectedNodeId = params.nodes.length === 1 ? params.nodes[0].id : null;
        }}
        onNodeDragStop={() => takeSnapshot()}
        onConnect={(connection) => {
          takeSnapshot();
          const sh = connection.sourceHandle ?? 'out';
          const th = connection.targetHandle ?? 'in';
          edges = [...edges, {
            id: `e-${connection.source}-${sh}-${connection.target}-${th}-${Date.now()}`,
            source: connection.source,
            sourceHandle: sh,
            target: connection.target,
            targetHandle: th,
            type: 'smoothstep',
          }];
          checkLimits();
        }}
        onEdgesDelete={() => takeSnapshot()}
        style="background:#0d1117;"
      >
        <Background variant={BackgroundVariant.Dots} gap={24} size={1} color="#374151" />
        <Controls />
        <MiniMap style="background:#161b27;" />
      </SvelteFlow>
    </div>

    <!-- Properties Panel -->
    <PropertiesPanel
      node={selectedNode as any}
      nodeType={selectedNodeType}
      {darkMode}
      onUpdate={updateNodeData}
    />
  </div>

  <!-- Toast -->
  {#if toastMsg}
    <div
      class="fixed bottom-6 left-1/2 -translate-x-1/2 px-4 py-2 rounded-lg text-sm shadow-xl"
      style="background:#1e2535; border:1px solid #374151; color:#e2e8f0; z-index:60; pointer-events:none;"
    >
      {toastMsg}
    </div>
  {/if}
</div>

<!-- Flow Picker Modal -->
{#if showPicker}
  <FlowPicker
    {darkMode}
    currentUserId={activeFlow?.user_id ?? 0}
    onOpen={loadFlow}
    onNewEmpty={createNewEmpty}
    onClose={() => { if (!activeFlow) requestClose(); else showPicker = false; }}
  />
{/if}
```

- [ ] **Step 2: Type-check**

```bash
cd /Users/cding/Workspace/ChenWeb/web && bun run check
```
Expected: 0 errors (fix any type errors that arise from @xyflow/svelte API differences)

- [ ] **Step 3: Manual integration test — start dev server**

```bash
cd /Users/cding/Workspace/ChenWeb/web && bun run dev
```

Walk through this checklist manually:
- [ ] Navigate to home3 → Tools → Flow
- [ ] Nav-rail collapses to 56px
- [ ] Flow Picker opens (no default set yet)
- [ ] Click "New Empty Flow" → blank canvas appears
- [ ] Drag a Text node from palette → node appears on canvas
- [ ] Drag an AI Assistant node → appears on canvas
- [ ] Click Text node → Properties Panel slides in, shows attributes
- [ ] Connect Text output to AI Assistant input → bezier edge appears
- [ ] Edit the text content in Properties Panel → node preview updates
- [ ] Press Cmd+S → "Saved" toast appears
- [ ] Press Cmd+Z → undo works
- [ ] Click ✕ Close → nav-rail restores to previous state

- [ ] **Step 4: Commit**

```bash
git add web/src/lib/components/shared-ui/canvas-01.svelte \
        web/src/lib/components/home3/content-panel.svelte
git commit -m "feat(flow): complete canvas-01 integration"
```

---

### Task 20: Final build verification

- [ ] **Step 1: Run full Go test suite**

```bash
cd /Users/cding/Workspace/ChenWeb && go test ./... 2>&1 | tail -20
```
Expected: all tests PASS, 0 failures

- [ ] **Step 2: Run frontend type check and lint**

```bash
cd /Users/cding/Workspace/ChenWeb/web && bun run check && bun run lint
```
Expected: 0 errors, 0 warnings (fix any lint issues)

- [ ] **Step 3: Production build**

```bash
cd /Users/cding/Workspace/ChenWeb/web && bun run build
```
Expected: successful build, no errors

- [ ] **Step 4: Go build**

```bash
cd /Users/cding/Workspace/ChenWeb && go build ./...
```
Expected: 0 errors

- [ ] **Step 5: Final commit**

```bash
git add .
git commit -m "feat(flow): flow canvas editor complete — backend stubs, 11 nodes, flow picker, properties panel"
```

---

## Summary

| Chunk | Tasks | Deliverables |
|-------|-------|-------------|
| 1 — Foundation | 1–9 | `@xyflow/svelte` installed, TypeScript types, flowService, Go stubs (9 handlers, 9 tests), Goose migration, routes registered |
| 2 — Home3 + Shell | 10–14 | Tools→Flow in nav-rail, Canvas01 mounted in content-panel, FlowCard, FlowPicker, FlowToolbar |
| 3 — Nodes + Integration | 15–20 | NodePalette, PropertiesPanel, BaseNode + 11 node components, full canvas-01.svelte integration, build verification |

**Key files:**
- `server/api/flowhandler/` — 3 Go files, 9 handler functions, 9 tests
- `web/src/lib/components/shared-ui/canvas-01.svelte` — main entry
- `web/src/lib/components/shared-ui/canvas/` — 5 component files + 12 node files
- `web/src/lib/services/flowService.ts` — API layer
- `web/src/lib/types/flow.ts` — shared types
