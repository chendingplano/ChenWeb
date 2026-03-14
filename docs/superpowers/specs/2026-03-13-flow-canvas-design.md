# Flow Canvas Editor — Design Spec

**Date:** 2026-03-13
**Project:** ChenWeb
**Status:** Approved by user

---

## Overview

Add a visual flow/canvas editor to ChenWeb's home3 application shell. Users build workflows by connecting typed nodes on a canvas. Flows are persisted to PostgreSQL, can be shared or private, and can be saved as reusable templates.

---

## User-Facing Requirements

- A **Tools → Flow** menu item appears in home3's nav-rail.
- Selecting it opens the canvas, which fills the entire content area. The nav-rail collapses to icon-only (56px) using the existing `railPinned` mechanism; it remains visible on the left edge.
- On open: if the user has a default flow, it loads immediately. Otherwise the **Flow Picker** modal opens.
- The canvas displays a dot-grid background, supports pan/zoom, undo/redo, and a minimap.
- A **Node Palette** docks to the left (200px); it can be dragged out as a floating panel.
- Users drag nodes from the palette onto the canvas.
- Clicking a node selects it: the **Properties Panel** slides in from the right (240px), showing **editable** attributes and connector labels. Connector names (e.g., `text`, `context`, `response`) are **fixed by the node type** and are read-only in the Properties Panel. Input connectors appear on the left side of the node, output connectors on the right.
- A **floating mini-toolbar** appears above a selected node with three icons: **Configure** (opens/focuses Properties Panel), **Duplicate**, **Delete**.
- Nodes connect via **smooth bezier edges** drawn from output to input connectors.
- A **top toolbar** provides: flow name (click to inline-rename — persisted on blur or Enter; dropdown arrow opens Flow Picker), undo, redo, zoom, fit-to-view, save, save-as-template (for the currently open flow), run (stubbed), close.
- Flows can be **shared** (visible to all users) or **private** (owner only), controlled by the creator at save time.
- Any flow can be **saved as a template**. New flows can be created from an existing flow or a template (fork).
- One flow per user can be marked as the **default flow**. Only the owner of a flow may set it as their default.

---

## Architecture

### Layout

When `activeMenu.itemId === 'flow'`, `content-panel.svelte` renders `<Canvas01 />` which fills the entire content area. The nav-rail collapses to icon-only (56px) via existing `railPinned = false` + `railExpanded = false` logic. No new SvelteKit routes required. When the user clicks **Close** (✕) or navigates to a different menu item, the nav-rail restores to its previous state (expanded width and pin state) that was captured before the canvas opened. `canvas-01.svelte` captures `railWidth` and `railPinned` into local `$state` variables on mount and restores them via a callback prop (or event) to `home3/+page.svelte` on close.

```
[Nav 56px] [Canvas01 — fills remaining width]
┌──────────────────────────────────────────────────────────────┐
│  FlowToolbar: [name ▼] | [undo][redo] | [zoom][fit] | [save][run] | [✕] │
├──────────────┬───────────────────────────────┬───────────────┤
│ NodePalette  │   @xyflow/svelte Canvas        │ PropertiesPanel│
│ (200px dock  │   - dot-grid background        │ (240px, slides│
│  or floating)│   - pan / zoom                 │  in on select)│
│              │   - drag-drop nodes            │               │
│ grouped by   │   - bezier edges               │               │
│ category,    │   - minimap (bottom-right)     │               │
│ searchable   │   - zoom controls (btm-center) │               │
└──────────────┴───────────────────────────────┴───────────────┘
```

### Rendering Engine

**`@xyflow/svelte`** — handles node rendering, edge routing, pan/zoom, drag-and-drop, and minimap. Each of the 11 node types is a custom Svelte component registered in the `nodeTypes` map passed to `<SvelteFlow>`.

### Undo / Redo

A snapshot `{ nodes, edges }` is taken **on mouse-up after any drag** and **immediately after any edge create or delete**. Snapshots are pushed onto `undoStack`. Undo pops the top snapshot from `undoStack`, pushes current state onto `redoStack`, and restores the popped snapshot. Max 50 snapshots; oldest is discarded when limit is exceeded. **The undo/redo stacks are cleared when a different flow is loaded.**

---

## Flow Picker Modal

Triggered on canvas open (no default flow), or when `GetDefaultFlow` returns 404 (default flow was deleted), or by clicking the dropdown arrow in the toolbar flow name.

**Tabs:** My Flows · Shared Flows · Templates

**Per tab:** searchable card grid. Each card shows a mini SVG thumbnail, flow name, node count, last-edited time, and privacy badge.

