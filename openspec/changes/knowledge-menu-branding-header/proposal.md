## Why

The Knowledge (`/home3/knowledge`) left menu column is currently a standalone shell with no way to identify the site or return to the semos Workspace, and no in-page language switch — a user landing on this page has to use browser back/URL editing to get back to `/semos/workspace`. The `/semos` marketing site already solves branding + language switching in `SiteHeader.svelte` / `LogoMark.svelte`; the Knowledge menu should surface the same affordances instead of leaving the page an orphaned island.

## What Changes

- Add a new row at the very top of the Knowledge left menu (`kb-menu`), above the existing "Knowledge" header row, containing:
  - The site **Company Logo** (image or dot+text wordmark), reusing `routes/semos/components/LogoMark.svelte`, linking to `/semos`.
  - A **Language Control** button that cycles the active locale, reusing the same `locales` / `getLocale` / `setLocale` logic as `SiteHeader.svelte`.
  - A **Workspace** button/link that navigates to `/semos/workspace`.
- Add a `+page.ts` load function for `/home3/knowledge` (mirroring `routes/semos/+layout.ts`) to fetch `SiteConfig` so `LogoMark` has `branding` to render.
- When the menu is collapsed (56px icon rail), the new row is replaced by three icon-only buttons (Logo mark, Language toggle, Workspace) stacked above the existing collapse/expand toggle, matching the icon-only treatment already used by collapsed nav items.
- No changes to menu item behavior, sections, or existing collapse/resize mechanics.

## Capabilities

### New Capabilities
- `kb-menu-branding-header`: Branding/navigation row (logo, language switch, Workspace link) at the top of the Knowledge page's left menu, in both expanded and collapsed states.

### Modified Capabilities
(none — no existing spec covers the Knowledge menu's header)

## Impact

- `ChenWeb/web/src/routes/home3/knowledge/+page.svelte` — new header row markup + collapsed-state icon buttons, new imports (`LogoMark`, `locales`/`setLocale`, `Languages` icon, site config type).
- `ChenWeb/web/src/routes/home3/knowledge/+page.ts` — new file, loads `SiteConfig` via `fetchSiteConfig` (same pattern as `routes/semos/+layout.ts`).
- No changes to `shared/`, other projects, or backend/API surfaces — this is a client-side Svelte/SvelteKit change confined to `ChenWeb/web`.
