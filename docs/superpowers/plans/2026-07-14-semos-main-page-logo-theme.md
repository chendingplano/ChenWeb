# SemOS Main Page Consolidation, Logo, Theme Consistency Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Consolidate the three parallel SemOS Main Page variants into one canonical `/semos` route, add a configurable company-logo image, and fix the light/dark mode flash so both Main and Workspace pages render consistently.

**Architecture:** `/semos2` (the "paper and ink" variant) becomes the sole `/semos` route; `/semos` (original) and `/semos1` are deleted. A new `LogoMark.svelte` component reads `config.branding.logo_image` and is shared by the header and footer, so the logo is automatically present everywhere those two are (i.e. every SemOS page). A blocking inline script in `app.html` applies the `.dark` class before Svelte hydrates, replacing the current `onMount`-only fix that causes a flash.

**Tech Stack:** SvelteKit 2 / Svelte 5 (runes), Tailwind CSS 4, Go 1.25 (Echo), TOML config, jj for version control.

## Global Constraints

- Scope is limited to the SemOS Main and Workspace pages only — no changes to `home3` or other ChenWeb routes.
- Repo root for all Go/config commands: `/Users/cding/Workspace/ChenWeb`. Repo root for `bun` commands: `/Users/cding/Workspace/ChenWeb/web`.
- Use exactly one `jj commit -m "..."` of the full working copy to close out each task (this repo uses jj, not git, per project convention). Do not use `git commit`. Do not run `jj restore`, `jj abandon`, `jj split`, or a separate `jj new`/`jj describe` — a prior incident in this repo lost an in-progress plan file to stray jj commands; `jj commit -m` alone both records the message and advances the working copy. Any unfamiliar file already present in the working copy belongs to the controller — leave it alone.
- Baseline verified before this plan was written: `bun run check` → 0 errors, 23 warnings (all pre-existing, unrelated files); `go build ./server/api/sitehandler/...` → success. Each task's verification step should not introduce new errors beyond this baseline.
- No Site Management upload UI, no automated test infra — this route has none today; verification is manual/browser-driven per the design spec.
- Follow `ChenWeb/CLAUDE.md`: simplicity first, no speculative abstractions, surgical changes only.

---

### Task 1: Consolidate the Main Page variant — delete `/semos`, `/semos1`, promote `/semos2`

**Files:**
- Delete: `web/src/routes/semos/` (entire old directory: `+layout.svelte`, `+layout.ts`, `+page.svelte`, `components/SiteHeader.svelte`, `components/SiteFooter.svelte`, `workspace/+page.svelte`)
- Delete: `web/src/routes/semos1/` (entire directory, same shape)
- Move: `web/src/routes/semos2/` → `web/src/routes/semos/` (all 6 files, same shape)
- Modify: `web/src/routes/semos/components/SiteHeader.svelte` (post-move path) — replace `semos2` with `semos` at lines 12, 13, 15, 31, 81, 95
- Modify: `web/src/routes/semos/components/SiteFooter.svelte` (post-move path) — replace `semos2` with `semos` at lines 11, 12, 96, 99, 102

**Interfaces:**
- Consumes: nothing from other tasks.
- Produces: canonical route `web/src/routes/semos/` (Main page at `/semos`, Workspace at `/semos/workspace`) that Tasks 2–4 will modify in place. `SiteHeader.svelte` and `SiteFooter.svelte` at this path are what Task 3 wires `LogoMark.svelte` into.

- [ ] **Step 1: Confirm no external references to the old paths before deleting anything**

Run: `grep -rn "semos1\|/semos2" web/src --include="*.ts" --include="*.svelte" --include="*.toml" | grep -v "web/src/routes/semos1\|web/src/routes/semos2"`
Expected: no output (empty). This confirms only the doomed directories reference these strings — matches the check already done during design (spec section 1).

- [ ] **Step 2: Delete the old `/semos` and `/semos1` directories**

