# Benchmark PDF Split Viewer Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split benchmark details into independently scrolling result and PDF panels, with source navigation and highlighting when a result entry is selected.

**Architecture:** Extend `MetricScoreView` to own active input/PDF state and selected source lines, and render the existing `PdfViewWindow` beside the current results. Resolve row IDs against benchmark input records, map supported source spans through `getRawLines`, and use the viewer's existing page binding and highlight callback. Add localized panel labels and empty-state text.

**Tech Stack:** Svelte 5, TypeScript, Paraglide, existing `PdfViewWindow` / `SharedPdfViewer`, `kbService.getRawLines`.

---

## Files

- Modify `web/src/lib/components/home3/metric-score-view.svelte` for input/PDF state, source row selection, the split layout, and responsive independently scrolling panel styles.
- Modify `web/messages/en.json` and `web/messages/zh-cn.json` for any new viewer panel labels and empty state.
- Do not modify `pdf-view-window.svelte` or `shared-pdf-viewer.svelte`; their existing `inputId`, `fileUrl`, `page`, `highlightVersion`, and `renderHighlights` interfaces are sufficient.

## Chunk 1: Benchmark result and PDF integration

### Task 1: Load PDF and source lines for the active input

- [ ] Derive the active input ID from the open result run, falling back to the selected document.
- [ ] Load raw lines through `getRawLines` with request sequencing so stale responses cannot replace data for a newly selected input.
- [ ] Reset page to 1 and clear selected source lines when the active input changes.
- [ ] Clear the selected entry and source lines when the open run changes, including when the input record stays the same.
- [ ] Resolve matched (`gold` + `pred`), missed (`gold`), and false-positive (`pred`) entry IDs to their respective records and parse the supported source-span formats.
- [ ] Combine matched-pair source lines and sort by line number; sort missed/false-positive source lines by line number. Select the first resolvable line for PDF navigation.
- [ ] Convert each raw line's `[x1, y1, x2, y2]` coordinates from the 1000-by-1000 coordinate space into viewport coordinates, and render `.pdf-highlight` rectangles for all valid resolved lines.
- [ ] Selecting an entry replaces the previous highlight set, navigates to its first resolvable page, and increments the existing highlight version.
- [ ] Handle empty/missing source spans and raw-line failures by clearing highlights and retaining the current PDF page.

### Task 2: Add independently scrolling panels and localized content

- [ ] Wrap the current benchmark result detail in the left panel and add an adjacent right panel containing `PdfViewWindow` for the active input ID.
- [ ] Keep PDF visible when a run has no score or metric rows; show a localized empty state only when no input ID is selected.
- [ ] Constrain both panels to independent scroll containers; stack them on narrow viewports.
- [ ] Add every new visible string in English and Chinese through Paraglide.
- [ ] Preserve text selection and add keyboard-accessible focusable result entries.

### Task 3: Review and static validation

- [ ] Review the diff against `docs/superpowers/specs/2026-10-07-benchmark-pdf-split-design.md` and confirm `server/api/kbhandler/metric_score_runner.go` has no changes beyond its pre-existing working-copy diff.
- [ ] Run `bun run check` from `web/` for Svelte, TypeScript, and localization consistency.
- [ ] Do not run test suites unless specifically requested.
