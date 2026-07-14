# SemOS Workspace Page Re-design — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port `/semos/workspace` off the deleted `/semos1` dark variant onto the canonical paper-and-ink visual system, move its announcements into TOML config, and move its empty-state copy into the i18n catalog.

**Architecture:** Four independent slices, back to front. The Go config struct gains two fields (real TDD — a test fixture and test already exist). The TS interface mirrors them. The shared bronze `Ornament` is extracted from its two current copy-pasted homes. Then the Workspace page is rewritten against all three.

**Tech Stack:** Go 1.25 + `toml.Unmarshal`, SvelteKit 5 (runes), Tailwind v4, `@inlang/paraglide-js`, `@lucide/svelte`, `bun`.

**Spec:** [2026-07-14-semos-workspace-redesign-design.md](../specs/2026-07-14-semos-workspace-redesign-design.md)
**Parent ADR:** `KnowledgeStore/doc-repo/adrs/202607/2026071102-adr-new-gui-semos.md`

## Global Constraints

- **Palette — the only colors permitted on this page.** Paper `#faf9f7`, band paper `#f3f1ec`, card gradient `white`→`#faf8f4`, ink `#17181c`, muted ink `#6f6c66`, bronze `#b08d57`. Dark mode: `#101216` (page), `#15181e` (band), `#1c2029`→`#171a21` (card), `#e9e7e2` (text), `#a5a29b` (muted).
- **Banned colors.** `#080b14`, `#6b7aff`, `#131726`, `#f4f2ed`, `#1a1a1a`, `#6b6b6b`, `#e8e7e4`, `#9a9aa0`, `#0a0d18`, `#1a1f30`. None may survive anywhere in `workspace/+page.svelte`. Grepping for them is a verification step.
- **One exception to the palette:** the Alarms count badge uses amber/red when the count is non-zero. It is a signal, not decoration.
- **No fabricated data.** Recent Activity and Alarms ship empty. Do not invent announcements, activity items, alarm items, or figures. The parent ADR carries a standing warning about a prior pass that shipped invented numbers.
- **Chrome → i18n catalog. Tenant content → TOML.** Feed titles and empty-state copy are chrome. Announcements are content. Never hardcode a user-visible string in a `.svelte` file.
- **Only two message catalogs exist:** `web/messages/en.json`, `web/messages/zh-cn.json`. Do not create `ja.json` or `ko.json`.
- **Go error codes** follow the existing `(CWB_SITE_00N)` convention in `sitehandler.go`.
- **No test runner exists for the web project.** Frontend verification is `bun run check` plus browser checks. Do not write Svelte unit tests; do not add vitest.
- **Commit with `jj`,** not `git`. Working dir for `jj` commands is `/Users/cding/Workspace/ChenWeb`.

---

## File Structure

| File | Responsibility | Task |
|---|---|---|
| `server/api/sitehandler/sitehandler.go` | `Workspace` struct gains `Kicker`, `Announcements` | 1 |
| `server/api/sitehandler/sitehandler_test.go` | asserts the new fields parse | 1 |
| `server/api/sitehandler/testdata/site-valid.toml` | fixture gains the new fields | 1 |
| `config/site/site-default.toml` | tenant-independent content | 2 |
| `config/site/site-default-zh-cn.toml` | zh-cn content | 2 |
| `config/site/tenant-demo.toml` | demo tenant content | 2 |
| `web/src/lib/services/siteConfigService.ts` | `SiteWorkspace` interface mirrors Go | 2 |
| `web/messages/{en,zh-cn}.json` | empty-state chrome copy | 2 |
| `web/src/routes/semos/components/Ornament.svelte` | **new** — the one bronze ornament | 3 |
| `web/src/routes/semos/+page.svelte` | consumes `Ornament` | 3 |
| `web/src/routes/semos/components/SiteFooter.svelte` | consumes `Ornament` | 3 |
| `web/src/routes/semos/workspace/+page.svelte` | **rewrite** — the re-skin | 4 |
| `KnowledgeStore/.../2026071102-adr-new-gui-semos.md` | changelog + schema table | 5 |

---

## Task 1: Go config schema — `kicker` and `announcements`

**Files:**
- Modify: `server/api/sitehandler/sitehandler.go:87-92` (the `Workspace` struct)
- Modify: `server/api/sitehandler/testdata/site-valid.toml` (the `[workspace]` block)
- Test: `server/api/sitehandler/sitehandler_test.go:10-30` (`TestLoadSiteConfigValid`)

**Interfaces:**
- Consumes: nothing.
- Produces: `sitehandler.Workspace` with two new fields, serialized by `GET /api/site-config` as JSON keys `kicker` (string) and `announcements` (array of string). Task 2's TS interface must match these exact JSON key names.

- [ ] **Step 1: Write the failing test**

In `server/api/sitehandler/sitehandler_test.go`, add these assertions inside the existing `TestLoadSiteConfigValid`, immediately after the `len(cfg.Workspace.Apps)` check (currently line 24-26):

