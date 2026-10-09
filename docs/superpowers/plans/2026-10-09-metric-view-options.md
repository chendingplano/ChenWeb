# Metric View Options Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add exclusive gold/dropped metric list modes and an optional set of PDF info cards for all metrics in the active view.

**Architecture:** Keep list mode, filtering, and card data in `metric-mgmt-view.svelte`. Extend `PdfViewWindow` and `shared-pdf-viewer.svelte` with an optional page-aware multi-card snippet while preserving the existing single-card API behavior.

**Tech Stack:** Svelte 5, TypeScript, Paraglide messages, existing PDF overlay components.

---

## Chunk 1: Metric list controls and data

- [ ] Replace `showDropped` with a gold/dropped view mode; gold is default.
- [ ] Load dropped rows through the existing `include_dropped` option and filter the returned union to the selected exclusive mode.
- [ ] Add the View selector and independent all-info-box checkbox below Order by.
- [ ] Add English and Chinese messages for all new controls.
- [ ] Verify refreshes and record changes respect the selected mode.

## Chunk 2: PDF info cards

- [ ] Extend the PDF viewer overlay snippet to receive the page number and support page-specific multiple cards.
- [ ] Preserve the existing selected metric card placement when all-card mode is off.
- [ ] In all-card mode, assign each visible metric to its first resolvable source page; order cards by source line and metric ID; omit unresolvable metrics.
- [ ] Stack cards at the upper right with a bounded, scrollable container.
- [ ] Review the final diff and run `bun run check` from `web/` per ChenWeb guidance.
