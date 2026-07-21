## 1. Admin split layout + preview

- [x] 1.1 In `page-config-admin-view.svelte`, wrap the existing controls + table in a left pane and add a right pane containing an `<iframe>`; keep all current table behavior unchanged
- [x] 1.2 Bind the iframe `src` to `selectedPage.route`; ensure changing the `Page` dropdown navigates the preview (verify req: selecting a page shows it)
- [x] 1.3 Add a collapse toggle for the preview pane; when collapsed the table expands to full width; default is preview shown
- [x] 1.4 Style the split (responsive widths, borders) consistent with the existing dark/light theme variables in the component

## 2. Inspector wiring (admin parent, page-agnostic)

- [x] 2.1 On iframe `load`, acquire `contentDocument` inside a try/catch; if unavailable or cross-origin, disable highlighting and return without throwing
- [x] 2.2 Inject a `<style>` with a `.kb-inspect-hl` rule (outline + outline-offset + subtle tint, no layout reflow) into the preview `<head>`; track the injected node for cleanup
- [x] 2.3 Attach delegated `mouseover`/`mouseout` listeners on the preview document; resolve the hovered key via `event.target.closest('[data-entry-key]')` and set a reactive `hoveredKey`
- [x] 2.4 Highlight the config row whose `entry_key === hoveredKey` (reactive class on the `<tr>`)
- [x] 2.5 On config-row `mouseenter`, `querySelector('[data-entry-key="…"]')` in the preview, add `.kb-inspect-hl`, and `scrollIntoView({block:'nearest'})`; clear on `mouseleave`; no-op when the element is absent
- [x] 2.6 Tear down listeners and the injected style before each re-inject and on `onDestroy`; confirm no double-binding after switching pages or reloading the preview

## 3. Page contract — knowledge page (first adopter)

- [x] 3.1 Add `data-entry-key={item.id}` to all three sidebar button variants in `home3/knowledge/+page.svelte` (collapsed icon button, expanded parent button, child button); add no logic to the page

## 4. Verification

- [x] 4.1 Playwright (webapp-testing): load the admin, select the knowledge page, hover a table row → assert the matching `[data-entry-key]` in the frame gains `.kb-inspect-hl`
- [x] 4.2 Playwright: hover a `[data-entry-key]` element in the frame → assert the matching config row gets the highlight class; and after expanding a menu section, hovering re-rendered children still highlights their rows
- [x] 4.3 Manual pass in the real split view: dropdown drives preview, collapse toggle works, stale/hidden entry_key hover is a safe no-op
- [x] 4.4 `mise build` / typecheck the web app clean

## 5. Docs + generalization path

- [x] 5.1 Document the `data-entry-key` = `entry_key` convention as the standard way to make a `getPageConfig`-driven page inspectable (in the change docs and/or the admin component header comment)
- [x] 5.2 Record `semos-workspace` as the designated next adopter (attribute-only) once the knowledge page is verified; note that no admin change is required to onboard it

## 6. Follow-ups (post-review feedback)

- [x] 6.1 Add a draggable divider between the list and preview panes; dragging adjusts pane widths (pointer capture, 20–80% clamp), verified with Playwright
- [x] 6.2 Add a preview toolbar with the route + "Open ↗" (new tab) fallback, and an inline hint when the frame renders empty/refused (`previewReadable` gate)
- [ ] 6.3 Diagnose blank preview on the deployed site (renders fine in local dev): capture the iframe request status/response + console in an authenticated browser session to determine whether it's a 401-in-iframe or a frame-policy refusal, then fix at the source (server serves framed HTML / relax frame policy for same-origin)

## 7. Table UX improvements (post-review feedback)

- [x] 7.1 Sticky list header — only the list body scrolls (left-pane/table-wrap restructure + `position:sticky` thead)
- [x] 7.2 Refresh button in the preview toolbar (reloads the iframe)
- [x] 7.3 STATUS column as a dropdown (active/disabled/suspended → enabled/accessible) with auto-save
- [x] 7.4 ACCESS_ROLE column as an inline chip editor (× to remove, menu to add, canonical roles from `/api/v1/system-admin/roles`), double-click to edit, auto-save
- [x] 7.5 EN/ZH-CN label cells inline-editable on double-click (Enter saves, Esc cancels, empty reverts to default), auto-save
- [x] 7.6 Verified all interactions with Playwright (7/7, PUT payloads asserted) + `svelte-check` clean
