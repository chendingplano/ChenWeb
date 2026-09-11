# Session JSON Display Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:executing-plans to implement this plan.

**Goal:** Render Pi/Chad session message content and tool-call parameters as readable recursive name/value pairs with interpreted string control characters.

**Architecture:** Extract the formatter into a focused TypeScript utility so its output can be tested independently, then use it from `chad-sessions-view.svelte` for both content fields. The utility will return escaped HTML fragments with indentation and `white-space: pre-wrap` on string values; invalid/non-JSON strings use an escaped text fallback.

**Tech Stack:** Svelte 5, TypeScript, Vitest, Bun.

---

## Chunk 1: Formatter behavior

### Task 1: Add failing formatter tests

**Files:**
- Create: `web/src/lib/components/home3/session-json-formatter.test.ts`

- [ ] **Step 1: Write tests** for nested object key/value output, array indexes, decoded newline display, invalid JSON fallback, and HTML escaping.
- [ ] **Step 2: Run the focused test** with `bun test web/src/lib/components/home3/session-json-formatter.test.ts`; confirm it fails because the formatter is not implemented.

### Task 2: Implement the formatter

**Files:**
- Create: `web/src/lib/components/home3/session-json-formatter.ts`

- [ ] **Step 1: Implement a recursive `formatSessionJson(value: unknown): string` helper** that parses valid JSON strings, renders objects and arrays recursively, preserves string whitespace, escapes text, and falls back to escaped plain text.
- [ ] **Step 2: Run the focused test** and confirm all formatter tests pass.

## Chunk 2: Integrate both session content surfaces

### Task 3: Replace raw serialization in the session view

**Files:**
- Modify: `web/src/lib/components/home3/chad-sessions-view.svelte`

- [ ] **Step 1: Import the formatter and replace the message-content formatter use.**
- [ ] **Step 2: Replace tool-call parameter serialization with the same formatter.**
- [ ] **Step 3: Keep token-count serialization behavior unchanged.**
- [ ] **Step 4: Run the focused formatter test and `bun run check` from `web/`.

## Chunk 3: Verification and handoff

- [ ] **Step 1: Inspect the diff** to ensure only the formatter, view, tests, and approved plan/spec files changed; do not include unrelated pre-existing changes.
- [ ] **Step 2: Run the relevant frontend verification command** from `ChenWeb/web` and record its exit status/output.
- [ ] **Step 3: Manually verify the Pi Sessions view with a message and tool-call parameter containing nested JSON and `\\n`.
- [ ] **Step 4: Commit implementation files with `jj` and verify the linear `jj log`.