```go
	if cfg.Workspace.Kicker != "Workspace" {
		t.Errorf("Workspace.Kicker = %q, want Workspace", cfg.Workspace.Kicker)
	}
	if len(cfg.Workspace.Announcements) != 1 {
		t.Fatalf("len(Workspace.Announcements) = %d, want 1", len(cfg.Workspace.Announcements))
	}
	if cfg.Workspace.Announcements[0] != "Welcome to your SemOS workspace." {
		t.Errorf("Announcements[0] = %q", cfg.Workspace.Announcements[0])
	}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd /Users/cding/Workspace/ChenWeb && go test ./server/api/sitehandler/ -run TestLoadSiteConfigValid -v
```

Expected: **compile failure** — `cfg.Workspace.Kicker undefined` and `cfg.Workspace.Announcements undefined`. That is the correct first failure; the struct does not have these fields yet.

- [ ] **Step 3: Add the fields to the struct**

In `server/api/sitehandler/sitehandler.go`, replace the `Workspace` struct (currently lines 87-92):

```go
type Workspace struct {
	Kicker         string         `toml:"kicker" json:"kicker"`
	BannerTitle    string         `toml:"banner_title" json:"banner_title"`
	BannerSubtitle string         `toml:"banner_subtitle" json:"banner_subtitle"`
	BannerImage    string         `toml:"banner_image" json:"banner_image"`
	Announcements  []string       `toml:"announcements" json:"announcements"`
	Apps           []WorkspaceApp `toml:"apps" json:"apps"`
}
```

- [ ] **Step 4: Run the test — it should now fail on values, not compilation**

```bash
cd /Users/cding/Workspace/ChenWeb && go test ./server/api/sitehandler/ -run TestLoadSiteConfigValid -v
```

Expected: **compiles, then FAILS** with `Workspace.Kicker = "", want Workspace`. The struct exists; the fixture has no data for it yet. This intermediate failure confirms the test is actually reading the fixture rather than passing vacuously.

- [ ] **Step 5: Add the fields to the test fixture**

In `server/api/sitehandler/testdata/site-valid.toml`, find the `[workspace]` table. Add `kicker` as its first key and `announcements` after `banner_image`. `announcements` must come **before** the first `[[workspace.apps]]` block — in TOML, a key written after an array-of-tables header belongs to that table, not to `[workspace]`.

```toml
[workspace]
kicker = "Workspace"
banner_title = "Your Workspace"
banner_subtitle = "Everything you need to work with your knowledge base, in one place."
banner_image = "/images/angleWalls.jpg"
announcements = ["Welcome to your SemOS workspace."]
```

Leave the existing `[[workspace.apps]]` blocks untouched — `TestLoadSiteConfigValid` asserts there are exactly 6 and that must keep passing.

- [ ] **Step 6: Run the full package test suite**

```bash
cd /Users/cding/Workspace/ChenWeb && go test ./server/api/sitehandler/ -v
```

Expected: **all 4 tests PASS** — `TestLoadSiteConfigValid`, `TestLoadSiteConfigMissingFile`, `TestGetTenantConfigFilename`, `TestGetTenantConfigFilenameNotFound`.

- [ ] **Step 7: Verify the whole server still builds**

```bash
cd /Users/cding/Workspace/ChenWeb && go build ./... && go vet ./server/api/sitehandler/
```

Expected: no output (success).

- [ ] **Step 8: Commit**

```bash
cd /Users/cding/Workspace/ChenWeb && jj commit server/api/sitehandler/ -m "feat(sitehandler): add workspace kicker and announcements to site config

Workspace announcements were hardcoded in the Svelte component, violating
the Content Configurability rule in ADR 2026071102. Announcements are tenant
content, so they belong in the per-tenant TOML file.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 2: Config files, TS interface, and i18n messages

Everything the frontend needs before the page itself can be rewritten. These
are grouped into one task because none of them is independently reviewable —
a TS interface with no config data behind it, or config data no type admits,
is not a meaningful checkpoint.

**Files:**
- Modify: `config/site/site-default.toml`
- Modify: `config/site/site-default-zh-cn.toml`
- Modify: `config/site/tenant-demo.toml`
- Modify: `web/src/lib/services/siteConfigService.ts:61-66` (`SiteWorkspace`)
- Modify: `web/messages/en.json`
- Modify: `web/messages/zh-cn.json`

**Interfaces:**
- Consumes: from Task 1 — JSON keys `kicker: string` and `announcements: string[]` on the workspace object returned by `GET /api/site-config`.
- Produces:
  - TS `SiteWorkspace` gains `kicker: string` and `announcements: string[]`. Task 4 reads `cfg.workspace.kicker` and `cfg.workspace.announcements`.
  - Three message functions, importable as `m.semos_workspace_no_announcements()`, `m.semos_workspace_no_activity()`, `m.semos_workspace_no_alarms()`. Task 4 calls all three. Paraglide generates these from the JSON at build time; they do not exist until the JSON has them.

- [ ] **Step 1: Add the new keys to `config/site/site-default.toml`**

Find the `[workspace]` table. Add `kicker` first and `announcements` after `banner_image`, **before** the first `[[workspace.apps]]` block:

```toml
[workspace]
kicker = "Workspace"
banner_title = "Your Workspace"
banner_subtitle = "Everything you need to work with your knowledge base, in one place."
banner_image = "/images/angleWalls.jpg"
announcements = ["Welcome to your SemOS workspace."]
```

Do not change the `[[workspace.apps]]` blocks.

- [ ] **Step 2: Add the same keys to `config/site/tenant-demo.toml`**

Same edit, same placement. Read the file's existing `[workspace]` block first and preserve its own `banner_title` / `banner_subtitle` / `banner_image` values — only *add* `kicker` and `announcements`. If this tenant's copy is in a different language, write `kicker` and the announcement in that language.

- [ ] **Step 3: Add the same keys to `config/site/site-default-zh-cn.toml`, in Chinese**

```toml
kicker = "工作台"
announcements = ["欢迎使用 SemOS 工作台。"]
```

Placement identical: `kicker` first in `[workspace]`, `announcements` after `banner_image`, before `[[workspace.apps]]`.

- [ ] **Step 4: Mirror the fields in the TypeScript interface**

In `web/src/lib/services/siteConfigService.ts`, replace `SiteWorkspace` (currently lines 61-66):

```ts
export interface SiteWorkspace {
	kicker: string;
	banner_title: string;
	banner_subtitle: string;
	banner_image: string;
	announcements: string[];
	apps: WorkspaceApp[];
}
```

- [ ] **Step 5: Add the three empty-state messages to `web/messages/en.json`**

Add after the existing `"semos_workspace_apps"` entry (remember to add a comma to that line):

```json
	"semos_workspace_apps": "Apps",
	"semos_workspace_no_announcements": "No announcements.",
	"semos_workspace_no_activity": "No recent activity.",
	"semos_workspace_no_alarms": "No alarms. Everything is running normally."
