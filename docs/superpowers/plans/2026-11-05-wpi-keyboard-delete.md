# WPI Editor Keyboard Delete Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Delete all selected WPI Editor canvas nodes and edges with either Backspace or Delete.

**Architecture:** Use Svelte Flow's built-in `deleteKey` property with both keys. Its current multi-selection removal and the editor's `onbeforedelete` undo snapshot remain responsible for deletion and history. Keep the loaded-page empty-selection guard unchanged; for a single selected node it retains the tracked ID that lets undo restore the Properties panel selection, while multi-selection has no single tracked ID.

**Tech Stack:** Svelte 5, Svelte Flow, TypeScript, Markdown.

---

## Chunk 1: Enable both delete keys

**Files:**
- Modify: `web/src/lib/components/shared-ui/wpi-editor.svelte` — add both keys to the existing `<SvelteFlow>` props.
- Modify: `../KnowledgeStore/doc-repo/devdocs/202610/2026102701-devdoc-wpi-editor.md` — update the existing keyboard shortcut and undo-history descriptions.

- [x] Set `deleteKey={['Backspace', 'Delete']}` on the editor's `<SvelteFlow>` component.
- [x] Update the WPI Editor devdoc to say that Backspace and Delete remove the selection, including selected nodes and edges, and to list both keys in the undo-history summary; retain its existing description that attached edges are removed.
- [x] Keep the existing loaded-page empty-selection guard in `onselectionchange`; for a single selected node it preserves the tracked ID needed to restore the Properties panel selection on undo.
- [x] Do not add or run tests; verification was not requested.
