## 1. Data loading

- [x] 1.1 Add `ChenWeb/web/src/routes/home3/knowledge/+page.ts` with `ssr = false` and a `load` that returns `{ siteConfig: await fetchSiteConfig(fetch) }`, mirroring `routes/semos/+layout.ts`.
- [x] 1.2 In `+page.svelte`, accept `data` via `let { data }: { data: PageData } = $props();` and read `data.siteConfig.branding` for the logo.

## 2. Expanded-state branding row

- [x] 2.1 Import `LogoMark` from `../../semos/components/LogoMark.svelte`, `locales`/`setLocale` from `$lib/paraglide/runtime` (alongside the existing `getLocale` import), and a `Languages` icon plus a suitable "workspace" icon from `@lucide/svelte`.
- [x] 2.2 Add a `nextLocale()` helper in the `<script>` block (same logic as `SiteHeader.svelte`).
- [x] 2.3 Insert a new 48px row at the top of `<aside class="kb-menu">`, above the existing "Knowledge" header row, rendered only when `!menuCollapsed`: Logo (wrapped in `<a href="/semos">`) on the left; Language Control icon button (`onclick={() => setLocale(nextLocale())}`) and Workspace icon button (`<a href="/semos/workspace">`) grouped on the right. Use existing theme variables (`panelBg`, `borderColor`, `textMuted`, `accent`, `hoverBg`) for styling consistency.
- [x] 2.4 Give the Language Control and Workspace buttons `title`/`aria-label` tooltips (e.g. "Switch language", "Go to Workspace").

## 3. Collapsed-state icon row

- [x] 3.1 Rendered only when `menuCollapsed`: add three icon-only buttons stacked above the existing expand-toggle button, matching the existing collapsed nav-item icon button styling (`h-5 w-5` icon, hover-to-`accent` color transition, `title`/`aria-label`): Logo mark (link to `/semos`), Language toggle, Workspace (link to `/semos/workspace`).
- [x] 3.2 Verify logo collapsed variant shows only the mark (image or dot), no wordmark text.

## 4. Verification

- [x] 4.1 Run the app locally (`mise dev` or equivalent) and manually verify on `/home3/knowledge`: logo navigates to `/semos`, language control cycles locale and updates visible i18n text, Workspace button navigates to `/semos/workspace` — in both expanded and collapsed menu states.
- [x] 4.2 Verify menu resize (180-480px) and existing collapse/expand toggle still work unchanged.
- [x] 4.3 Verify dark mode styling of the new row/icons matches existing theme variables (no hardcoded colors that break dark mode).
- [x] 4.4 Run project lint/typecheck (e.g. `mise check` / `bun run check` per ChenWeb conventions) to catch type errors from the new `PageData`/import usage.