```

- [ ] **Step 6: Add the same three keys to `web/messages/zh-cn.json`**

```json
	"semos_workspace_apps": "应用",
	"semos_workspace_no_announcements": "暂无公告。",
	"semos_workspace_no_activity": "暂无最近活动。",
	"semos_workspace_no_alarms": "暂无告警，系统运行正常。"
```

- [ ] **Step 7: Verify both JSON catalogs are still valid JSON**

```bash
cd /Users/cding/Workspace/ChenWeb/web && bun -e "JSON.parse(require('fs').readFileSync('messages/en.json')); JSON.parse(require('fs').readFileSync('messages/zh-cn.json')); console.log('both catalogs parse OK')"
```

Expected: `both catalogs parse OK`. A trailing-comma mistake in Step 5 or 6 shows up here.

- [ ] **Step 8: Verify the Go loader accepts all three real config files**

The Go tests only cover the fixture. This confirms the *real* config files parse
with the new schema — if a key landed inside `[[workspace.apps]]` by mistake, it
surfaces here, not in the browser.

```bash
cd /Users/cding/Workspace/ChenWeb && cat > /tmp/checkcfg_test.go <<'EOF'
package sitehandler

import "testing"

func TestRealConfigFilesParse(t *testing.T) {
	for _, p := range []string{
		"../../../config/site/site-default.toml",
		"../../../config/site/site-default-zh-cn.toml",
		"../../../config/site/tenant-demo.toml",
	} {
		cfg, err := LoadSiteConfig(p)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if cfg.Workspace.Kicker == "" {
			t.Errorf("%s: Workspace.Kicker is empty", p)
		}
		if len(cfg.Workspace.Announcements) == 0 {
			t.Errorf("%s: Workspace.Announcements is empty", p)
		}
		if len(cfg.Workspace.Apps) != 6 {
			t.Errorf("%s: len(Apps) = %d, want 6", p, len(cfg.Workspace.Apps))
		}
	}
}
EOF
cp /tmp/checkcfg_test.go server/api/sitehandler/zz_realcfg_test.go
go test ./server/api/sitehandler/ -run TestRealConfigFilesParse -v
rm server/api/sitehandler/zz_realcfg_test.go /tmp/checkcfg_test.go
```

Expected: `PASS`. The `len(Apps) = 6` assertion is the one that catches the TOML
placement error — if `announcements` were written after an `[[workspace.apps]]`
header it would corrupt that array.

This is a scratch check, deliberately deleted afterward: it asserts against live
content files that a tenant admin is expected to edit, so keeping it would make
routine content edits break the build.

- [ ] **Step 9: Typecheck the frontend**

```bash
cd /Users/cding/Workspace/ChenWeb/web && bun run check
```

Expected: **no new errors** mentioning `siteConfigService` or `SiteWorkspace`. This run also regenerates the paraglide message functions from the JSON, so `m.semos_workspace_no_alarms` becomes available to Task 4. Pre-existing unrelated errors elsewhere in the project may appear — note them, do not fix them.

- [ ] **Step 10: Commit**

```bash
cd /Users/cding/Workspace/ChenWeb && jj commit config/site/ web/src/lib/services/siteConfigService.ts web/messages/ -m "feat(semos): move workspace announcements to config, empty-state copy to i18n

Announcements are tenant content, so they live in TOML. Empty-state copy is
UI chrome, so it lives in the message catalog. Both were previously hardcoded
English string literals in workspace/+page.svelte.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 3: Extract the shared `Ornament` component

