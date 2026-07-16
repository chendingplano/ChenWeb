## Context

`ChenWeb/web/src/routes/home3/knowledge/+page.svelte` renders its own left menu (`<aside class="kb-menu">`, lines ~458-714) inline — there is no shared Sidebar/Layout component with `/semos`. The menu already has a collapse mechanic: local `$state` `menuCollapsed` + `menuWidth` (180-480px, default 280px), toggled via a `PanelLeftIcon`/`PanelLeftCloseIcon` button in a 48px header row. Nav items in that menu already have an established expanded-vs-collapsed rendering split (full card with icon+label+description when expanded, icon-only button with `title`/`aria-label` tooltip when collapsed) — this change follows that exact split for the new row.

`/semos` solves branding + language switching in `SiteHeader.svelte` + `LogoMark.svelte`, fed by `SiteConfig` (via `fetchSiteConfig`) loaded in `routes/semos/+layout.ts`. The Knowledge route currently has no `+page.ts`/load function and no access to `SiteConfig`.

## Goals / Non-Goals

**Goals:**
- Reuse existing branding/language logic (`LogoMark.svelte`, `locales`/`getLocale`/`setLocale`) rather than reimplementing it.
- Match the Knowledge menu's own established expanded/collapsed visual language (icon-only + tooltip when collapsed), not the semos header's pill-button style, since the sidebar is much narrower (56-480px vs semos's max-w-7xl bar).
- Zero impact on existing menu behavior: sections, resize, existing collapse toggle all unchanged.

**Non-Goals:**
- No shared cross-project Sidebar/Layout component extraction — out of scope, would be a larger refactor CLAUDE.md's "surgical changes" rule advises against here.
- No changes to `/semos` pages or components beyond importing `LogoMark.svelte` from them.
- No dark-mode-specific redesign beyond reusing the Knowledge page's existing `darkMode`/theme color variables (`panelBg`, `borderColor`, `accent`, `textMuted`, etc. already used throughout the aside).

## Decisions

**1. New row placement:** A new 48px row is inserted at the very top of `<aside class="kb-menu">`, *above* the existing "Knowledge" label + collapse-toggle row. Confirmed with user over the alternative of merging into or replacing the existing row — keeps the existing row's meaning (page title + explicit collapse action) intact.

**2. Reuse `LogoMark.svelte` directly via relative import** (`../../semos/components/LogoMark.svelte`) rather than duplicating it or moving it to `$lib/components`. It's a small, already-generic component (`branding`, `textClass`, `dotClass` props, no semos-specific coupling). Moving it to `$lib/` would be an unrelated refactor; duplicating it would drift out of sync. A cross-route-folder import is unusual but SvelteKit allows importing any `.svelte` file regardless of its route folder — it's just a component, not a route boundary.

**3. New `+page.ts` for `/home3/knowledge`**, mirroring `routes/semos/+layout.ts` (`ssr = false`, `load` returns `{ siteConfig: await fetchSiteConfig(fetch) }`). This is additive — the page currently has no load function, so there's no risk of colliding with existing data.

**4. Language control reuses `SiteHeader.svelte`'s exact logic** (`locales`, `getLocale()`, `setLocale()`, `nextLocale()` cycling) — copied inline into the Knowledge page's `<script>` (same pattern already used for `getLocale()` which is already imported here for `menuConfigLang`), not extracted into a shared helper, since it's ~4 lines and CLAUDE.md's "no abstractions for single-use code" applies.

**5. Collapsed-state rendering:** three new icon-only buttons (Logo mark → link to `/semos`, Language toggle → `Languages` icon, Workspace → link to `/semos/workspace`) stacked vertically in the 56px rail, above the existing expand-toggle button. Each uses the same icon-button styling already defined inline for collapsed nav leaf items (`h-5 w-5` icon, `title`/`aria-label` tooltip, hover color transition to `accent`). Per user's explicit direction: "add icons to the collapsed column ... on top of the existing icons."

**6. Logo when collapsed:** show only the dot/image mark (no wordmark text) at icon size, consistent with how every other collapsed row in this menu is icon-only.

## Risks / Trade-offs

- **Extra network fetch on page load** (`fetchSiteConfig`) → Mitigation: `SiteConfig` is a lightweight JSON fetch already used elsewhere in the app (`/semos`); acceptable one-time cost, not in a hot loop.
- **Cross-route-folder import (`semos/components/LogoMark.svelte` from `home3/knowledge`)** creates a soft coupling between the two route trees → Mitigation: `LogoMark` has no semos-specific logic/imports, so the coupling is purely file-location, not behavioral; acceptable per CLAUDE.md's "don't refactor things that aren't broken."
- **280px default width is tight** for Logo + two controls in one row → Mitigation: keep Language/Workspace as icon-only buttons with tooltips even in the expanded row (not full text pills like semos), so the row fits comfortably at the 180px minimum width too.

## Migration Plan

Purely additive UI change behind no flag; ships in the same deploy as the rest of `ChenWeb/web`. No data migration. Rollback = revert the commit.