**Actions:**
- Click card → open flow; undo/redo stacks are cleared.
- Right-click card → context menu (items shown/hidden based on ownership):
  - **Open** — always shown; load flow
  - **Set as default** — shown only if `current_user === owner`
  - **Save as template** — shown only if `current_user === owner`; copies flow as a new template row with `is_template = TRUE`, `is_shared = TRUE` (templates are always public)
  - **Duplicate** — always shown; forks into a new private flow owned by the current user
  - **Delete** — shown only if `current_user === owner`
- Templates tab: same card grid; "Use template" button forks the template into a new private flow.
- "New Empty Flow" button → creates a blank private flow, opens canvas.

### Authorization Summary

| Action | Allowed when |
|--------|-------------|
| Open | owner OR `is_shared = TRUE` |
| Set as default | owner only |
| Save as template | owner only |
| Duplicate | any user (forks into their own flow) |
| Delete | owner only |

---

## Node Types (11 initial)

All nodes share a **BaseNode** chrome: title bar (icon + name), floating mini-toolbar on selection, input connectors (left), output connectors (right). Edits in the Properties Panel write directly into the node's `data` object in state, triggering a reactive re-render.

| ID | Label | Category | Inputs | Outputs | Key Attributes |
|----|-------|----------|--------|---------|----------------|
| `ai-assistant` | AI Assistant | AI | text, context | response | model (select), system_prompt, temperature |
| `coding-assistant` | Coding Assistant | AI | code, context | code, explanation | language, model |
| `text` | Text | Data | — | text | content (multiline textarea) |
| `document` | Document | Data | — | document | doc_id, doc_source |
| `file` | File | Data | — | file | file_path, file_type |
| `media` | Media | Data | — | media | media_url, media_type |
| `tool` | Tool | Actions | args | result | tool_name, tool_config |
| `mcp` | MCP | Actions | request | response | server_url, auth_token |
| `http-request` | HTTP Request | Actions | body? | response | url, method, headers |
| `rule` | Rule | Actions | data | pass, fail | rule_expression, description |
| `git` | GIT | Actions | files? | result | repo_url, branch, operation |

### Properties Panel — UI Control Types

| Node Type | Attribute | UI Control |
|-----------|-----------|------------|
| ai-assistant | model | `<select>` (GPT-4o, GPT-4o-mini, Claude 3.5, etc.) |
| ai-assistant | system_prompt | `<textarea>` multiline |
| ai-assistant | temperature | `<input type="range">` 0–2, step 0.1 |
| coding-assistant | language | `<select>` (Python, TypeScript, Go, etc.) |
| coding-assistant | model | `<select>` same model list |
| text | content | `<textarea>` multiline |
| document | doc_id | `<input type="text">` |
| document | doc_source | `<input type="text">` |
| file | file_path | `<input type="text">` |
| file | file_type | `<select>` (csv, json, txt, pdf, …) |
| media | media_url | `<input type="text">` |
| media | media_type | `<select>` (image, audio, video) |
| tool | tool_name | `<input type="text">` |
| tool | tool_config | `<textarea>` (JSON) |
| mcp | server_url | `<input type="text">` |
| mcp | auth_token | `<input type="password">` (masked) |
| http-request | url | `<input type="text">` |
| http-request | method | `<select>` (GET, POST, PUT, DELETE, PATCH) |
| http-request | headers | `<textarea>` (JSON) |
| rule | rule_expression | `<textarea>` |
| rule | description | `<input type="text">` |
| git | repo_url | `<input type="text">` |
| git | branch | `<input type="text">` |
| git | operation | `<select>` (clone, pull, push, commit, status) |

All nodes also show a **Name** field (`<input type="text">`, max 100 chars) at the top of the Properties Panel — this edits the node's display label. `flow_name` in the toolbar is max **255 chars** (client-side enforced; matches DB column).

**Soft limits:** the frontend warns (toast) once when the node count first exceeds 500, or once when the edge count first exceeds 1000. The toast does not repeat on subsequent adds beyond the threshold. The action is not blocked. No hard server-side limit for this phase.

---

## Database Schema

### Table: `flows`

```sql
CREATE TABLE flows (
    flow_id           BIGSERIAL PRIMARY KEY,
    user_id           BIGINT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    flow_name         VARCHAR(255) NOT NULL,
    flow_desc         TEXT,
    is_default        BOOLEAN NOT NULL DEFAULT FALSE,
    is_shared         BOOLEAN NOT NULL DEFAULT FALSE,
    is_template       BOOLEAN NOT NULL DEFAULT FALSE,
    template_category VARCHAR(100),
    flow_data         JSONB NOT NULL DEFAULT '{"nodes":[],"edges":[]}',
    thumbnail_svg     TEXT,   -- inline SVG string, generated client-side and stored on save
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Only one default flow per user
CREATE UNIQUE INDEX flows_user_default_idx ON flows (user_id) WHERE is_default = TRUE;
```

