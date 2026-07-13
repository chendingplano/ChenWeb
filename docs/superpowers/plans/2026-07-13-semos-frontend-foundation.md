# SemOS Customer-Facing Frontend — Phase 1 (Foundation) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the foundation of the new SemOS customer-facing frontend: tenant table, two-source site-config system (tenant-independent file + per-tenant files), Go API endpoints, and the two designed pages (Main, Workspace Landing) with site-wide i18n and light/dark mode.

**Architecture:** A new `sitehandler` Go package loads TOML site-config files (the shared tenant-independent file named by `[config].config_filename` in `config.local.toml`; per-tenant filenames looked up from a new `site_tenants` table) and serves them as JSON. A new `/semos` SvelteKit route group renders the Main and Workspace Landing pages from that config, with a shared layout providing header/nav, paraglide language switcher, and a `.dark`-class theme toggle.

**Tech Stack:** Go 1.25 + Echo v4.15.1 + viper + pelletier/go-toml/v2 + goose migrations (backend); SvelteKit + Svelte 5 runes + Tailwind v4 + @inlang/paraglide-js + @lucide/svelte (frontend).

**Spec:** `KnowledgeStore/doc-repo/adrs/202607/2026071102-adr-new-gui-semos.md` (design brief).

## Scope: Phase 1 of 3

The design brief covers 22 pages plus admin subsystems. Per plan-scoping rules this is split into independently shippable phases:

- **Phase 1 (this plan):** tenant table, config plumbing, site-config API, `/semos` layout (i18n + theme), Main page, Workspace Landing page.
- **Phase 2 (separate plan):** remaining marketing/placeholder pages (Product Overview, Pricing, Feature pages, etc.).
- **Phase 3 (separate plan):** User Management + Site Management — blocked on the follow-up auth/role ADR.

## Global Constraints

- Design brief decisions are binding: config storage is **files** (TOML, in Git); `[config].config_filename` in `ChenWeb/config.local.toml` names **only** the shared tenant-independent file; tenant-dependent config filenames come **only** from the tenant DB table.
- Tenants are **placeholders** this pass — no user↔tenant binding exists yet (deferred to future auth ADR). The tenant config endpoint takes an explicit `tenant_id` path param and requires only an authenticated session.
- Site-wide **light + dark mode** via the existing `.dark` class / CSS variables in `web/src/app.css` — do not invent a second token system.
- Site-wide **i18n** via the existing paraglide setup (`web/src/lib/paraglide/runtime.js`, locales `en`, `zh-cn`). Message keys prefixed `semos_`.
- **VCS is jj**, not git: commit with `jj commit -m "..."`. Repo: `/Users/cding/Workspace/ChenWeb`.
- Go module path: `github.com/chendingplano/deepdoc`. Shared lib: `github.com/chendingplano/shared/go`.
- DB pool in handlers: `ApiTypes.ProjectDBHandle` (import `ApiTypes "github.com/chendingplano/shared/go/api/ApiTypes"`).
- New table via goose SQL migration in `project_migrations/` (pattern: `YYYYMMDD000001_name.sql`); migrations run automatically at server startup.
- Frontend checks: `cd web && bun run check` (svelte-check). There is no JS unit-test runner in `web/package.json` — do not add one in this plan.
- ChenWeb coding rules apply (`ChenWeb/CLAUDE.md`): simplicity first, surgical changes, no speculative flexibility.
- All Go handler operations must log via the package logger pattern used by neighboring handlers.

---

### Task 1: `site_tenants` migration

**Status: COMPLETE** (commit `lmtt 0a5f6c9`, review Approved). Task text retained for the record.

**Files:**
- Create: `project_migrations/20260713000001_create_site_tenants.sql`

**Interfaces:**
- Produces: table `site_tenants(tenant_id VARCHAR(128) PK, tenant_name TEXT, config_filename TEXT NOT NULL, created_at, updated_at)` — consumed by Task 3's `GetTenantConfigFilename`.

- [x] **Step 1: Write the migration**

```sql
-- +goose Up
CREATE TABLE IF NOT EXISTS site_tenants (
    tenant_id       VARCHAR(128) PRIMARY KEY,
    tenant_name     TEXT NOT NULL DEFAULT '',
    config_filename TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE site_tenants IS
    'Tenant registry for the SemOS customer-facing frontend (ADR 2026071102). '
    'config_filename is the per-tenant site-config TOML path relative to the ChenWeb repo root. '
    'Minimal placeholder shape; full RBAC/tenant schema deferred to a future auth ADR.';

-- +goose Down
DROP TABLE IF EXISTS site_tenants;
```

