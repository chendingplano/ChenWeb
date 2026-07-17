## Why

`/home3` currently renders a bespoke "MyAI Assistant v3.0" hero banner (`hero-header.svelte`) with its own nav (Main/Workspace), language/notification/dark-mode icons, and a status strip. This looks and behaves differently from every other page in the product, including its own child route `/home3/knowledge`, which already renders the shared `SiteHeader` (logo, Home/Workspace/Knowledge Base/About Us nav, language switcher, dark-mode toggle, Log Out). The user wants `/home3` to use the same shared `SiteHeader` so the dashboard is visually consistent with the rest of the site.

## What Changes

- `/home3` renders the shared `SiteHeader` (same component used on `/home3/knowledge` and `/semos` pages) at the top of the page instead of the bespoke `hero-header.svelte` banner.
- `hero-header.svelte`'s usage in `routes/home3/+page.svelte` is removed. Its status-strip content ("3 agents active", "12 tasks running", "All systems nominal") is dropped from `/home3` — `SiteHeader` has no equivalent, and the user did not ask for it to be preserved elsewhere. The `hero-header.svelte` component itself is **not** deleted: `/home2` and `/home4` still use it and are out of scope for this change.
- The rest of `/home3`'s page content (stat tiles, Activity Feed, Quick Launch, Agent Status, footer) is unchanged.

## Capabilities

### New Capabilities
- `home3-dashboard-header`: `/home3` renders the shared `SiteHeader` above its dashboard content, matching `/home3/knowledge` and `/semos` pages.

### Modified Capabilities
(none — the previous hero banner was never captured as a spec'd capability)

## Impact

- `ChenWeb/web/src/routes/home3/+page.svelte` — remove `HeroHeader` import/usage; render `SiteHeader` instead (likely via a new `home3/+layout.svelte`, mirroring `home3/knowledge/+layout.svelte`).
- `ChenWeb/web/src/lib/components/home3/hero-header.svelte` — left in place (still used by `/home2` and `/home4`).
- No changes to `/home3/knowledge`, `/home2`, `/home4`, or their existing header usage.
- No changes to `shared/`, other projects, or the Go backend.
