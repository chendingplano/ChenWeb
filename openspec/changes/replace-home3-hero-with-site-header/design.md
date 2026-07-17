## Context

`routes/home3/+page.svelte` renders `<HeroHeader {darkMode} {cfg} height={HERO_HEADER_HEIGHT} onToggleDark={toggleDark} />` directly (line 244) — there is no `home3/+layout.svelte`. `SiteHeader` (`routes/semos/components/SiteHeader.svelte`) manages its own dark-mode state via the `theme` store and only needs a `config: SiteConfig` prop; `home3/+page.svelte` already derives `cfg = $derived(data.siteConfig)` (line 24) from the existing `home3/+layout.ts`, so no new data loading is required.

`/home3` has sibling subroutes — `chunks`, `doc-structure`, `inputs`, `metrics`, `doc-review-report/[id]` — none of which currently have their own layout, plus `knowledge`, which already has its own `+layout.svelte` rendering `SiteHeader`.

## Goals / Non-Goals

**Goals:**
- `/home3` (the dashboard page only) renders `SiteHeader` where `HeroHeader` used to be.

**Non-Goals:**
- Do not touch `/home3/knowledge`, `/home3/chunks`, `/home3/doc-structure`, `/home3/inputs`, `/home3/metrics`, or `/home3/doc-review-report/[id]`.
- Do not delete or modify `hero-header.svelte` itself — `/home2` and `/home4` still use it.
- Do not preserve the hero's status strip ("3 agents active" / "12 tasks running" / "All systems nominal") — `SiteHeader` has no equivalent and none was requested.

## Decisions

- **Edit `routes/home3/+page.svelte` directly; do not add a `home3/+layout.svelte`.** A layout at the `/home3` level would cascade to every sibling subroute listed above and would double-render `SiteHeader` on `/home3/knowledge` (which already gets its own via `home3/knowledge/+layout.svelte` — Svelte layouts nest). Editing the page directly confines the change to exactly the one route the user pointed at.
- **Reuse `SiteHeader` as-is, no new props/variants.** It already takes just `config` and self-manages dark mode; `home3/+page.svelte` already has `cfg` available. Swapping the one line is sufficient — no changes to `SiteHeader.svelte` itself.
- **Remove `HERO_HEADER_HEIGHT` and `toggleDark`'s wiring into the header only if they become unused.** `toggleDark`/`darkMode` are used elsewhere on the page (borderColor/accent derivations, other components at lines 250/287/327) and must stay; only the `HeroHeader` import and its JSX usage (and `HERO_HEADER_HEIGHT`, if nothing else references it) are removed.

## Risks / Trade-offs

- [Losing the hero's status strip / "Main"/"Workspace" quick nav] → Acceptable: this is exactly the visual the user asked to replace, and `SiteHeader`'s own nav (Home/Workspace/Knowledge Base/About Us) already covers cross-navigation.
- [Visual height/spacing shift below the header, since `SiteHeader` and `HeroHeader` have different heights] → Low risk; verify visually after the change and adjust surrounding spacing only if it looks broken, per Surgical Changes.
