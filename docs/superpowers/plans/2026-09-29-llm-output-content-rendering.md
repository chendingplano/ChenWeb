# LLM Usage Logs Output Content Rendering Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Render JSON encoded in chat message `content` values as readable name–value rows in LLM Usage Logs.

**Architecture:** Keep the existing renderer in `llm-usage-logs-view.svelte`. While rendering a chat-message object, attempt to parse its string `content`; use the current renderer on success and the current string fallback on failure.

**Tech Stack:** Svelte 5, TypeScript.

---

## Chunk 1: Render structured message content

**Files:**
- Modify: `web/src/lib/components/home3/llm-usage-logs-view.svelte`

- [x] In `renderJsonHtml`, detect objects with a string `role` property and parse only their string-valued `content` fields as JSON.
- [x] Reuse existing recursive rendering for parsed values; preserve literal rendering on parse failure and for every other string.
- [x] Reduce nested indentation and make the body dialog resizable within viewport bounds.
- [x] Run `bun run check` from `web/` and inspect the focused diff.
