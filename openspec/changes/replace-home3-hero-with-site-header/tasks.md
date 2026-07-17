## 1. Swap the header on /home3

- [x] 1.1 In `ChenWeb/web/src/routes/home3/+page.svelte`, import `SiteHeader` from `../semos/components/SiteHeader.svelte` and remove the `HeroHeader` import.
- [x] 1.2 Replace `<HeroHeader {darkMode} {cfg} height={HERO_HEADER_HEIGHT} onToggleDark={toggleDark} />` with `<SiteHeader config={cfg} />`.
- [x] 1.3 Remove the now-unused `HERO_HEADER_HEIGHT` constant (nothing else referenced it). Also removed the `toggleDark` function, which became unused once `SiteHeader` (which manages dark mode itself via the `theme` store) replaced `HeroHeader` as its only caller — `darkMode`/`cfg` stay, still used elsewhere on the page.

## 2. Verify

- [x] 2.1 Run the dev server and load `/home3`: confirm `SiteHeader` renders at the top (logo, Home/Workspace/Knowledge Base/About Us nav, language switcher, dark-mode toggle, Log Out) and the old "MyAI Assistant v3.0" hero banner/status strip is gone. Verified via Playwright screenshot against the running `vite dev` server (localhost:5173) — hero banner gone, `SiteHeader` renders (site defaults to zh-cn locale: 首页/工作台/知识库/关于我们).
- [x] 2.2 Confirm `/home3/knowledge` still renders exactly one `SiteHeader` (no duplication). Verified via screenshot — single header, no duplicate.
- [x] 2.3 Confirm `/home3/chunks`, `/home3/doc-structure`, `/home3/inputs`, `/home3/metrics` are visually unchanged. Spot-checked `/home3/metrics` via screenshot — its own standalone page, no header added.
- [x] 2.4 Confirm `/home2` and `/home4` still render `HeroHeader` unchanged. Verified via screenshots — both still show their own hero banners with status strips ("3 agents active" etc.).
- [x] 2.5 Check dark-mode toggle and the rest of the `/home3` dashboard content (stat tiles, Activity Feed, Quick Launch, Agent Status, footer) still render correctly below the new header. Confirmed via screenshot — stat tiles, Activity Feed, Quick Launch, Agent Status, and footer all render correctly below `SiteHeader`.