```bash
cd /Users/cding/Workspace/ChenWeb
rm -rf web/src/routes/semos
rm -rf web/src/routes/semos1
```

- [ ] **Step 3: Move `/semos2` into the now-empty `/semos` path**

```bash
cd /Users/cding/Workspace/ChenWeb
mv web/src/routes/semos2 web/src/routes/semos
```

- [ ] **Step 4: Rename internal `semos2` references to `semos`**

```bash
cd /Users/cding/Workspace/ChenWeb
sed -i '' 's/semos2/semos/g' web/src/routes/semos/components/SiteHeader.svelte
sed -i '' 's/semos2/semos/g' web/src/routes/semos/components/SiteFooter.svelte
```

This turns `href="/semos2"` → `href="/semos"`, `href="/semos2/workspace"` → `href="/semos/workspace"`, and `semos2-mobile-nav` → `semos-mobile-nav` (both the `id` and `aria-controls` attribute) in both files — the only two files that contained the string.

- [ ] **Step 5: Verify no `semos2` string remains and no dangling `/semos1` references**

Run: `grep -rn "semos2\|semos1" web/src/routes/semos/`
Expected: no output (empty).

Run: `ls web/src/routes | grep semos`
Expected:
```
semos
```
(only one directory now, no `semos1`/`semos2`)

- [ ] **Step 6: Type-check**

Run: `cd web && bun run check`
Expected: `svelte-check found 0 errors` (warning count may match or be below the 23-warning baseline; no new errors).

- [ ] **Step 7: Commit**

```bash
cd /Users/cding/Workspace/ChenWeb
jj commit -m "refactor(semos): consolidate on the paper-and-ink variant, delete /semos and /semos1"
```

---

### Task 2: Add `logo_image` to the site config schema (TOML, Go, TypeScript) + placeholder asset

**Files:**
- Modify: `config/site/site-default.toml:4-7` (the `[branding]` table)
- Modify: `server/api/sitehandler/sitehandler.go:21-25` (the `Branding` struct)
- Modify: `web/src/lib/services/siteConfigService.ts:4-8` (the `SiteBranding` interface)
- Create: `web/static/images/logo-semos.svg` (placeholder logo asset)

**Interfaces:**
- Consumes: nothing from other tasks.
- Produces: `SiteConfig.branding.logo_image` (Go: `Branding.LogoImage string`, JSON/TOML key `logo_image`; TypeScript: `SiteBranding.logo_image: string`) — this is what Task 3's `LogoMark.svelte` reads.

- [ ] **Step 1: Add `logo_image` to the TOML config**

Modify `config/site/site-default.toml`, current `[branding]` block:

```toml
[branding]
site_name = "SemOS"
logo_text = "SemOS"
powered_by = "Powered by SemOS"
```

New:

```toml
[branding]
site_name = "SemOS"
logo_text = "SemOS"
logo_image = "/images/logo-semos.svg"
powered_by = "Powered by SemOS"
```

- [ ] **Step 2: Add `LogoImage` to the Go `Branding` struct**

Modify `server/api/sitehandler/sitehandler.go`, current:

```go
type Branding struct {
	SiteName  string `toml:"site_name" json:"site_name"`
	LogoText  string `toml:"logo_text" json:"logo_text"`
	PoweredBy string `toml:"powered_by" json:"powered_by"`
}
```

New:

```go
type Branding struct {
	SiteName  string `toml:"site_name" json:"site_name"`
	LogoText  string `toml:"logo_text" json:"logo_text"`
	LogoImage string `toml:"logo_image" json:"logo_image"`
	PoweredBy string `toml:"powered_by" json:"powered_by"`
}
```

- [ ] **Step 3: Verify the Go package builds**

Run: `cd /Users/cding/Workspace/ChenWeb && go build ./server/api/sitehandler/...`
Expected: no output, exit code 0.

- [ ] **Step 4: Add `logo_image` to the TypeScript `SiteBranding` interface**

