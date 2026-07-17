## Context

`routes/home3/+page.svelte` currently holds all of the dashboard's markup and state directly (nav rail, content panel, context shelf, drag-resize logic, `SiteHeader` at the top per `replace-home3-hero-with-site-header`). `routes/home3/+layout.ts` fetches `siteConfig` and exposes it as `data.siteConfig`, consumed via `let { data } = $props(); const cfg = $derived(data.siteConfig);`.

`SiteHeader.svelte`'s `nav` array (lines 22-27) is the single source of truth for both desktop and mobile nav rendering and for `isActive`/`handleNavClick` gating — adding an entry there is sufficient to get a working, gated, highlighted nav link with no other header changes.

## Goals / Non-Goals

**Goals:**
- `/development` renders identically to `/home3` (same dashboard, same behavior), reachable via a real SvelteKit route so the URL bar shows `/development`.
- No duplicated dashboard markup/logic between the two routes.
- New nav entry behaves exactly like `Workspace`/`Knowledge Base` (auth-gated, active-highlighted).

**Non-Goals:**
- Do not change `/home3`'s behavior or URL — it keeps working standalone.
- Do not share `/home3`'s subroutes (`knowledge`, `chunks`, etc.) under `/development` — only the top-level dashboard page.
- Do not build a generic "route aliasing" mechanism — this is a one-off duplicate route, not a pattern to generalize.

## Decisions

- **Extract `home3/+page.svelte`'s body into a new `$lib/components/home3/dashboard.svelte` taking `siteConfig` as a prop**, rather than (a) duplicating the markup into `routes/development/+page.svelte`, or (b) making `/development` a `redirect()` to `/home3`. Duplication would drift over time; a redirect changes the address bar back to `/home3`, which the user explicitly ruled out. Extraction keeps one source of truth for the dashboard and lets both routes be thin wrappers:
  ```svelte
  <!-- routes/home3/+page.svelte and routes/development/+page.svelte -->
  <script lang="ts">
    import Dashboard from '$lib/components/home3/dashboard.svelte';
    let { data } = $props();
  </script>
  <Dashboard siteConfig={data.siteConfig} />
  ```
- **`routes/development/+layout.ts` is a copy of `routes/home3/+layout.ts`** (same `fetchSiteConfig` load), since SvelteKit layouts aren't shared across unrelated route trees without a shared route group — copying a 9-line file is simpler than restructuring routes into a group for one reuse.
- **Add the nav entry as plain array data, not a new component or prop.** `SiteHeader`'s existing `nav` array already drives active-state, auth-gating, and both desktop/mobile rendering generically — no header logic changes needed.
- **New i18n key `semos_nav_development`** follows the existing `semos_nav_*` naming/lookup pattern (`m.semos_nav_development()`), consistent with `semos_nav_home`/`semos_nav_workspace`/etc.

## Risks / Trade-offs

- [Two routes now depend on one shared component; a future change to one route's dashboard needs an explicit decision about whether it should apply to both] → Acceptable for now — the proposal explicitly wants them identical; if they diverge later, that's a separate change to un-share them.
- [`routes/development/+layout.ts` duplicates `routes/home3/+layout.ts` verbatim] → Accepted per the Decisions section above; both are trivial 9-line files, not worth a shared-route-group refactor for this one duplication.
