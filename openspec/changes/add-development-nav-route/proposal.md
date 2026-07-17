## Why

The user wants a way to reach the `/home3` dashboard from the shared `SiteHeader` nav under a more descriptive URL, `/development`, sitting between "Knowledge Base" and "About Us". The address bar should read `/development`, not redirect back to `/home3` — so `/development` needs to be a real route rendering the same dashboard, not a client-side redirect.

## What Changes

- `SiteHeader.svelte`'s `nav` array gets a new entry — `Development` → `/development`, `requiresAuth: true` — inserted between the existing "Knowledge Base" and "About Us" entries. It behaves exactly like "Workspace"/"Knowledge Base": gated by `handleNavClick`, highlighted via `isActive` when on `/development`, shown in both desktop and mobile nav (all automatic, since both render from the same `nav` array).
- New i18n message key `semos_nav_development` added to `web/messages/en.json` ("Development") and `web/messages/zh-cn.json` ("开发").
- The `/home3` dashboard's markup (currently inline in `routes/home3/+page.svelte`) is extracted into a shared component so it can be rendered by two routes without duplication. `routes/home3/+page.svelte` and a new `routes/development/+page.svelte` both render this shared component.
- New `routes/development/+layout.ts` (copy of `routes/home3/+layout.ts`) fetches `siteConfig` for the new route.
- `/home3` keeps working exactly as it does today — this is additive, not a move/rename.

## Capabilities

### New Capabilities
- `development-nav-route`: a "Development" entry in the shared `SiteHeader` nav that links to `/development`, which renders the same dashboard as `/home3` under its own URL.

### Modified Capabilities
(none — no previously spec'd capability's requirements change; `/home3` itself keeps behaving as spec'd by `home3-dashboard-header`)

## Impact

- `ChenWeb/web/src/routes/semos/components/SiteHeader.svelte` — add nav entry.
- `ChenWeb/web/messages/en.json`, `ChenWeb/web/messages/zh-cn.json` — add `semos_nav_development` key.
- `ChenWeb/web/src/lib/components/home3/` — new shared dashboard component extracted from `routes/home3/+page.svelte`.
- `ChenWeb/web/src/routes/home3/+page.svelte` — slimmed down to render the extracted shared component.
- `ChenWeb/web/src/routes/development/+page.svelte`, `ChenWeb/web/src/routes/development/+layout.ts` — new files.
- No changes to `/home3`'s other subroutes (`knowledge`, `chunks`, `doc-structure`, `inputs`, `metrics`, `doc-review-report`), the Go backend, or `shared/`.
