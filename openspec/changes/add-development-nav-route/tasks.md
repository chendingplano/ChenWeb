## 1. Extract the dashboard into a shared component

- [x] 1.1 Create `ChenWeb/web/src/lib/components/home3/dashboard.svelte` containing everything currently in `routes/home3/+page.svelte`'s `<script>` and markup, changing `let { data } = $props()` to a `siteConfig` prop (e.g. `let { siteConfig }: { siteConfig: SiteConfig } = $props();`) and `cfg = $derived(data.siteConfig)` to just using `siteConfig` directly (or `const cfg = siteConfig;` if the rest of the file references `cfg`).
- [x] 1.2 Replace `routes/home3/+page.svelte`'s contents with a thin wrapper that imports `Dashboard` from `$lib/components/home3/dashboard.svelte` and renders `<Dashboard siteConfig={data.siteConfig} />`.

## 2. Add the /development route

- [x] 2.1 Create `ChenWeb/web/src/routes/development/+layout.ts` as a copy of `routes/home3/+layout.ts` (fetches `siteConfig` via `fetchSiteConfig`, `ssr = false`).
- [x] 2.2 Create `ChenWeb/web/src/routes/development/+page.svelte` as a thin wrapper identical in shape to the new `routes/home3/+page.svelte`: imports `Dashboard` and renders `<Dashboard siteConfig={data.siteConfig} />`.

## 3. Add the nav entry

- [x] 3.1 Add `"semos_nav_development": "Development"` to `ChenWeb/web/messages/en.json` and `"semos_nav_development": "开发"` to `ChenWeb/web/messages/zh-cn.json`, matching the existing `semos_nav_*` keys' placement.
- [x] 3.2 In `routes/semos/components/SiteHeader.svelte`'s `nav` array, insert `{ label: m.semos_nav_development(), href: '/development', requiresAuth: true }` between the "Knowledge Base" and "About Us" entries.

## 4. Verify

- [x] 4.1 Run the dev server and load `/development`: confirm the URL stays `/development` (no redirect) and the dashboard renders identically to `/home3` (SiteHeader, nav rail, content panel, context shelf, stat tiles, Activity Feed, etc.). Verified via Playwright screenshot against the running `vite dev` server — pixel-identical to `/home3`, URL stayed `/development`.
- [x] 4.2 Confirm `/home3` still renders exactly as before (no regression from the extraction). Verified via screenshot — unchanged.
- [x] 4.3 Confirm the "Development" nav item appears between "Knowledge Base" and "About Us" in both desktop and mobile nav, is highlighted as active on `/development`, and is auth-gated the same way as "Workspace"/"Knowledge Base" when logged out. Verified via screenshots: desktop and mobile nav both show 首页/工作台/知识库/开发/关于我们 in order; 开发 is highlighted active on `/development`; clicking 开发 while logged out redirected to `/semos` and showed the login prompt, matching Workspace/Knowledge Base gating.
