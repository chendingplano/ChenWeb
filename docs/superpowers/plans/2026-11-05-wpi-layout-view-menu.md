# WPI Layouter View Menu Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a floating-toolbar menu that opens the selected page element's page or element layouter in the Properties panel's Class editor.

**Architecture:** `PagePreview` will render an accessible dropdown and report the chosen layouter view to `wpi-editor.svelte`. The editor will retain the element selection, set the Class tab active, and derive a `ClassTarget` from the active page or selected element's pane, then pass that target to the existing `WpiClassPanel`.

**Tech Stack:** Svelte 5, TypeScript, Bits UI DropdownMenu, Lucide Svelte icons, Paraglide messages.

---

## Chunk 1: Add layouter view selection to the floating toolbar and Properties panel

**Files:**
- Modify: `web/src/lib/components/shared-ui/canvas/PagePreview.svelte` — add the view dropdown, translated menu labels, and callback.
- Modify: `web/src/lib/components/shared-ui/wpi-editor.svelte` — track/reset view state, resolve class targets from the selected element and page, activate the Class tab, and pass the target to `WpiClassPanel`.
- Modify: `web/messages/en.json` — English menu labels, tooltip, and accessible name.
- Modify: `web/messages/zh-cn.json` — matching Chinese menu labels, tooltip, and accessible name.

- [ ] **Step 1: Add editor view state, target resolution, and toolbar availability**

  In `wpi-editor.svelte`, track `null | 'page-layouter' | 'element-layouter'` as the active layouter view. Derive the matching target from `loadedPage.page_layouter.layouter` or from the selected node's `pane` and `loadedPage.panels[pane].elem_layouter.layouter`. Treat only a non-empty string ID as usable; do not require a successful class fetch. When a view is chosen, keep the node selected and set `propertiesTab = 'class'`. Pass the derived target to `WpiClassPanel`, falling back to the existing `classTarget(...)` when no view is active or the current selection has no usable target. Pass booleans for each usable target to `PagePreview` so its menu can disable only unavailable items, and connect the toolbar callback to the editor's view-state setter.

- [ ] **Step 2: Define view-state lifecycle**

  Keep the chosen view while the page stays open and derive it for the current selected element. Clear it when selection is cleared or a different page/tab is loaded. If the newly selected element has no usable ID for the active view, temporarily fall back to that element's default class target but retain the chosen view so it resumes on a later selection with a usable target. If a non-empty ID cannot be loaded, keep the target active and let the existing class editor show its unavailable-class state.

- [ ] **Step 3: Add the floating toolbar dropdown and localized labels**

  In `PagePreview.svelte`, add an Eye icon trigger using the existing `$lib/components/ui/dropdown-menu` pattern. Give it a localized tooltip and accessible name. Add Page Layouter and Element Layouter menu items, disabling each item only when its target ID is missing or empty. Report the chosen view through a new callback prop that is wired to the editor state setter. Use Paraglide keys in both English and Chinese for the tooltip, accessible name, and both menu labels.

- [ ] **Step 4: Review the focused diff**

  Confirm the menu keeps the current selection, opens the Class tab, uses the page layouter or the selected element's pane layouter, and resets on selection clear/page or tab change. Confirm no unrelated hunks are staged or committed. Do not run tests unless requested.