Modify `web/src/lib/services/siteConfigService.ts`, current:

```ts
export interface SiteBranding {
	site_name: string;
	logo_text: string;
	powered_by: string;
}
```

New:

```ts
export interface SiteBranding {
	site_name: string;
	logo_text: string;
	logo_image: string;
	powered_by: string;
}
```

- [ ] **Step 5: Create the placeholder logo SVG**

Create `web/static/images/logo-semos.svg`:

```svg
<svg xmlns="http://www.w3.org/2000/svg" width="120" height="28" viewBox="0 0 120 28" role="img" aria-label="SemOS">
	<rect x="2" y="10" width="8" height="8" fill="#b08d57" transform="rotate(45 6 14)"/>
	<text x="20" y="20" font-family="Georgia, 'Times New Roman', serif" font-size="17" font-weight="700" letter-spacing="0.3" fill="#b08d57">SemOS</text>
</svg>
```

Bronze (`#b08d57`) is the existing accent color already used on both light and dark backgrounds elsewhere in this design (e.g. the footer's ornament dividers), so this single asset needs no light/dark variants of its own.

- [ ] **Step 6: Vet the package**

Run: `cd /Users/cding/Workspace/ChenWeb && go vet ./server/api/sitehandler/...`
Expected: no output, exit code 0. (The TOML parsing round-trip with the new field is exercised for real in Task 5's manual check, once the server actually serves `GET /api/site-config`.)

- [ ] **Step 7: Commit**

```bash
cd /Users/cding/Workspace/ChenWeb
jj commit -m "feat(semos): add configurable logo_image to site config schema"
```

---

### Task 3: Add `LogoMark.svelte` and wire it into the header and footer

**Files:**
- Create: `web/src/routes/semos/components/LogoMark.svelte`
- Modify: `web/src/routes/semos/components/SiteHeader.svelte` (wordmark block, lines 31-36 pre-edit)
- Modify: `web/src/routes/semos/components/SiteFooter.svelte` (wordmark block, lines 33-38 pre-edit — the "Column 1: Company info" block; the separate copyright-bar `logo_text` usage at the former line 121 is left untouched, since that is plain copyright text, not the visual brand mark)

**Interfaces:**
- Consumes: `SiteBranding` type and `logo_image`/`logo_text` fields from Task 2 (`web/src/lib/services/siteConfigService.ts`).
- Produces: `LogoMark.svelte` component with props `{ branding: SiteBranding; textClass: string }`, used by both header and footer. No other task depends on this.

- [ ] **Step 1: Create `LogoMark.svelte`**

Create `web/src/routes/semos/components/LogoMark.svelte`:

```svelte
<script lang="ts">
	import type { SiteBranding } from '$lib/services/siteConfigService';

	let { branding, textClass }: { branding: SiteBranding; textClass: string } = $props();
</script>

{#if branding.logo_image}
	<img src={branding.logo_image} alt={branding.logo_text} class="h-7 w-auto" />
{:else}
	<span class="inline-block h-2.5 w-2.5 rotate-45 rounded-[2px] bg-[#b08d57]"></span>
	<span class={textClass}>{branding.logo_text}</span>
{/if}
```

- [ ] **Step 2: Wire it into `SiteHeader.svelte`**

Modify `web/src/routes/semos/components/SiteHeader.svelte`. Current wordmark block:

```svelte
		<!-- Wordmark with bronze tick, echoing a monogram without copying one -->
		<a href="/semos" class="group flex items-baseline gap-1.5">
			<span class="inline-block h-2.5 w-2.5 rotate-45 rounded-[2px] bg-[#b08d57] transition-transform duration-300 group-hover:rotate-[135deg]"></span>
			<span class="text-[1.1rem] font-bold tracking-[0.02em] text-[#17181c] dark:text-[#e9e7e2]">
				{config.branding.logo_text}
			</span>
		</a>
```

New:

```svelte
		<!-- Wordmark: image logo if configured, else bronze-tick monogram + text -->
		<a href="/semos" class="group flex items-baseline gap-1.5">
			<LogoMark
				branding={config.branding}
				textClass="text-[1.1rem] font-bold tracking-[0.02em] text-[#17181c] dark:text-[#e9e7e2]"
			/>
		</a>
```

Add the import at the top of the `<script>` block, alongside the existing imports:

```ts
	import LogoMark from './LogoMark.svelte';
```

(Note: the fallback branch's hover-driven `group-hover:rotate-[135deg]` diamond spin is dropped from the fallback markup below since `LogoMark`'s fallback diamond is static — this is an acceptable minor visual simplification since the diamond is disappearing entirely once a real logo image is configured anyway.)

- [ ] **Step 3: Wire it into `SiteFooter.svelte`**

Modify `web/src/routes/semos/components/SiteFooter.svelte`. Current wordmark block (Column 1):

```svelte
				<div class="flex items-baseline gap-1.5">
					<span class="inline-block h-2 w-2 rotate-45 rounded-[2px] bg-[#b08d57]"></span>
					<span class="text-base font-bold tracking-tight text-[#17181c] dark:text-[#e9e7e2]">
						{config.branding.logo_text}
					</span>
				</div>
```

New:

```svelte
				<div class="flex items-baseline gap-1.5">
					<LogoMark
						branding={config.branding}
						textClass="text-base font-bold tracking-tight text-[#17181c] dark:text-[#e9e7e2]"
					/>
				</div>
```

Add the import at the top of the `<script>` block:

```ts
	import LogoMark from './LogoMark.svelte';
```

- [ ] **Step 4: Type-check**

Run: `cd web && bun run check`
Expected: `svelte-check found 0 errors`.

- [ ] **Step 5: Verify the copyright-bar `logo_text` usage is untouched**

Run: `grep -n "logo_text" web/src/routes/semos/components/SiteFooter.svelte`
Expected: exactly one remaining match, the copyright line: `<p>&copy; 2026 {config.branding.logo_text}. All rights reserved.</p>`

- [ ] **Step 6: Commit**

```bash
cd /Users/cding/Workspace/ChenWeb
jj commit -m "feat(semos): render configurable logo image in header and footer via LogoMark"
```

---

### Task 4: Fix the light/dark mode flash (FOUC)

**Files:**
- Modify: `web/src/app.html`
- Modify: `web/src/lib/stores/semosTheme.svelte.ts`

**Interfaces:**
- Consumes: the existing `semos-theme` localStorage key (unchanged) and the existing `.dark` class contract from `web/src/app.css` (unchanged).
- Produces: nothing consumed by other tasks — this is the last code change before manual verification in Task 5.

- [ ] **Step 1: Add the blocking inline theme-init script to `app.html`**

Modify `web/src/app.html`. Current:

```html
<!doctype html>
<html lang="%paraglide.lang%">
	<head>
		<meta charset="utf-8" />
		<meta name="viewport" content="width=device-width, initial-scale=1" />
		%sveltekit.head%
	</head>
	<body data-sveltekit-preload-data="hover">
		<div style="display: contents">%sveltekit.body%</div>
	</body>
</html>
```

New:

```html
<!doctype html>
<html lang="%paraglide.lang%">
	<head>
		<meta charset="utf-8" />
		<meta name="viewport" content="width=device-width, initial-scale=1" />
		<script>
			(function () {
				if (!location.pathname.startsWith('/semos')) return;
				var stored = localStorage.getItem('semos-theme');
				var dark = stored
					? stored === 'dark'
					: window.matchMedia('(prefers-color-scheme: dark)').matches;
				document.documentElement.classList.toggle('dark', dark);
			})();
		</script>
		%sveltekit.head%
	</head>
	<body data-sveltekit-preload-data="hover">
		<div style="display: contents">%sveltekit.body%</div>
	</body>
</html>
```

This runs synchronously before `%sveltekit.head%`/`%sveltekit.body%` are parsed, so the `.dark` class is set before the SemOS pages paint for the first time. It's gated to `/semos*` paths — confirmed via `grep -rl "dark:" web/src/routes --include="*.svelte" | grep -v "/semos"` (empty result) that no other route uses the `dark:` Tailwind variant, so this cannot affect any other page.

- [ ] **Step 2: Simplify `semosTheme.init()` to sync from the already-applied class instead of re-applying it**

Modify `web/src/lib/stores/semosTheme.svelte.ts`. Current:

```ts
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
```

New:

```ts
	/**
	 * Call once from the /semos layout (browser only). The `.dark` class
	 * itself is already applied by the blocking inline script in
	 * app.html (before hydration, to avoid a flash) — this only syncs the
	 * reactive `mode` state so the header's sun/moon icon matches on
	 * first paint.
	 */
	init() {
		const stored = localStorage.getItem(STORAGE_KEY);
		if (stored === 'dark' || stored === 'light') {
			this.mode = stored;
		} else {
			this.mode = window.matchMedia('(prefers-color-scheme: dark)').matches
				? 'dark'
				: 'light';
		}
	}
```

(Only the trailing `this.apply();` call is removed; `toggle()` and the private `apply()` method are unchanged, since `toggle()` still needs to apply the class when the user clicks the button.)

- [ ] **Step 3: Type-check**

Run: `cd web && bun run check`
Expected: `svelte-check found 0 errors`.

- [ ] **Step 4: Commit**

```bash
cd /Users/cding/Workspace/ChenWeb
jj commit -m "fix(semos): apply dark mode before hydration to eliminate theme flash"
```

---

### Task 5: Manual verification

**Files:** none (no code changes — this task only runs and observes the app).

**Interfaces:**
- Consumes: the fully assembled `/semos` and `/semos/workspace` pages from Tasks 1-4.
- Produces: a pass/fail confirmation for this plan. Nothing downstream depends on it.

- [ ] **Step 1: Start the app**

```bash
cd /Users/cding/Workspace/ChenWeb
mise run dev
```

Expected: both the API and web dev servers start without errors (watch for `(CWB_SITE_...)` log lines from `sitehandler` indicating `GET /api/site-config` was served successfully once a page loads).

- [ ] **Step 2: Verify the logo renders**

Open `/semos` in a browser. Confirm the header shows the `logo-semos.svg` image (bronze diamond + "SemOS" wordmark) where the plain-text wordmark used to be. Open `/semos/workspace` and confirm the same in its header. Scroll to the footer on both pages and confirm the Column 1 company-info block also shows the image, while the copyright bar at the very bottom still reads "© 2026 SemOS. All rights reserved." (plain text, unchanged).

- [ ] **Step 3: Verify no flash and persistence on `/semos`**

Click the sun/moon toggle in the header to switch to dark mode. Hard-refresh the page (Cmd+Shift+R). Confirm the page paints directly in dark mode with no visible flash of light-mode styling, and the header icon shows "Sun" (meaning dark is active) immediately on load.

- [ ] **Step 4: Verify consistency on `/semos/workspace`**

With dark mode still active from Step 3, type `/semos/workspace` directly into the browser's address bar (a full navigation, not a client-side link click) and load it. Confirm it also paints directly in dark mode with no flash — this is the specific "mode doesn't apply consistently across pages" symptom from the original bug report, now fixed.

- [ ] **Step 5: Verify the light-mode direction too**

Toggle back to light mode on either page. Hard-refresh both `/semos` and `/semos/workspace` in turn. Confirm both load directly in light mode with no flash of dark styling.

- [ ] **Step 6: Report result**

If all checks in Steps 2-5 pass, this plan is complete — no commit needed for this task since no files changed. If any check fails, stop and report which step failed and what was observed instead of the expected behavior, rather than attempting an ad hoc fix outside this plan.

---