- [x] **Step 2: Verify the migration applies** (done via psql direct apply; goose parsing gets exercised by Task 4's server start)

- [x] **Step 3: Commit**

---

### Task 2: Config plumbing — `[config].config_filename` + site-config TOML files

**Files:**
- Modify: `server/cmd/config/config.go` (add section to `AppConfigDef` around line 155, getter after `GetLanguages` around line 297)
- Modify: `config.local.toml` (add `[config]` section)
- Create: `config/site/site-default.toml`
- Create: `config/site/tenant-demo.toml`

**Interfaces:**
- Produces: `config.GetSiteConfigFilename() string` — consumed by Task 3.
- Produces: site-config TOML schema (sections `[branding]`, `[hero]`, `[[highlights]]`, `[[features]]`, `[footer]`, `[workspace]`, `[[workspace.apps]]`) — parsed by Task 3, rendered by Tasks 6–7.

- [ ] **Step 1: Add the config section and getter to `server/cmd/config/config.go`**

Inside `AppConfigDef` (after the `Languages LanguagesConfig` field, line ~155), add:

```go
	// SiteConfig names the tenant-independent site-config file for the
	// customer-facing frontend (ADR 2026071102). Tenant-dependent config
	// filenames come from the site_tenants table, never from here.
	SiteConfig SiteConfigSection `mapstructure:"config"`
```

After the `PDFParserConfig` struct definition (line ~175), add:

```go
type SiteConfigSection struct {
	ConfigFilename string `mapstructure:"config_filename"`
}
```

After `GetLanguages()` (line ~297), add:

```go
func GetSiteConfigFilename() string {
	return AppConfig.SiteConfig.ConfigFilename
}
```

- [ ] **Step 2: Add the `[config]` section to `config.local.toml`**

Append to `/Users/cding/Workspace/ChenWeb/config.local.toml`:

```toml
[config]
config_filename = "config/site/site-default.toml"
```

- [ ] **Step 3: Create `config/site/site-default.toml`** (tenant-independent; placeholder copy per design brief — content quality is explicitly not the point of this pass)

```toml
# Tenant-independent site config for the SemOS customer-facing frontend.
# Named by [config].config_filename in config.local.toml (ADR 2026071102).

[branding]
site_name = "SemOS"
logo_text = "SemOS"
powered_by = "Powered by SemOS"

[hero]
slogan = "Your Knowledge, Working for You"
subtitle = "SemOS turns documents into searchable, reviewable, actionable knowledge — and lets you build AI applications on top of it."
image = "/images/angleWalls.jpg"
cta_primary_label = "Get Started"
cta_primary_href = "/semos/workspace"
cta_secondary_label = "Sign Up / Login"
cta_secondary_href = "/login"

[[highlights]]
title = "Every document, understood"
description = "Upload files or collect them from the web. SemOS parses PDF, Word, PPT and more, extracting structure, not just text."
image = "/images/angleWalls.jpg"

[[highlights]]
title = "Artifacts, not just pages"
description = "Metrics, compliance provisions, entities and relations are extracted automatically and become first-class, searchable objects."
image = "/images/angleWalls.jpg"

[[highlights]]
title = "Search that actually finds"
description = "Hybrid search combines BM25 keyword matching with semantic vector similarity, so you find what you meant, not just what you typed."
image = "/images/angleWalls.jpg"

[[highlights]]
title = "Reviews with receipts"
description = "Document reviews cite their source lines. Every finding links back to the exact place in the original document."
image = "/images/angleWalls.jpg"

[[highlights]]
title = "Build your own AI apps"
description = "A development environment for building and offering AI applications on top of your knowledge base."
image = "/images/angleWalls.jpg"

[[features]]
key = "knowledge_base"
title = "Knowledge Base"
description = "Centralize documents and extracted artifacts in one governed store."
href = "/home3/knowledge"

[[features]]
key = "chat"
title = "Chat with Documents"
description = "Ask questions in plain language; answers grounded in your documents."
href = "/home3"

[[features]]
key = "search"
title = "Search the Knowledge Base"
description = "Hybrid BM25 + semantic search across everything you have ingested."
href = "/home3/knowledge"

[[features]]
key = "app_dev"
title = "Agent and App Development"
description = "Develop, run, and offer your own AI applications and agents."
href = "/home3"

[footer]
text = "SemOS — Knowledge Management AI Application System"

[workspace]
banner_title = "Your Workspace"
banner_subtitle = "Everything you need to work with your knowledge base, in one place."
banner_image = "/images/angleWalls.jpg"

[[workspace.apps]]
name = "Knowledge Base"
description = "Browse and manage documents and artifacts."
href = "/home3/knowledge"
icon = "database"

[[workspace.apps]]
name = "Chat with Your Knowledge Base"
description = "Grounded Q&A over your documents."
href = "/home3"
icon = "message-circle"

[[workspace.apps]]
name = "Search Your Knowledge Base"
description = "Hybrid keyword + semantic search."
href = "/home3/knowledge"
icon = "search"

[[workspace.apps]]
name = "Document Reviews"
description = "Run and read document reviews with cited findings."
href = "/home3/inputs"
icon = "file-check"

[[workspace.apps]]
name = "Workflows"
description = "Automate multi-step document processing."
href = "/home3"
icon = "workflow"

[[workspace.apps]]
name = "Agents and Harness"
description = "Build and run AI agents over your knowledge."
href = "/home3"
icon = "bot"
```

- [ ] **Step 4: Create `config/site/tenant-demo.toml`**

Copy of `site-default.toml` with these fields changed (rest identical — copy the full file, then edit):

```toml
[branding]
site_name = "Demo Corp Knowledge Portal"
logo_text = "Demo Corp"
powered_by = "Powered by SemOS"

[hero]
slogan = "Demo Corp: Institutional Knowledge, On Tap"
```

- [ ] **Step 5: Verify it compiles**

```bash
cd /Users/cding/Workspace/ChenWeb && go build ./server/... && go vet ./server/cmd/config/
```

Expected: no errors.

- [ ] **Step 6: Commit**

```bash
jj commit -m "feat(semos): [config].config_filename plumbing + site-config TOML files"
```

---

### Task 3: `sitehandler` package (TDD)

**Files:**
- Create: `server/api/sitehandler/sitehandler.go`
- Create: `server/api/sitehandler/sitehandler_test.go`
- Create: `server/api/sitehandler/testdata/site-valid.toml` (copy of `config/site/site-default.toml` from Task 2)

**Interfaces:**
- Consumes: `config.GetSiteConfigFilename()` (Task 2), table `site_tenants` (Task 1), `ApiTypes.ProjectDBHandle`.
- Produces: `sitehandler.GetSiteConfig(c echo.Context) error` and `sitehandler.GetTenantSiteConfig(c echo.Context) error` — registered in Task 4. JSON response shape consumed by Task 5's TS types:
  `{branding:{site_name,logo_text,powered_by}, hero:{slogan,subtitle,image,cta_primary_label,cta_primary_href,cta_secondary_label,cta_secondary_href}, highlights:[{title,description,image}], features:[{key,title,description,href}], footer:{text}, workspace:{banner_title,banner_subtitle,banner_image,apps:[{name,description,href,icon}]}}`

- [ ] **Step 1: Write failing tests** (`server/api/sitehandler/sitehandler_test.go`)

```go
package sitehandler

import (
	"database/sql"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestLoadSiteConfigValid(t *testing.T) {
	cfg, err := LoadSiteConfig("testdata/site-valid.toml")
	if err != nil {
		t.Fatalf("LoadSiteConfig: %v", err)
	}
	if cfg.Branding.SiteName != "SemOS" {
		t.Errorf("SiteName = %q, want SemOS", cfg.Branding.SiteName)
	}
	if len(cfg.Highlights) != 5 {
		t.Errorf("len(Highlights) = %d, want 5", len(cfg.Highlights))
	}
	if len(cfg.Features) != 4 {
		t.Errorf("len(Features) = %d, want 4", len(cfg.Features))
	}
	if len(cfg.Workspace.Apps) != 6 {
		t.Errorf("len(Workspace.Apps) = %d, want 6", len(cfg.Workspace.Apps))
	}
	if cfg.Hero.CTAPrimaryHref != "/semos/workspace" {
		t.Errorf("CTAPrimaryHref = %q", cfg.Hero.CTAPrimaryHref)
	}
}

func TestLoadSiteConfigMissingFile(t *testing.T) {
	if _, err := LoadSiteConfig("testdata/does-not-exist.toml"); err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestGetTenantConfigFilename(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT config_filename FROM site_tenants").
		WithArgs("demo").
		WillReturnRows(sqlmock.NewRows([]string{"config_filename"}).
			AddRow("config/site/tenant-demo.toml"))

	got, err := GetTenantConfigFilename(db, "demo")
	if err != nil {
		t.Fatalf("GetTenantConfigFilename: %v", err)
	}
	if got != "config/site/tenant-demo.toml" {
		t.Errorf("got %q", got)
	}
}

func TestGetTenantConfigFilenameNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT config_filename FROM site_tenants").
		WithArgs("nope").
		WillReturnError(sql.ErrNoRows)

	if _, err := GetTenantConfigFilename(db, "nope"); err == nil {
		t.Fatal("expected error for unknown tenant, got nil")
	}
}
```

- [ ] **Step 2: Create `testdata/site-valid.toml`** — exact copy of `config/site/site-default.toml` from Task 2 Step 3.

- [ ] **Step 3: Run tests, verify they fail**

```bash
cd /Users/cding/Workspace/ChenWeb && go test ./server/api/sitehandler/ -v
```

Expected: FAIL — `LoadSiteConfig`, `GetTenantConfigFilename` undefined.

- [ ] **Step 4: Implement** (`server/api/sitehandler/sitehandler.go`)

```go
// Package sitehandler serves site-config for the SemOS customer-facing
// frontend (ADR 2026071102). Two sources: the tenant-independent file named
// by [config].config_filename in config.local.toml, and per-tenant files
// whose names are looked up from the site_tenants table.
package sitehandler

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"

	"github.com/labstack/echo/v4"
	toml "github.com/pelletier/go-toml/v2"

	"github.com/chendingplano/deepdoc/server/cmd/config"
	ApiTypes "github.com/chendingplano/shared/go/api/ApiTypes"
)

type Branding struct {
	SiteName  string `toml:"site_name" json:"site_name"`
	LogoText  string `toml:"logo_text" json:"logo_text"`
	PoweredBy string `toml:"powered_by" json:"powered_by"`
}

type Hero struct {
	Slogan            string `toml:"slogan" json:"slogan"`
	Subtitle          string `toml:"subtitle" json:"subtitle"`
	Image             string `toml:"image" json:"image"`
	CTAPrimaryLabel   string `toml:"cta_primary_label" json:"cta_primary_label"`
	CTAPrimaryHref    string `toml:"cta_primary_href" json:"cta_primary_href"`
	CTASecondaryLabel string `toml:"cta_secondary_label" json:"cta_secondary_label"`
	CTASecondaryHref  string `toml:"cta_secondary_href" json:"cta_secondary_href"`
}

type Highlight struct {
	Title       string `toml:"title" json:"title"`
	Description string `toml:"description" json:"description"`
	Image       string `toml:"image" json:"image"`
}

type Feature struct {
	Key         string `toml:"key" json:"key"`
	Title       string `toml:"title" json:"title"`
	Description string `toml:"description" json:"description"`
	Href        string `toml:"href" json:"href"`
}

type Footer struct {
	Text string `toml:"text" json:"text"`
}

type WorkspaceApp struct {
	Name        string `toml:"name" json:"name"`
	Description string `toml:"description" json:"description"`
	Href        string `toml:"href" json:"href"`
	Icon        string `toml:"icon" json:"icon"`
}

type Workspace struct {
	BannerTitle    string         `toml:"banner_title" json:"banner_title"`
	BannerSubtitle string         `toml:"banner_subtitle" json:"banner_subtitle"`
	BannerImage    string         `toml:"banner_image" json:"banner_image"`
	Apps           []WorkspaceApp `toml:"apps" json:"apps"`
}

type SiteConfig struct {
	Branding   Branding    `toml:"branding" json:"branding"`
	Hero       Hero        `toml:"hero" json:"hero"`
	Highlights []Highlight `toml:"highlights" json:"highlights"`
	Features   []Feature   `toml:"features" json:"features"`
	Footer     Footer      `toml:"footer" json:"footer"`
	Workspace  Workspace   `toml:"workspace" json:"workspace"`
}

// LoadSiteConfig parses one complete site-config TOML file.
func LoadSiteConfig(path string) (*SiteConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("(CWB_SITE_001) read site config %s: %w", path, err)
	}
	var cfg SiteConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("(CWB_SITE_002) parse site config %s: %w", path, err)
	}
	return &cfg, nil
}

// GetTenantConfigFilename looks up a tenant's site-config filename from the
// site_tenants table. Tenant-dependent filenames come only from this table,
// never from config.local.toml (ADR 2026071102).
func GetTenantConfigFilename(db *sql.DB, tenantID string) (string, error) {
	var filename string
	err := db.QueryRow(
		"SELECT config_filename FROM site_tenants WHERE tenant_id = $1",
		tenantID,
	).Scan(&filename)
	if err != nil {
		return "", fmt.Errorf("(CWB_SITE_003) tenant %q: %w", tenantID, err)
	}
	return filename, nil
}

// GetSiteConfig serves the tenant-independent site config.
// Endpoint: GET /api/site-config (public — all pre-login pages use this).
func GetSiteConfig(c echo.Context) error {
	path := config.GetSiteConfigFilename()
	if path == "" {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "(CWB_SITE_004) [config].config_filename not set in config.local.toml",
		})
	}
	cfg, err := LoadSiteConfig(path)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, cfg)
}

// GetTenantSiteConfig serves a tenant's site config.
// Endpoint: GET /api/v1/site-config/tenant/:tenant_id (authenticated).
// Tenants are placeholders this pass (no user<->tenant binding yet); any
// authenticated session may fetch any tenant's config until the follow-up
// auth ADR lands.
func GetTenantSiteConfig(c echo.Context) error {
	tenantID := c.Param("tenant_id")
	if tenantID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "(CWB_SITE_005) tenant_id is required",
		})
	}
	filename, err := GetTenantConfigFilename(ApiTypes.ProjectDBHandle, tenantID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	}
	cfg, err := LoadSiteConfig(filename)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, cfg)
}
```

Note: add logging consistent with neighboring handler packages (Global Constraints) — the code above is the baseline, not a prohibition on logging.

- [ ] **Step 5: Run tests, verify they pass**

```bash
cd /Users/cding/Workspace/ChenWeb && go test ./server/api/sitehandler/ -v
```

Expected: PASS (4 tests).

- [ ] **Step 6: Commit**

```bash
jj commit -m "feat(semos): sitehandler package — site-config loading + tenant lookup"
```

---

### Task 4: Register routes + end-to-end API verification

**Files:**
- Modify: `server/api/routes.go` (public route next to `/api/config` at line ~213; authed route inside the `/api/v1` group after line ~223)

**Interfaces:**
- Consumes: `sitehandler.GetSiteConfig`, `sitehandler.GetTenantSiteConfig` (Task 3).
- Produces: `GET /api/site-config` (public), `GET /api/v1/site-config/tenant/:tenant_id` (authenticated) — consumed by Task 5.

- [ ] **Step 1: Add routes to `server/api/routes.go`**

Add to the import block:

```go
	"github.com/chendingplano/deepdoc/server/api/sitehandler"
```

After `e.GET("/api/config", confighandler.GetConfig)` (line ~213):

```go
	// Tenant-independent site config for the SemOS customer-facing frontend
	// (public: pre-login pages need it). ADR 2026071102.
	e.GET("/api/site-config", sitehandler.GetSiteConfig)
```

After the `/api/v1/health` handler registration (line ~223):

```go
	// Tenant-dependent site config (authenticated; tenant binding is a
	// placeholder until the follow-up auth ADR). ADR 2026071102.
	apiGroup.GET("/site-config/tenant/:tenant_id", sitehandler.GetTenantSiteConfig)
```

- [ ] **Step 2: Build and start the server**

```bash
cd /Users/cding/Workspace/ChenWeb && go build ./server/... && go run ./server/cmd/deepdoc &
```

- [ ] **Step 3: Verify the public endpoint**

```bash
curl -s http://localhost:8080/api/site-config | head -c 400
```

Expected: JSON starting with `{"branding":{"site_name":"SemOS"...`.

- [ ] **Step 4: Seed the demo tenant and verify auth gating**

```bash
psql -h 127.0.0.1 -p 5432 -d <project_db_from_env> -c \
  "INSERT INTO site_tenants (tenant_id, tenant_name, config_filename) \
   VALUES ('demo', 'Demo Corp', 'config/site/tenant-demo.toml') \
   ON CONFLICT (tenant_id) DO NOTHING;"

curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/api/v1/site-config/tenant/demo
```

Expected: `401` (or a redirect status from auth middleware) — the endpoint is gated. The authenticated happy path is exercised via the browser in Task 7. Stop the server afterwards (`kill %1`).

- [ ] **Step 5: Commit**

```bash
jj commit -m "feat(semos): register /api/site-config and /api/v1/site-config/tenant routes"
```

---

### Task 5: Frontend service, theme store, i18n messages

**Files:**
- Create: `web/src/lib/services/siteConfigService.ts`
- Create: `web/src/lib/stores/semosTheme.svelte.ts`
- Modify: `web/messages/en.json` (add `semos_*` keys)
- Modify: `web/messages/zh-cn.json` (add `semos_*` keys)

**Interfaces:**
- Consumes: `GET /api/site-config`, `GET /api/v1/site-config/tenant/:tenant_id` (Task 4).
- Produces: `fetchSiteConfig(fetchFn?): Promise<SiteConfig>`, `fetchTenantSiteConfig(tenantId, fetchFn?): Promise<SiteConfig>`, exported TS interface `SiteConfig` (mirrors Task 3 JSON); `semosTheme` store with `.mode`, `.toggle()`, `.init()`; message keys `semos_nav_home`, `semos_nav_workspace`, `semos_nav_knowledge_base`, `semos_nav_about`, `semos_signup_login`, `semos_get_started`, `semos_workspace_announcements`, `semos_workspace_recent`, `semos_workspace_alarms`, `semos_workspace_apps` — consumed by Tasks 6–7.

- [ ] **Step 1: Write `web/src/lib/services/siteConfigService.ts`**

```typescript
// Site config for the SemOS customer-facing frontend (ADR 2026071102).
// Shapes mirror server/api/sitehandler/sitehandler.go JSON tags exactly.

export interface SiteBranding {
	site_name: string;
	logo_text: string;
	powered_by: string;
}

export interface SiteHero {
	slogan: string;
	subtitle: string;
	image: string;
	cta_primary_label: string;
	cta_primary_href: string;
	cta_secondary_label: string;
	cta_secondary_href: string;
}

export interface SiteHighlight {
	title: string;
	description: string;
	image: string;
}

export interface SiteFeature {
	key: string;
	title: string;
	description: string;
	href: string;
}

export interface WorkspaceApp {
	name: string;
	description: string;
	href: string;
	icon: string;
}

export interface SiteWorkspace {
	banner_title: string;
	banner_subtitle: string;
	banner_image: string;
	apps: WorkspaceApp[];
}

export interface SiteConfig {
	branding: SiteBranding;
	hero: SiteHero;
	highlights: SiteHighlight[];
	features: SiteFeature[];
	footer: { text: string };
	workspace: SiteWorkspace;
}

async function getJSON<T>(url: string, fetchFn: typeof fetch): Promise<T> {
	const res = await fetchFn(url, { credentials: 'same-origin' });
	if (!res.ok) {
		throw new Error(`site config request failed: ${res.status} ${url}`);
	}
	return (await res.json()) as T;
}

/** Tenant-independent config — all public pages. */
export function fetchSiteConfig(fetchFn: typeof fetch = fetch): Promise<SiteConfig> {
	return getJSON<SiteConfig>('/api/site-config', fetchFn);
}

/** Tenant-dependent config — authenticated pages only. */
export function fetchTenantSiteConfig(
	tenantId: string,
	fetchFn: typeof fetch = fetch
): Promise<SiteConfig> {
	return getJSON<SiteConfig>(
		`/api/v1/site-config/tenant/${encodeURIComponent(tenantId)}`,
		fetchFn
	);
}
```

- [ ] **Step 2: Write `web/src/lib/stores/semosTheme.svelte.ts`**

```typescript
// Light/dark mode for the SemOS frontend. Uses the existing `.dark` class
// contract from app.css (ADR 2026071102: site-wide toggle, both modes v1).

const STORAGE_KEY = 'semos-theme';

class SemosTheme {
	mode = $state<'light' | 'dark'>('light');

	/** Call once from the /semos layout (browser only). */
	init() {
		const stored = localStorage.getItem(STORAGE_KEY);
		if (stored === 'dark' || stored === 'light') {
			this.mode = stored;
		} else {
			this.mode = window.matchMedia('(prefers-color-scheme: dark)').matches
				? 'dark'
				: 'light';
		}
		this.apply();
	}

	toggle() {
		this.mode = this.mode === 'dark' ? 'light' : 'dark';
		localStorage.setItem(STORAGE_KEY, this.mode);
		this.apply();
	}

	private apply() {
		document.documentElement.classList.toggle('dark', this.mode === 'dark');
	}
}

export const semosTheme = new SemosTheme();
```

- [ ] **Step 3: Add message keys**

Merge into `web/messages/en.json` (keep existing keys untouched):

```json
{
	"semos_nav_home": "Home",
	"semos_nav_workspace": "Workspace",
	"semos_nav_knowledge_base": "Knowledge Base",
	"semos_nav_about": "About Us",
	"semos_signup_login": "Sign Up / Login",
	"semos_get_started": "Get Started",
	"semos_workspace_announcements": "Announcements",
	"semos_workspace_recent": "Recent Activities",
	"semos_workspace_alarms": "Alarms and Errors",
	"semos_workspace_apps": "Apps"
}
```

Merge into `web/messages/zh-cn.json`:

```json
{
	"semos_nav_home": "首页",
	"semos_nav_workspace": "工作台",
	"semos_nav_knowledge_base": "知识库",
	"semos_nav_about": "关于我们",
	"semos_signup_login": "注册 / 登录",
	"semos_get_started": "立即开始",
	"semos_workspace_announcements": "公告",
	"semos_workspace_recent": "最近活动",
	"semos_workspace_alarms": "告警与错误",
	"semos_workspace_apps": "应用"
}
```

- [ ] **Step 4: Recompile paraglide + typecheck**

```bash
cd /Users/cding/Workspace/ChenWeb/web && bun run prepare && bun run check
```

Expected: 0 errors (pre-existing warnings acceptable — compare against a `bun run check` baseline taken before this task).

- [ ] **Step 5: Commit**

```bash
jj commit -m "feat(semos): site-config service, theme store, semos i18n messages"
```

---

### Task 6: `/semos` layout + Main page

**Files:**
- Create: `web/src/routes/semos/+layout.ts`
- Create: `web/src/routes/semos/+layout.svelte`
- Create: `web/src/routes/semos/components/SiteHeader.svelte`
- Create: `web/src/routes/semos/components/SiteFooter.svelte`
- Create: `web/src/routes/semos/+page.svelte`

**Interfaces:**
- Consumes: `fetchSiteConfig`, `SiteConfig` types, `semosTheme`, `m.semos_*` messages (Task 5); paraglide `locales`, `getLocale`, `setLocale` from `$lib/paraglide/runtime`.
- Produces: layout `data.siteConfig: SiteConfig` available to all `/semos/*` pages via `page.data` — consumed by Task 7.

- [ ] **Step 1: Write `web/src/routes/semos/+layout.ts`**

```typescript
import type { LayoutLoad } from './$types';
import { fetchSiteConfig } from '$lib/services/siteConfigService';

// Client-rendered like the rest of this app; SSR/SEO is a later optimization.
export const ssr = false;

export const load: LayoutLoad = async ({ fetch }) => {
	return { siteConfig: await fetchSiteConfig(fetch) };
};
```

- [ ] **Step 2: Write `web/src/routes/semos/components/SiteHeader.svelte`**

```svelte
<script lang="ts">
	import type { SiteConfig } from '$lib/services/siteConfigService';
	import { semosTheme } from '$lib/stores/semosTheme.svelte';
	import { locales, getLocale, setLocale } from '$lib/paraglide/runtime';
	import { Sun, Moon, Languages } from '@lucide/svelte';
	import * as m from '$lib/paraglide/messages';

	let { config }: { config: SiteConfig } = $props();

	const nav = [
		{ label: m.semos_nav_home(), href: '/semos' },
		{ label: m.semos_nav_workspace(), href: '/semos/workspace' },
		{ label: m.semos_nav_knowledge_base(), href: '/home3/knowledge' },
		{ label: m.semos_nav_about(), href: '/semos' }
	];

	function nextLocale(): (typeof locales)[number] {
		const idx = locales.indexOf(getLocale());
		return locales[(idx + 1) % locales.length];
	}
</script>

<header
	class="sticky top-0 z-40 w-full border-b border-border bg-background/80 backdrop-blur"
>
	<div class="mx-auto flex h-14 max-w-6xl items-center justify-between px-4">
		<a href="/semos" class="text-lg font-semibold tracking-tight text-foreground">
			{config.branding.logo_text}
		</a>
		<nav class="hidden items-center gap-6 md:flex">
			{#each nav as item (item.href + item.label)}
				<a
					href={item.href}
					class="text-sm text-muted-foreground transition-colors hover:text-foreground"
				>
					{item.label}
				</a>
			{/each}
		</nav>
		<div class="flex items-center gap-2">
			<button
				type="button"
				class="rounded-md p-2 text-muted-foreground hover:bg-accent hover:text-foreground"
				aria-label="Switch language"
				onclick={() => setLocale(nextLocale())}
			>
				<Languages class="h-4 w-4" />
			</button>
			<button
				type="button"
				class="rounded-md p-2 text-muted-foreground hover:bg-accent hover:text-foreground"
				aria-label="Toggle dark mode"
				onclick={() => semosTheme.toggle()}
			>
				{#if semosTheme.mode === 'dark'}
					<Sun class="h-4 w-4" />
				{:else}
					<Moon class="h-4 w-4" />
				{/if}
			</button>
			<a
				href={config.hero.cta_secondary_href}
				class="rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground hover:opacity-90"
			>
				{m.semos_signup_login()}
			</a>
		</div>
	</div>
</header>
```

- [ ] **Step 3: Write `web/src/routes/semos/components/SiteFooter.svelte`**

```svelte
<script lang="ts">
	import type { SiteConfig } from '$lib/services/siteConfigService';

	let { config }: { config: SiteConfig } = $props();
</script>

<footer class="border-t border-border">
	<div
		class="mx-auto flex max-w-6xl flex-col items-center justify-between gap-2 px-4 py-8 text-sm text-muted-foreground md:flex-row"
	>
		<p>{config.footer.text}</p>
		<p>{config.branding.powered_by}</p>
	</div>
</footer>
```

- [ ] **Step 4: Write `web/src/routes/semos/+layout.svelte`**

```svelte
<script lang="ts">
	import { onMount } from 'svelte';
	import { semosTheme } from '$lib/stores/semosTheme.svelte';
	import SiteHeader from './components/SiteHeader.svelte';
	import SiteFooter from './components/SiteFooter.svelte';

	let { data, children } = $props();

	onMount(() => semosTheme.init());
</script>

<div class="flex min-h-screen flex-col bg-background text-foreground">
	<SiteHeader config={data.siteConfig} />
	<main class="flex-1">
		{@render children?.()}
	</main>
	<SiteFooter config={data.siteConfig} />
</div>
```

- [ ] **Step 5: Write `web/src/routes/semos/+page.svelte`** (Main page)

```svelte
<script lang="ts">
	import * as m from '$lib/paraglide/messages';

	let { data } = $props();
	const cfg = $derived(data.siteConfig);
</script>

<svelte:head>
	<title>{cfg.branding.site_name}</title>
</svelte:head>

<!-- Hero banner -->
<section class="mx-auto max-w-6xl px-4 py-20 md:py-28">
	<div class="max-w-3xl">
		<h1 class="text-4xl font-semibold tracking-tight md:text-6xl">
			{cfg.hero.slogan}
		</h1>
		<p class="mt-6 text-lg text-muted-foreground">{cfg.hero.subtitle}</p>
		<div class="mt-8 flex flex-wrap gap-3">
			<a
				href={cfg.hero.cta_primary_href}
				class="rounded-md bg-primary px-5 py-2.5 text-sm font-medium text-primary-foreground hover:opacity-90"
			>
				{m.semos_get_started()}
			</a>
			<a
				href={cfg.hero.cta_secondary_href}
				class="rounded-md border border-border px-5 py-2.5 text-sm font-medium hover:bg-accent"
			>
				{m.semos_signup_login()}
			</a>
		</div>
	</div>
</section>

<!-- Product highlights (~5, alternating) -->
<section class="border-t border-border">
	<div class="mx-auto max-w-6xl space-y-20 px-4 py-20">
		{#each cfg.highlights as h, i (h.title)}
			<div
				class="grid items-center gap-8 md:grid-cols-2 {i % 2 === 1
					? 'md:[&>*:first-child]:order-2'
					: ''}"
			>
				<div>
					<h2 class="text-2xl font-semibold tracking-tight">{h.title}</h2>
					<p class="mt-3 text-muted-foreground">{h.description}</p>
				</div>
				<img
					src={h.image}
					alt={h.title}
					class="aspect-video w-full rounded-lg border border-border object-cover"
				/>
			</div>
		{/each}
	</div>
</section>

<!-- Main features (4 cards) -->
<section class="border-t border-border">
	<div class="mx-auto max-w-6xl px-4 py-20">
		<div class="grid gap-6 sm:grid-cols-2 lg:grid-cols-4">
			{#each cfg.features as f (f.key)}
				<a
					href={f.href}
					class="rounded-lg border border-border p-6 transition-colors hover:bg-accent"
				>
					<h3 class="font-semibold">{f.title}</h3>
					<p class="mt-2 text-sm text-muted-foreground">{f.description}</p>
				</a>
			{/each}
		</div>
	</div>
</section>
```

- [ ] **Step 6: Typecheck**

```bash
cd /Users/cding/Workspace/ChenWeb/web && bun run check
```

Expected: 0 new errors versus baseline.

- [ ] **Step 7: Verify in the browser**

With the Go server running (`go run ./server/cmd/deepdoc`) and dev server (`cd web && bun run dev`):

- Open `http://localhost:5173/semos` — hero slogan "Your Knowledge, Working for You", 5 highlights, 4 feature cards, footer with "Powered by SemOS".
- Click the moon icon — page flips to dark (background near-black); reload — dark persists.
- Click the language icon — nav labels switch to Chinese (首页 / 工作台 / …).

- [ ] **Step 8: Commit**

```bash
jj commit -m "feat(semos): /semos layout with header/footer/theme/i18n + Main page"
```

---

### Task 7: Workspace Landing page

**Files:**
- Create: `web/src/routes/semos/workspace/+page.svelte`

**Interfaces:**
- Consumes: layout `data.siteConfig` (Task 6), `fetchTenantSiteConfig` (Task 5), `m.semos_workspace_*` messages (Task 5).

- [ ] **Step 1: Write `web/src/routes/semos/workspace/+page.svelte`**

The page renders from the layout's tenant-independent config by default. A `?tenant=<id>` query param fetches that tenant's config instead (dev/preview path for the tenant mechanism — the real user→tenant binding is deferred to the auth ADR).

```svelte
<script lang="ts">
	import { page } from '$app/state';
	import * as m from '$lib/paraglide/messages';
	import {
		fetchTenantSiteConfig,
		type SiteConfig
	} from '$lib/services/siteConfigService';
	import {
		Database,
		MessageCircle,
		Search,
		FileCheck,
		Workflow,
		Bot,
		LayoutGrid
	} from '@lucide/svelte';

	let { data } = $props();

	let tenantConfig = $state<SiteConfig | null>(null);
	let tenantError = $state<string | null>(null);

	const tenantId = $derived(page.url.searchParams.get('tenant'));
	const cfg = $derived(tenantConfig ?? data.siteConfig);

	$effect(() => {
		tenantConfig = null;
		tenantError = null;
		if (tenantId) {
			fetchTenantSiteConfig(tenantId)
				.then((c) => (tenantConfig = c))
				.catch((e) => (tenantError = String(e)));
		}
	});

	// Icon names in site-config TOML -> lucide components.
	const icons: Record<string, typeof LayoutGrid> = {
		database: Database,
		'message-circle': MessageCircle,
		search: Search,
		'file-check': FileCheck,
		workflow: Workflow,
		bot: Bot
	};

	// Placeholder feed content; real data sources wired in a later phase.
	const announcements = ['Welcome to your SemOS workspace.'];
	const recent: string[] = [];
	const alarms: string[] = [];
</script>

<svelte:head>
	<title>{cfg.workspace.banner_title} — {cfg.branding.site_name}</title>
</svelte:head>

<!-- Workspace banner -->
<section class="relative border-b border-border">
	<img
		src={cfg.workspace.banner_image}
		alt=""
		class="absolute inset-0 h-full w-full object-cover opacity-15"
	/>
	<div class="relative mx-auto max-w-6xl px-4 py-14">
		<h1 class="text-3xl font-semibold tracking-tight">{cfg.workspace.banner_title}</h1>
		<p class="mt-2 text-muted-foreground">{cfg.workspace.banner_subtitle}</p>
		{#if tenantError}
			<p class="mt-2 text-sm text-destructive">{tenantError}</p>
		{/if}
	</div>
</section>

<div class="mx-auto max-w-6xl px-4 py-10">
	<!-- Announcements / Recent activities / Alarms -->
	<div class="grid gap-6 md:grid-cols-3">
		{#each [
			{ title: m.semos_workspace_announcements(), items: announcements, empty: '—' },
			{ title: m.semos_workspace_recent(), items: recent, empty: 'No recent activity.' },
			{ title: m.semos_workspace_alarms(), items: alarms, empty: 'No alarms.' }
		] as block (block.title)}
			<div class="rounded-lg border border-border p-5">
				<h2 class="text-sm font-semibold text-muted-foreground">{block.title}</h2>
				<ul class="mt-3 space-y-2 text-sm">
					{#if block.items.length === 0}
						<li class="text-muted-foreground">{block.empty}</li>
					{:else}
						{#each block.items as item (item)}
							<li>{item}</li>
						{/each}
					{/if}
				</ul>
			</div>
		{/each}
	</div>

	<!-- Apps grid: large rounded rectangles -->
	<h2 class="mt-12 text-lg font-semibold">{m.semos_workspace_apps()}</h2>
	<div class="mt-4 grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
		{#each cfg.workspace.apps as app (app.name)}
			{@const Icon = icons[app.icon] ?? LayoutGrid}
			<a
				href={app.href}
				class="flex flex-col gap-3 rounded-2xl border border-border p-8 transition-colors hover:bg-accent"
			>
				<Icon class="h-8 w-8 text-primary" />
				<span class="text-base font-semibold">{app.name}</span>
				<span class="text-sm text-muted-foreground">{app.description}</span>
			</a>
		{/each}
	</div>
</div>
```

- [ ] **Step 2: Typecheck**

```bash
cd /Users/cding/Workspace/ChenWeb/web && bun run check
```

Expected: 0 new errors versus baseline.

- [ ] **Step 3: Verify in the browser**

With Go server + dev server running:

- `http://localhost:5173/semos/workspace` — banner, three feed cards, 6 app tiles with icons; header/footer/theme/language all work.
- Log in via the existing `/login` flow, then open `http://localhost:5173/semos/workspace?tenant=demo` — banner title changes to demo-tenant branding ("Demo Corp: …" hero fields; workspace section text unchanged since only `[branding]`/`[hero]` differ in `tenant-demo.toml`) and `site_name` in the tab title reads "Demo Corp Knowledge Portal".
- Without a session, `?tenant=demo` shows the error line (401) instead of crashing.

- [ ] **Step 4: Commit**

```bash
jj commit -m "feat(semos): Workspace Landing page with apps grid and tenant preview param"
```

---

### Task 8: Full verification + documentation impact

**Files:**
- Modify: `KnowledgeStore/doc-repo/adrs/202607/2026071102-adr-new-gui-semos.md` (change log entry; separate repo — commit there too)

- [ ] **Step 1: Run the full check suite**

```bash
cd /Users/cding/Workspace/ChenWeb && go vet ./server/... && go test ./server/api/sitehandler/ ./server/cmd/config/
cd web && bun run check && bun run build
```

Expected: all pass; build completes.

- [ ] **Step 2: Documentation impact (per workspace protocol)**

- *What knowledge changed:* Phase 1 of the SemOS frontend exists (`/semos`, `/semos/workspace`), site-config file format, `site_tenants` table, two new API endpoints.
- *Docs affected:* design brief ADR 2026071102 (implementation started).
- *Update:* append to the brief's Change Logs:

```markdown
* 2026/07/13, Implementation Phase 1 landed in ChenWeb: site_tenants
  migration, [config].config_filename plumbing, config/site/*.toml files,
  sitehandler package, GET /api/site-config and
  GET /api/v1/site-config/tenant/:tenant_id, /semos layout (i18n +
  light/dark) with Main and Workspace Landing pages. Phases 2 (remaining
  pages) and 3 (User/Site Management, post-auth-ADR) pending.
```

- *Intentionally undocumented:* per-page copy (placeholder content, config-editable by design).

- [ ] **Step 3: Commit both repos**

```bash
cd /Users/cding/Workspace/ChenWeb && jj commit -m "chore(semos): phase-1 verification"
cd /Users/cding/Workspace/KnowledgeStore && jj commit -m "docs: ADR 2026071102 change log — SemOS frontend phase 1 implemented"
```

(If KnowledgeStore uses plain git rather than jj, use `git add -A && git commit -m ...` there instead.)

---

## Self-Review Notes

- **Spec coverage (phase 1 scope):** tenant table ✓ (T1), `[config].config_filename` ✓ (T2), file-per-tenant config in Git ✓ (T2), tenant filename from DB only ✓ (T3), public vs authed endpoints matching tenant-independent/dependent split ✓ (T4), i18n site-wide ✓ (T5/T6), light+dark ✓ (T5/T6), Main page ✓ (T6), Workspace Landing ✓ (T7), configurable app list ✓ (TOML `workspace.apps`), docs protocol ✓ (T8). Remaining sitemap pages and admin pages are explicitly Phase 2/3.
- **Type consistency:** Go JSON tags ↔ TS interfaces ↔ TOML keys use identical snake_case names; `SiteConfig` name shared across layers deliberately.
- **Known deferred items (by design, from the brief):** user↔tenant binding, role gating (`admin`), Site Management editor, `[frontend].supported_languages` reconciliation with paraglide locale list (only `en`/`zh-cn` have message catalogs today).
