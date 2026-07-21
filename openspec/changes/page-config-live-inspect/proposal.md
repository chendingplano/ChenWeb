## Why

Editing DB-backed page content (`kb.page_def` / `kb.page_config`) is done by `entry_key`, which is a stable id divorced from what the user actually sees on the page. Admins configuring a page have no way to tell which row in the config table maps to which element on the live page, so they must guess, edit, reload the real page, and check — a slow, error-prone loop. Providing a live two-way visual mapping between config rows and rendered elements makes configuration self-explanatory.

## What Changes

- The Page Content admin view becomes a **split layout**: the existing entry table on the left, a **same-origin `<iframe>` preview** of the selected page on the right. Selecting a page in the existing `Page` dropdown sets the iframe `src` from the page's `route`, so the preview always shows the page being configured. A collapse toggle lets the wide table reclaim full width.
- **Hovering a config table row highlights the matching element** in the preview (outline + scroll-into-view).
- **Hovering a configurable element in the preview highlights the matching config row** — the primary direction, since it answers "what configures this thing I'm looking at?".
- A reusable, **page-agnostic contract**: any element whose text/content comes from `getPageConfig` carries `data-entry-key="<entry_key>"`. The admin inspector queries this generically, so **no admin code changes are ever needed to onboard a new page**.
- The **knowledge page** (`home3-knowledge`) is the first adopter (sidebar buttons stamped with `data-entry-key`). The **workspace page** (`semos-workspace`) and every future page with configurable/i18n content adopt the feature purely by stamping the attribute.

## Capabilities

### New Capabilities
- `page-config-inspect`: Live, bidirectional visual mapping between page-config admin rows (`entry_key`) and the rendered elements they configure, delivered via an embedded same-origin preview and a reusable `data-entry-key` page contract.

### Modified Capabilities
<!-- No spec-level requirement changes to existing capabilities; the admin table
     and frontend integration behaviors are extended, not altered. Existing
     capability specs live only under change dirs (no archived openspec/specs/),
     so this is tracked as one new capability. -->

## Impact

- **Frontend (admin):** `web/src/lib/components/home3/page-config-admin-view.svelte` — split layout, iframe preview bound to `selectedPage.route`, and the page-agnostic inspector logic (event delegation over `iframe.contentDocument`, injected highlight CSS, reactive row highlighting). Mounted at both `/development` System Admin (`content-panel.svelte`) and `semos/admin/page-config`, so both surfaces get the feature.
- **Frontend (page contract):** `web/src/routes/home3/knowledge/+page.svelte` — add `data-entry-key={item.id}` to the three sidebar button variants. No logic added to the page.
- **Docs:** document the `data-entry-key` convention as the standard way to make a configurable page inspectable; name `semos-workspace` as the designated next adopter (post-test).
- **No backend, API, DB, or dependency changes.** Relies on same-origin framing (no `X-Frame-Options`/`frame-ancestors` policy is set, so framing is already allowed).
