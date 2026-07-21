## Context

Page content is configured through `page-config-admin-view.svelte`, whose table lists entries keyed by `entry_key` — a stable id with no visual connection to what renders on the page. Admins can't see which row drives which element, so they edit blind. Both the admin surface and the pages it configures are served from a single origin (no `X-Frame-Options` / `frame-ancestors` policy is set anywhere in Caddy or the server), which makes direct, same-origin DOM access between an admin page and an embedded preview possible.

On the page side, every page that uses `getPageConfig(pageKey, lang)` already has, per configurable element, the exact `entry_key` it resolves against (the knowledge page uses the menu item's `id` as that key; the workspace page follows the same pattern). So the join key already exists on both sides — nothing new needs to be computed, only surfaced.

## Goals / Non-Goals

**Goals:**
- Bidirectional hover mapping between admin config rows and rendered elements, with the preview embedded in the admin view.
- A reusable, page-agnostic contract so onboarding a new page requires zero admin-code changes — only a `data-entry-key` attribute on the page.
- Selecting a page in the existing dropdown drives the preview.
- Instrument the knowledge page as the first adopter; leave workspace + future pages a one-attribute path.

**Non-Goals:**
- No cross-origin support (feature degrades to no-highlight if a page ever moves cross-origin; it must not error).
- No editing from the preview, no click-to-edit — hover-to-map only.
- No changes to backend, API, DB schema, or the resolution/config data model.
- Not pre-instrumenting workspace or other pages in this change (done after the knowledge page is verified).

## Decisions

**D1 — Embedded same-origin iframe, not two coordinated windows.**
The preview is an `<iframe>` inside the admin view, `src` bound to `selectedPage.route`. Rationale: makes "select page → show page" trivial (set `src`), keeps everything in one tab, and — being same-origin — lets the parent read and observe the preview DOM directly. Alternative (two windows + `BroadcastChannel`): rejected because controlling an independent window's navigation is fragile and every page would need to carry a listener.

**D2 — Attribute-only page contract; all logic in the admin parent.**
A configurable page's sole obligation is `data-entry-key="<entry_key>"` on each element whose content comes from `getPageConfig`. All detection, highlighting, scroll-into-view, and reverse row-highlight live in the admin, driven through `iframe.contentDocument`. Rationale: near-zero per-page code, no duplicated inspect logic, and the admin stays page-agnostic — any page lights up the instant it stamps the attribute. Alternative (per-page inspect agent via `postMessage`): rejected as more page code for no benefit while same-origin holds.

**D3 — Event delegation on the preview document, re-injected on `load`.**
On each iframe `load`, the parent grabs `contentDocument`, injects a `<style>` with the `.kb-inspect-hl` highlight rule into its `<head>`, and attaches delegated `mouseover`/`mouseout` listeners on the document. `event.target.closest('[data-entry-key]')` yields the hovered key. Delegation on the document survives the page's internal SvelteKit re-renders (expand/collapse, section switches) with no re-wiring; a hard navigation re-fires `load` and re-injects. Listeners/observers are torn down on component destroy and before each re-inject to avoid leaks/duplicates.

**D4 — Highlight state is reactive in the admin, imperative in the preview.**
Preview→admin (the primary direction): the delegated handler sets a reactive `hoveredKey`; the row whose `entry_key === hoveredKey` gets a highlight class via normal Svelte reactivity. Admin→preview: on row `mouseenter`, the parent imperatively `querySelector`s `[data-entry-key="…"]` in the preview, adds `.kb-inspect-hl`, and `scrollIntoView({block:'nearest'})`; `mouseleave` clears it. This keeps admin DOM idiomatic (Svelte-managed) while treating the foreign document imperatively.

**D5 — Collapsible preview pane.**
The entry table is wide (many locale columns). The preview pane has a collapse toggle so the table can reclaim full width when not inspecting. Default: preview shown.

## Risks / Trade-offs

- **Same-origin dependency** → If a configurable page is ever served cross-origin, `contentDocument` access throws. Mitigation: guard every access in a try/catch (or `contentDocument` null-check) and degrade to plain preview with no highlighting; never throw.
- **Iframe loads full page chrome** (nav rail, header) → visually noisy preview. Accepted for now; the highlight + scroll still make the mapping clear. A `?preview=1` chrome-hiding mode is out of scope (would add per-page code, violating D2).
- **Row hovered but no element in preview** (item hidden/suspended/unauthorized, or a stale `entry_key`) → `querySelector` returns null. Mitigation: no-op; the page's existing `unknownMenuConfigIds` banner already flags stale ids, so no new surface is needed.
- **Listener leaks / double-binding across reloads** → Mitigation: always detach previous listeners and disconnect the injected style before re-injecting on `load`, and on component `onDestroy`.
- **Stacking-context / z-index of `.kb-inspect-hl`** inside the preview → use `outline` (not border) + `outline-offset` so highlight doesn't reflow the previewed layout.

## Adoption guide (generalization)

Onboarding any additional page to live inspect requires **no change to the admin component** — the inspector is page-agnostic. To make a `getPageConfig`-driven page inspectable:

1. On every element whose text/content comes from `getPageConfig`, add `data-entry-key="<entry_key>"`, where `<entry_key>` is the same key used in `kb.page_config` (on the knowledge page this is the menu item's `id`).
2. Ensure that page is listed in `kb.page_def` (so it appears in the admin `Page` dropdown with its `route`). That is the whole contract.

**Designated next adopter: the workspace page** (`semos-workspace`, `web/src/routes/semos/workspace/+page.svelte`), to be instrumented once the knowledge page is verified in production. It already uses `getPageConfig('semos-workspace', …)` with the same overlay model, so adoption is: stamp `data-entry-key` on its configurable elements — nothing else. Every future page with configurable/i18n content follows the same one-attribute path.
