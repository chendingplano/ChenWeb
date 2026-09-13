# KB Products Viewer Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose the existing `kb.products` record/source-document viewer immediately after Metrics in Knowledge System.

**Architecture:** Reuse `ProductsView` → `KbExtractionView` → `KbInputRecordBrowser` and the existing `listKbProducts` API. Only navigation order, product-specific presentation, and focused tests need changing.

**Tech Stack:** Svelte 5, SvelteKit, TypeScript, Vitest.

---

## Chunk 1: Navigation and presentation

- [x] Move the `kb-products` menu item directly after `kb-metrics` in `web/src/routes/home3/knowledge/+page.svelte`.
- [x] Update `ProductsView` copy and remove the related-products detail group so the viewer presents product records and source documents only.
- [x] Run the focused frontend checks and inspect the diff.
