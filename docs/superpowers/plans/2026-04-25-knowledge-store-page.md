# Knowledge Store Page Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Knowledge Stores management dashboard in `/home3/knowledge`, including active-store selection, CRUD dialogs, and shared active-store state for other knowledge views.

**Architecture:** Add a dedicated Knowledge Store view component and a small Svelte 5 runes store for active selection. Extend the kb service layer with knowledge-store CRUD helpers, wire the route to render the new dashboard for the Knowledge Stores section, and keep the UI aligned with the existing home3 shell while making the cards selection-first and management-friendly.

**Tech Stack:** SvelteKit, Svelte 5 runes, TypeScript, existing home3 component patterns, fetch-based kb service helpers

---

## Chunk 1: Frontend Data Contracts and Active Store State

### Task 1: Add knowledge store service types and requests

- [ ] Define `KnowledgeStoreRecord` and CRUD payload/response types in `web/src/lib/services/kbService.ts`.
- [ ] Add `listKnowledgeStores`, `createKnowledgeStore`, `updateKnowledgeStore`, and `deleteKnowledgeStore` fetch helpers.
- [ ] Reuse the existing `fetchOrThrow` pattern so errors stay consistent with the rest of the kb UI.

### Task 2: Add a shared active knowledge store store

- [ ] Create `web/src/lib/components/home3/knowledge-store-state.svelte.ts`.
- [ ] Track the active store id and active store object with simple setters/selectors.
- [ ] Keep the store narrowly scoped so other `/home3/knowledge` sections can consume it later without coupling to page UI.

## Chunk 2: Knowledge Stores Dashboard UI

### Task 3: Build the dashboard component

- [ ] Create `web/src/lib/components/home3/knowledge-store-view.svelte`.
- [ ] Render the header banner, active-store indicator, add button, responsive card grid, and selected-card highlight.
- [ ] Show card identity/source details first and operational badges second.
- [ ] Implement loading, empty, and error states.

### Task 4: Add create/edit/delete dialogs

- [ ] Implement create and edit flows with a shared form state inside the dashboard component.
- [ ] Implement delete confirmation with special copy when deleting the active store.
- [ ] Refresh local state optimistically where safe, otherwise reload after mutation.

## Chunk 3: Route Integration and Verification

### Task 5: Wire the view into the knowledge route

- [ ] Update `web/src/routes/home3/knowledge/+page.svelte` so `kb-search` renders the new dashboard component instead of the placeholder.
- [ ] Default the page to `kb-search` if that better matches the new user flow, or keep the existing default if preserving current behavior matters more.

### Task 6: Verify

- [ ] Run `npm run check` from `ChenWeb/web`.
- [ ] Fix any type or Svelte issues introduced by the new page.
- [ ] Manually review the visual hierarchy for desktop and mobile breakpoints.