The bronze diamond-and-dots ornament is currently copy-pasted in two places.
The Workspace page needs a third. The ornament is the site's signature mark —
if the copies drift, "consistent with the main page" silently stops being
true. Extract it before adding a consumer.

**Files:**
- Create: `web/src/routes/semos/components/Ornament.svelte`
- Modify: `web/src/routes/semos/+page.svelte:119-127` (delete the local `ornament` snippet) and its two `{@render ornament()}` call sites (lines ~138, ~181, ~237)
- Modify: `web/src/routes/semos/components/SiteFooter.svelte:21-28` (the inline copy)

**Interfaces:**
- Consumes: nothing.
- Produces: `Ornament.svelte`, default export, **no props**. Used as `<Ornament />`. It renders only the mark itself — it carries **no vertical padding**, so every caller controls its own spacing. This matters: the Main page wraps it in `py-14 md:py-20`, the footer in `pt-14`, and Task 4 will use tighter spacing. A component with baked-in padding could not serve all three.

- [ ] **Step 1: Create the component**

Create `web/src/routes/semos/components/Ornament.svelte`. This markup is lifted verbatim from the `ornament` snippet in `semos/+page.svelte` — the bronze diamond flanked by two pairs of fading dots — with the wrapper's `py-2` removed so callers own their spacing:

```svelte
<!--
	The bronze ornament: a rotated diamond flanked by fading dots. Used
	instead of a horizontal rule wherever the paper-and-ink pages mark a
	passage between blocks (ADR 2026071102).

	Carries no vertical padding on purpose — the Main page, the footer and
	the Workspace page each space it differently. Wrap it, don't pad it.
-->
<div class="flex items-center justify-center gap-3" aria-hidden="true">
	<span class="h-1 w-1 rounded-full bg-[#b08d57]/40"></span>
	<span class="h-1.5 w-1.5 rounded-full bg-[#b08d57]/60"></span>
	<span class="inline-block h-2.5 w-2.5 rotate-45 bg-[#b08d57]"></span>
	<span class="h-1.5 w-1.5 rounded-full bg-[#b08d57]/60"></span>
	<span class="h-1 w-1 rounded-full bg-[#b08d57]/40"></span>
</div>
```

- [ ] **Step 2: Use it in the Main page**

In `web/src/routes/semos/+page.svelte`:

1. Add the import to the top of the `<script>` block, after the `@lucide/svelte` import:

```ts
	import Ornament from './components/Ornament.svelte';
```

2. **Delete** the entire `{#snippet ornament()} ... {/snippet}` block (currently lines 119-127, including the `<!-- ORNAMENT DIVIDER -->` comment banner above it).

3. Replace each of the three `{@render ornament()}` call sites with `<Ornament />`. The wrappers around them keep their padding exactly as-is. For example, the divider between highlights becomes:

```svelte
				{#if i > 0}
					<div use:reveal class="reveal py-14 md:py-20">
						<Ornament />
					</div>
				{/if}
```

and the one between highlights and features:

```svelte
<div use:reveal class="reveal py-12 md:py-16">
	<Ornament />
</div>
```

and the one in the closing CTA (inside `<div use:reveal class="reveal mx-auto max-w-xl">`):

```svelte
				<Ornament />
```

Note the CTA's ornament previously rendered with the snippet's own `py-2`, and
the `<h2>` beneath it already carries `mt-8`. Losing that `py-2` is a 8px
tightening in one spot. Accept it — do not add a compensating wrapper. Confirm
it looks right in Step 5.

- [ ] **Step 3: Use it in the footer**

In `web/src/routes/semos/components/SiteFooter.svelte`:

1. Add the import after the `LogoMark` import:

```ts
	import Ornament from './Ornament.svelte';
```

2. Replace the inline ornament markup (currently lines 22-28 — the `<div class="flex items-center justify-center gap-3 pt-14">` and its five spans) with a padded wrapper around the component, preserving the `pt-14`:

```svelte
	<div class="pt-14">
		<Ornament />
	</div>
```

- [ ] **Step 4: Typecheck**

```bash
cd /Users/cding/Workspace/ChenWeb/web && bun run check
```

Expected: no new errors. In particular no "`ornament` is not defined" — that would mean a `{@render ornament()}` call site was missed in Step 2.

- [ ] **Step 5: Verify in the browser that nothing moved**

This is a pure refactor: the Main page and footer must look **identical** to before.

```bash
cd /Users/cding/Workspace/ChenWeb/web && bun run dev
```

Visit `http://localhost:5173/semos`. Confirm:
- Four ornaments render: between each pair of highlights, between highlights and features, and above the closing CTA heading.
- The footer ornament renders above the footer columns.
- All are bronze `#b08d57`, diamond centered between two pairs of dots.
- Nothing else about the page's spacing or appearance changed.

- [ ] **Step 6: Commit**