**`thumbnail_svg`** is an inline SVG string (e.g. `<svg ...>...</svg>`) generated client-side by `canvas-01.svelte` from the current node positions and edges, serialised as a string, and sent to the server on `PUT /flows/:id`. The FlowCard renders it as raw SVG inside the card thumbnail area. Recommended soft cap: 50 KB. When a flow is forked or saved as a template, the server copies `thumbnail_svg` from the source row.

**Pre-condition — `users` table**: `user_id` references the existing `users` table managed by the Ory Kratos auth layer. The exact table name and primary key column must be confirmed against the existing schema before writing the migration. This is a hard dependency that must be resolved before the migration file is created.

`flow_data` JSONB structure matches `@xyflow/svelte`'s native node/edge format:
```json
{
  "nodes": [
    { "id": "n1", "type": "text", "position": { "x": 100, "y": 80 }, "data": { "content": "Hello" } }
  ],
  "edges": [
    { "id": "e1", "source": "n1", "sourceHandle": "out-text", "target": "n2", "targetHandle": "in-text" }
  ]
}
```

### Node Types: static in handler (no DB table)

The 11 node type definitions are returned as **hardcoded JSON** from `GetNodeTypes`. No `flow_node_types` table is required for this phase. This avoids a seeding dependency and simplifies the implementation.

---

## REST API

All endpoints under `/api/v1`, protected by auth middleware. User identity is read from the session/JWT in the request context.

### Error Envelope

All error responses use:
```json
{ "error": { "code": "NOT_FOUND", "message": "flow not found" } }
```

Standard HTTP codes: `400` bad request, `401` unauthenticated, `403` forbidden (wrong owner), `404` not found, `500` internal error.

### Endpoints

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/flows` | `ListFlows` | `?scope=mine` → `WHERE user_id = $me` (all owned flows including own shared and own templates) · `?scope=shared` → `WHERE is_shared = TRUE AND is_template = FALSE AND user_id != $me` (other users' shared non-template flows; intentionally excludes caller's own shared flows which appear in `mine`) · `?scope=templates` → `WHERE is_template = TRUE` (all templates including caller's own). No pagination. |
| POST | `/flows` | `CreateFlow` | Create new flow; request body includes `flow_name`, `flow_data`, `thumbnail_svg` (may be null for blank flows), `is_shared`; returns created flow record |
| GET | `/flows/default` | `GetDefaultFlow` | Get user's default flow; returns 404 if none set |
| GET | `/flows/:id` | `GetFlow` | Get one flow; 403 if private and not owner |
| PUT | `/flows/:id` | `UpdateFlow` | Update name, desc, data, sharing, thumbnail; 403 if not owner |
| DELETE | `/flows/:id` | `DeleteFlow` | Delete flow; 403 if not owner |
| PUT | `/flows/:id/default` | `SetDefaultFlow` | Mark as default (clears previous default for user); 403 if not owner |
| POST | `/flows/:id/fork` | `ForkFlow` | Fork into new flow: `is_default=false`, `is_template=false`, `is_shared=false`, name = "Copy of \<original name\>" (truncated to 255 chars); allowed if owner OR source `is_shared=TRUE`; 403 otherwise |
| POST | `/flows/:id/template` | `SaveAsTemplate` | Copy flow as template: `is_template=true`, `is_shared=true`; 403 if not owner |
| GET | `/flow-node-types` | `GetNodeTypes` | Return static list of 11 node type definitions as `{ "nodeTypes": [ { "id": "ai-assistant", "label": "AI Assistant", "category": "AI", "icon": "Bot", "inputs": ["text","context"], "outputs": ["response"], "defaultData": { "model": "gpt-4o", "system_prompt": "", "temperature": 0.7 } }, ... ] }` |

All handlers are **stubs** for this implementation phase. "Stub" means: real route registration, real auth middleware, real request parsing, correct HTTP status codes (200/201/400/401/403/404/500), and structured log entries using `loggerutil.CreateDefaultLogger("CWB_FLW_XXX")` — but no database interaction. Success responses return a hardcoded placeholder JSON body matching the expected shape. `UpdateFlow` sets `updated_at = NOW()` in the handler (not via DB trigger); this is noted here so it's handled even in the stub response shape.

---

## Frontend Error Handling

- **Save failure** (`PUT /flows/:id` returns 5xx): show an error toast ("Failed to save — your changes are preserved"); retain `isDirty = true`; do not clear undo stack.
- **Load failure** (`GET /flows/:id` returns 4xx/5xx): show error toast; re-open Flow Picker.
- **Default flow 404** (`GET /flows/default` returns 404): open Flow Picker.
- **Node count warning**: if nodes > 500 or edges > 1000, show a toast warning on every add that crosses the threshold. Do not block the action.
- **Unsaved changes navigation guard**: if `isDirty = true` and the user clicks Close (✕), switches to a different menu item, or opens the Flow Picker to load a different flow, show a confirmation dialog: "You have unsaved changes. Discard and continue?" Cancel returns to the canvas; Confirm discards changes and proceeds.
- **Fork 403**: show error toast "You don't have permission to duplicate this flow."

---

## File Breakdown

### Frontend (new files)

```
web/src/lib/components/shared-ui/
  canvas-01.svelte                  ← main entry, all canvas state
  canvas/
    FlowToolbar.svelte
    FlowPicker.svelte
    NodePalette.svelte
    PropertiesPanel.svelte
    FlowCard.svelte
    nodes/
      BaseNode.svelte
      AiAssistantNode.svelte
      TextNode.svelte
      FileNode.svelte
      DocumentNode.svelte
      MediaNode.svelte
      ToolNode.svelte
      McpNode.svelte
      RuleNode.svelte
      CodingAssistantNode.svelte
      HttpRequestNode.svelte
      GitNode.svelte