```bash
cd /Users/cding/Workspace/ChenWeb && jj commit web/src/routes/semos/ -m "refactor(semos): extract the bronze Ornament into a shared component

It was copy-pasted in the main page and the footer, and the workspace page
needs a third copy. The ornament is the site's signature mark; three
independent copies could drift apart.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 4: Rewrite the Workspace page

**Files:**
- Rewrite: `web/src/routes/semos/workspace/+page.svelte`

**Interfaces:**
- Consumes:
  - Task 2 — `cfg.workspace.kicker: string`, `cfg.workspace.announcements: string[]`, and `m.semos_workspace_no_announcements()`, `m.semos_workspace_no_activity()`, `m.semos_workspace_no_alarms()`.
  - Task 3 — `Ornament.svelte` (no props).
  - Pre-existing — `cfg.workspace.banner_title`, `banner_subtitle`, `banner_image`, `apps[]`; `m.semos_workspace_announcements()`, `m.semos_workspace_recent()`, `m.semos_workspace_alarms()`, `m.semos_workspace_apps()`.
- Produces: nothing consumed by a later task.

**Behavior that must not regress:** the `?tenant=<id>` query param still fetches that tenant's config via `fetchTenantSiteConfig` and still surfaces its error state. Keep the existing `$state` / `$derived` / `$effect` block at the top of the script exactly as it is — only its *rendering* changes.

- [ ] **Step 1: Replace the file**

Write `web/src/routes/semos/workspace/+page.svelte`:

```svelte
<script lang="ts">
	import { page } from '$app/state';
	import { m } from '$lib/paraglide/messages.js';
	import { fetchTenantSiteConfig, type SiteConfig } from '$lib/services/siteConfigService';
	import Ornament from '../components/Ornament.svelte';
	import {
		Database,
		MessageCircle,
		Search,
		FileCheck,
		Workflow,
		Bot,
		LayoutGrid,
		ArrowUpRight,
		Megaphone,
		Activity,
		AlertTriangle
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

	const icons: Record<string, typeof LayoutGrid> = {
		database: Database,
		'message-circle': MessageCircle,
		search: Search,
		'file-check': FileCheck,
		workflow: Workflow,
		bot: Bot
	};

	// Recent activity and alarms have no backing endpoint yet. They render an
	// empty state rather than sample content: ADR 2026071102 records a prior
	// pass that shipped invented figures, and demo data that reads as real is
	// exactly that mistake. Wire these to a real feed when one exists.
	const recentActivity: string[] = [];
	const alarms: string[] = [];

	const feeds = $derived([
		{
			title: m.semos_workspace_announcements(),
			icon: Megaphone,
			items: cfg.workspace.announcements ?? [],
			empty: m.semos_workspace_no_announcements(),
			alert: false
		},
		{
			title: m.semos_workspace_recent(),
			icon: Activity,
			items: recentActivity,
			empty: m.semos_workspace_no_activity(),
			alert: false
		},
		{
			title: m.semos_workspace_alarms(),
			icon: AlertTriangle,
			items: alarms,
			empty: m.semos_workspace_no_alarms(),
			// An alarm count is a signal, not decoration — it is the one thing on
			// this page allowed to break the bronze palette when it is non-zero.
			alert: true
		}
	]);

	function reveal(node: HTMLElement) {
		const observer = new IntersectionObserver(
			(entries) => {
				for (const entry of entries) {
					if (entry.isIntersecting) {
						entry.target.classList.add('is-visible');
						observer.unobserve(entry.target);
					}
				}
			},
			{ threshold: 0.1, rootMargin: '0px 0px -10% 0px' }
		);
		observer.observe(node);
		return {
			destroy() {
				observer.disconnect();
			}
		};
	}
</script>

<svelte:head>
	<title>{cfg.workspace.banner_title} — {cfg.branding.site_name}</title>
</svelte:head>

<!-- ═════════════════════════════════════════════════
     BANNER — the main hero's paper-veil treatment at
     app-shell height. Marketing pages breathe; this
     one gets you to your apps (ADR 2026071102).
     ═════════════════════════════════════════════════ -->
<section class="relative overflow-hidden">
	<img src={cfg.workspace.banner_image} alt="" class="absolute inset-0 h-full w-full object-cover" />
	<div class="absolute inset-0 bg-[#faf9f7]/70 dark:bg-[#101216]/70"></div>
	<div
		class="absolute inset-0 bg-gradient-to-r from-[#faf9f7] via-[#faf9f7]/50 to-transparent dark:from-[#101216] dark:via-[#101216]/50"
	></div>

	<div class="relative mx-auto max-w-7xl px-6 py-14 md:py-16">
		<div use:reveal class="reveal max-w-2xl">
			<div class="flex items-center gap-3">
				<span class="inline-block h-1.5 w-1.5 rotate-45 bg-[#b08d57]"></span>
				<span class="text-xs font-bold tracking-[0.22em] uppercase text-[#6f6c66] dark:text-[#a5a29b]">
					{cfg.workspace.kicker}
				</span>
			</div>

			<h1
				class="mt-5 text-[clamp(1.9rem,3vw+0.6rem,2.75rem)] font-bold leading-[1.1] tracking-tight text-[#17181c] dark:text-[#e9e7e2]"
			>
				{cfg.workspace.banner_title}
			</h1>

			<p class="mt-4 max-w-[56ch] leading-relaxed text-[#6f6c66] dark:text-[#a5a29b]">
				{cfg.workspace.banner_subtitle}
			</p>

			{#if tenantError}
				<p class="mt-5 flex items-center gap-1.5 text-sm text-[#b4462f] dark:text-[#e08a76]">
					<AlertTriangle class="h-4 w-4 shrink-0" />
					{tenantError}
				</p>
			{/if}
		</div>
	</div>
</section>

<div use:reveal class="reveal py-10 md:py-12">
	<Ornament />
</div>

<!-- ═════════════════════════════════════════════════
     FEEDS — the main page's card material: white-to-
     paper gradient, white top bevel, layered shadow.
     ═════════════════════════════════════════════════ -->
<section class="relative">
	<div class="mx-auto max-w-7xl px-6">
		<div use:reveal class="reveal grid gap-6 md:grid-cols-3">
			{#each feeds as block, i (block.title)}
				<div
					style="transition-delay: {i * 60}ms"
					class="flex flex-col rounded-2xl border-t border-white bg-gradient-to-b from-white to-[#faf8f4] p-6 shadow-[0_1px_2px_rgba(23,24,28,0.06),0_3px_6px_rgba(23,24,28,0.05),0_12px_28px_rgba(23,24,28,0.09)] dark:border-white/10 dark:from-[#1c2029] dark:to-[#171a21] dark:shadow-[0_1px_2px_rgba(0,0,0,0.4),0_12px_28px_rgba(0,0,0,0.5)]"
				>
					<div class="flex items-start justify-between gap-2">
						<h2
							class="flex items-center gap-2 text-xs font-bold tracking-[0.18em] uppercase text-[#b08d57]"
						>
							<block.icon class="h-3.5 w-3.5" />
							{block.title}
						</h2>
						{#if block.items.length > 0}
							<span
								class="rounded-full px-2 py-0.5 text-[11px] font-bold tabular-nums {block.alert
									? 'bg-[#b4462f]/12 text-[#b4462f] dark:bg-[#e08a76]/15 dark:text-[#e08a76]'
									: 'bg-[#b08d57]/12 text-[#b08d57]'}"
							>
								{block.items.length}
							</span>
						{/if}
					</div>

					{#if block.items.length === 0}
						<!-- Designed empty state: a quiet bronze mark, not an apology. -->
						<div class="flex flex-1 flex-col items-center justify-center gap-2.5 py-8">
							<span
								class="inline-block h-2 w-2 rotate-45 bg-[#b08d57]/25 dark:bg-[#b08d57]/35"
								aria-hidden="true"
							></span>
							<p class="text-center text-sm text-[#6f6c66]/70 dark:text-[#a5a29b]/60">
								{block.empty}
							</p>
						</div>
					{:else}
						<ul class="mt-5 space-y-2.5 text-sm leading-relaxed text-[#6f6c66] dark:text-[#a5a29b]">
							{#each block.items as item (item)}
								<li class="flex gap-2.5">
									<span
										class="mt-[0.45rem] inline-block h-1 w-1 shrink-0 rounded-full bg-[#b08d57]/50"
										aria-hidden="true"
									></span>
									<span>{item}</span>
								</li>
							{/each}
						</ul>
					{/if}
				</div>
			{/each}
		</div>
	</div>
</section>

<div use:reveal class="reveal py-10 md:py-12">
	<Ornament />
</div>

<!-- ═════════════════════════════════════════════════
     APPS — the main page's feature-card idiom exactly:
     bronze icon coin, hover lift, revealed arrow.
     ═════════════════════════════════════════════════ -->
<section class="relative bg-[#f3f1ec] dark:bg-[#15181e]">
	<div class="mx-auto max-w-7xl px-6 py-16 md:py-20">
		<div use:reveal class="reveal mb-10 flex items-center gap-3">
			<span class="inline-block h-1.5 w-1.5 rotate-45 bg-[#b08d57]"></span>
			<h2 class="text-xs font-bold tracking-[0.22em] uppercase text-[#6f6c66] dark:text-[#a5a29b]">
				{m.semos_workspace_apps()}
			</h2>
		</div>

		<div use:reveal class="reveal grid gap-7 sm:grid-cols-2 lg:grid-cols-3">
			{#each cfg.workspace.apps as app, i (app.name)}
				{@const Icon = icons[app.icon] ?? LayoutGrid}
				<a
					href={app.href}
					style="transition-delay: {i * 60}ms"
					class="group relative flex flex-col rounded-2xl border-t border-white bg-gradient-to-b from-white to-[#faf8f4] p-7 shadow-[0_1px_2px_rgba(23,24,28,0.06),0_3px_6px_rgba(23,24,28,0.05),0_12px_28px_rgba(23,24,28,0.09)] transition-all duration-300 hover:-translate-y-1.5 hover:shadow-[0_2px_4px_rgba(23,24,28,0.07),0_6px_12px_rgba(23,24,28,0.07),0_24px_48px_rgba(23,24,28,0.14)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[#b08d57]/50 dark:border-white/10 dark:from-[#1c2029] dark:to-[#171a21] dark:shadow-[0_1px_2px_rgba(0,0,0,0.4),0_12px_28px_rgba(0,0,0,0.5)]"
				>
					<div
						class="inline-flex w-fit rounded-full bg-gradient-to-b from-[#f6f4ef] to-[#e9e5dc] p-3.5 text-[#b08d57] shadow-[inset_0_1px_1px_rgba(255,255,255,0.9),0_2px_6px_rgba(23,24,28,0.12)] transition-transform duration-300 group-hover:scale-105 dark:from-[#252a35] dark:to-[#1c2029] dark:shadow-[inset_0_1px_1px_rgba(255,255,255,0.08),0_2px_6px_rgba(0,0,0,0.5)]"
					>
						<Icon class="h-6 w-6" />
					</div>
					<div class="mt-6 flex items-start justify-between gap-2">
						<h3 class="font-bold tracking-tight text-[#17181c] dark:text-[#e9e7e2]">{app.name}</h3>
						<ArrowUpRight
							class="mt-0.5 h-4 w-4 shrink-0 text-[#b08d57]/0 transition-all duration-300 group-hover:translate-x-0.5 group-hover:-translate-y-0.5 group-hover:text-[#b08d57]"
						/>
					</div>
					<p class="mt-2.5 text-sm leading-relaxed text-[#6f6c66] dark:text-[#a5a29b]">
						{app.description}
					</p>
				</a>
			{/each}
		</div>
	</div>
</section>

<style>
	.reveal {
		opacity: 0;
		transform: translateY(0.75rem);
		transition:
			opacity 0.5s cubic-bezier(0.16, 1, 0.3, 1),
			transform 0.5s cubic-bezier(0.16, 1, 0.3, 1);
	}
	.reveal:global(.is-visible) {
		opacity: 1;
		transform: translateY(0);
	}
	@media (prefers-reduced-motion: reduce) {
		.reveal {
			opacity: 1;
			transform: none;
			transition: none;
		}
	}
</style>
```

- [ ] **Step 2: Verify no banned color survived**

```bash
cd /Users/cding/Workspace/ChenWeb && grep -nEi '080b14|6b7aff|131726|f4f2ed|1a1a1a|6b6b6b|e8e7e4|9a9aa0|0a0d18|1a1f30' web/src/routes/semos/workspace/+page.svelte && echo "!!! BANNED COLOR FOUND" || echo "clean — no /semos1 palette remains"
```

Expected: `clean — no /semos1 palette remains`.

- [ ] **Step 3: Verify no hardcoded user-visible English survived**

```bash
cd /Users/cding/Workspace/ChenWeb && grep -nE "'(No |Welcome|Announcements|Alarms|Recent)" web/src/routes/semos/workspace/+page.svelte && echo "!!! HARDCODED STRING FOUND" || echo "clean — all copy comes from config or the message catalog"
```

Expected: `clean — all copy comes from config or the message catalog`.

- [ ] **Step 4: Typecheck**

```bash
cd /Users/cding/Workspace/ChenWeb/web && bun run check
```

Expected: no new errors. An error on `m.semos_workspace_no_alarms` means Task 2 Step 9 did not regenerate the paraglide output — rerun it.

- [ ] **Step 5: Verify in the browser, both modes**

```bash
cd /Users/cding/Workspace/ChenWeb/web && bun run dev
```

Visit `http://localhost:5173/semos/workspace` and check, in **light mode**:
- The banner is light paper, not near-black. The bronze diamond + kicker sits above the title.
- Two ornaments: banner→feeds, feeds→apps.
- Three feed cards. Announcements shows one bulleted item from config; the other two show the bronze-diamond empty state.
- Six app cards with bronze icon coins. Hovering one lifts it and reveals the arrow.
- Nothing is indigo.

Then toggle **dark mode** (the header's moon/sun button) and confirm all of the above still holds on the dark paper palette.

Then navigate `/semos` → `/semos/workspace` and back. The header, footer, ornament, card depth, and accent color must not change between them. That is the whole point of the task.

- [ ] **Step 6: Verify the language switcher moved the copy out of the component**

Click the header's language button. The empty-state text under Recent Activities and Alarms must change language, and the announcement text must change too (it comes from the other locale's config file). If the empty-state text stays English, it is still hardcoded somewhere.

- [ ] **Step 7: Verify the tenant path did not regress**

Visit `http://localhost:5173/semos/workspace?tenant=demo`. It must load the demo tenant's config — its own banner title and its own announcement.

Then visit `http://localhost:5173/semos/workspace?tenant=nope`. The error must surface in the banner, in the muted red (`#b4462f`), not silently swallowed.

- [ ] **Step 8: Judge the banner image**

The spec flags this: `banner_image` is `/images/angleWalls.jpg`, chosen to sit at 40% opacity under a **near-black** hero. Under a pale veil it may read as busy or muddy.

Look at it now, in both modes. If it fights the paper treatment, replace the value in all three config files with an image whose register matches the Main hero. This is a config edit, not a code edit. If it looks fine, leave it and say so.

- [ ] **Step 9: Commit**

```bash
cd /Users/cding/Workspace/ChenWeb && jj commit web/src/routes/semos/workspace/ config/site/ -m "feat(semos): re-skin the workspace page onto the paper-and-ink system

The page still wore the /semos1 dark variant that was deleted when /semos2
won: near-black banner, indigo accent, angled clip-path, glassy cards. It
contradicted the header and footer wrapping it.

Now shares the main page's material — paper veil, bronze ornaments, icon
coins, layered card depth — at app-shell density per ADR 2026071102. Feeds
render designed empty states; no data is fabricated.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 5: Update the ADR

The workspace coding protocol requires a code change to answer *which docs were
updated*. Both edits go in the parent ADR.

**Files:**
- Modify: `KnowledgeStore/doc-repo/adrs/202607/2026071102-adr-new-gui-semos.md` — the Change Logs list and the "As-built config schema" table

**Interfaces:**
- Consumes: nothing. Produces: nothing.

- [ ] **Step 1: Add a changelog entry**

At the end of the `## Change Logs` list (after the `2026/07/14` entry about per-locale site config), add:

```markdown
* 2026/07/14, Re-designed the Workspace Landing page. It had been left on the
  deleted `/semos1` dark variant (near-black banner, indigo `#6b7aff`, angled
  clip-path, glassy cards) while the Main page moved to `/semos2` paper-and-ink,
  so it contradicted the shared header/footer. Now uses the same material —
  paper veil, bronze ornaments, icon coins, layered card depth — at app-shell
  density (compact banner, tighter rhythm) per the Design Direction density
  rule. Closed two rule violations found in the page: announcements were
  hardcoded in the Svelte component (now `[workspace].announcements` in TOML)
  and empty-state copy was hardcoded English (now in the message catalog).
  Established the principle that resolves where content belongs: **UI chrome →
  message catalog, tenant content → TOML** — which is the precedent already set
  by nav labels vs. hero copy, so it does not pre-empt the open nav-label
  question. Recent Activity and Alarms ship as designed empty states: no
  endpoint backs them and fabricating demo content is the same mistake as the
  invented `[[stats]]` figures. Extracted the bronze ornament to a shared
  `Ornament.svelte`. Noted that `ja`/`ko` message catalogs do not exist despite
  being in `supported_languages` — folded into the language-config open
  question. Full design:
  [2026-07-14-semos-workspace-redesign-design.md](../../../../ChenWeb/docs/superpowers/specs/2026-07-14-semos-workspace-redesign-design.md).
```

- [ ] **Step 2: Update the As-built config schema table**

In the `#### As-built config schema (2026/07/14)` section, replace the final row of the table:

```markdown
| `[workspace]`, `[[workspace.apps]]` | Workspace Landing content |
```

with:

```markdown
| `[workspace]` | `kicker`, `banner_title`, `banner_subtitle`, `banner_image`, `announcements` (array of strings) |
| `[[workspace.apps]]` | `name`, `description`, `href`, `icon` |
```

- [ ] **Step 3: Add the reference**

At the end of the `## References` list, add:

```markdown
- [2026-07-14-semos-workspace-redesign-design.md](../../../../ChenWeb/docs/superpowers/specs/2026-07-14-semos-workspace-redesign-design.md) — Workspace Landing page re-design: paper-and-ink port at app-shell density, `[workspace].announcements` config, and the chrome-vs-content rule for where a string belongs.
```

- [ ] **Step 4: Add the two new open questions**

In `## Open Questions`, under the `Open as of 2026/07/14:` list, add:

```markdown
- **`ja` and `ko` message catalogs do not exist.** `project.inlang/settings.json`
  declares `locales: ["en", "zh-cn"]`, but `[frontend].supported_languages` lists
  four. Folds into the existing language-config reconciliation question below.
- **The `web` project has no working test runner.** `.test.ts` files exist but
  `vitest` is not installed and no test script is defined, so they cannot run.
  Frontend changes currently carry no automated regression net beyond
  `svelte-check`. Pre-existing; surfaced by the Workspace re-design.
```

- [ ] **Step 5: Commit the ADR**

`KnowledgeStore` is its own git repo, separate from `ChenWeb`.

```bash
cd /Users/cding/Workspace/KnowledgeStore && jj commit doc-repo/adrs/202607/2026071102-adr-new-gui-semos.md -m "docs(adr): record the SemOS Workspace page re-design

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

If `KnowledgeStore` turns out to be a plain git repo rather than `jj`, use:

```bash
cd /Users/cding/Workspace/KnowledgeStore && git add doc-repo/adrs/202607/2026071102-adr-new-gui-semos.md && git commit -m "docs(adr): record the SemOS Workspace page re-design

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Definition of done

- [ ] `go test ./server/api/sitehandler/` passes.
- [ ] `bun run check` reports no new errors.
- [ ] `grep -Ei '080b14|6b7aff|131726|f4f2ed' web/src/routes/semos/workspace/+page.svelte` finds nothing.
- [ ] `/semos` → `/semos/workspace` shows no change in header, footer, ornament, card depth, or accent color, in **both** light and dark mode.
- [ ] The language switcher changes the feed empty-state copy.
- [ ] `?tenant=demo` loads the demo config; `?tenant=nope` shows the error.
- [ ] No fabricated announcements, activity items, or alarms anywhere.
- [ ] ADR 2026071102 has the changelog entry, the updated schema table, the reference, and the two new open questions.