web/src/lib/services/
  flowService.ts                    ← all flow REST calls + error handling

web/src/lib/types/
  flow.ts                           ← Flow, FlowNode, FlowEdge, NodeType, Connector interfaces
```

### Frontend (modified files)

```
web/src/routes/home3/
  nav-rail.svelte      ← add Tools section with Flow child (id: 'flow')
  content-panel.svelte ← render <Canvas01 /> when activeMenu.itemId === 'flow'
```

### Backend (new files)

```
server/api/
  flowhandler/
    flowhandler.go          ← ListFlows, CreateFlow, GetFlow, UpdateFlow, DeleteFlow
    flowhandler_actions.go  ← GetDefaultFlow, SetDefaultFlow, ForkFlow, SaveAsTemplate
    nodetypes.go            ← GetNodeTypes (returns static hardcoded JSON)
  appdatastores/
    table-flows.go          ← CreateFlowsTable() — flows table + unique index

server/api/routes.go        ← register 10 new endpoints
```

### Database migration

One new Goose migration file: `server/migrations/YYYYMMDDHHMMSS_create_flows.sql` — creates the `flows` table and the partial unique index.

---

## State (canvas-01.svelte)

```typescript
let activeFlow        = $state<Flow | null>(null)
let nodes             = $state<Node[]>([])
let edges             = $state<Edge[]>([])
let selectedNodeId    = $state<string | null>(null)
let showPicker        = $state(false)
let paletteFloating   = $state(false)
let undoStack         = $state<Snapshot[]>([])   // max 50
let redoStack         = $state<Snapshot[]>([])
let lastSavedSnapshot = $state<Snapshot | null>(null)
// lastSavedSnapshot is set:
//   - On load: set to { nodes, edges } from the loaded flow
//   - "New Empty Flow": set to { nodes: [], edges: [] } immediately (before POST), so isDirty works even if POST fails
//   - On successful POST /flows: update to match server-returned flow_data (in case server normalises it)
//   - On successful PUT /flows/:id: update to current { nodes, edges }
// If POST /flows fails: lastSavedSnapshot remains { nodes: [], edges: [] } — any edits the user has made
//   will correctly show isDirty = true, and the unsaved-changes guard will fire on navigation.
let isDirty           = $derived(
  lastSavedSnapshot !== null &&
  JSON.stringify({ nodes, edges }) !== JSON.stringify(lastSavedSnapshot)
)
```

---

## Dependencies

- **`@xyflow/svelte`** — add to `web/package.json`

---

## Out of Scope (this phase)

- Flow execution engine (Run button is present but stubbed; clicks log a message)
- Node-level execution / streaming output
- Real-time collaborative editing
- Flow versioning / history
- Mobile support
- Thumbnail auto-regeneration on every save (thumbnail is generated on demand, not on every node drag)
- Pagination for `ListFlows` (all flows returned in a single response for this phase)
- File naming convention note: node component files use PascalCase derived from kebab-case type ID (e.g. `http-request` → `HttpRequestNode.svelte`, `coding-assistant` → `CodingAssistantNode.svelte`)
