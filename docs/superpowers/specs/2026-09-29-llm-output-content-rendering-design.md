# LLM Usage Logs Output Content Rendering

Date: 2026-09-29  
Status: Approved for implementation

## Goal

Make JSON returned as a chat message's `content` value readable in the LLM Usage Logs body dialog.

## Behavior

- Keep the existing recursive name–value rendering for the outer response JSON.
- In any string field, render valid embedded JSON objects and arrays recursively while retaining surrounding text.
- Also parse a string-valued chat message `content` as JSON when the whole value is JSON.
- Keep malformed JSON content as a literal string.
- Continue escaping text through the existing HTML renderer.
- Use a smaller indentation step for nested objects and arrays.
- Render each name–value row on its own line and let value columns use the dialog width.
- Allow the body dialog to be resized while keeping it within the viewport.

## Implementation

Make the narrow change in `llm-usage-logs-view.svelte`'s existing `renderJsonHtml` helper. Preserve current rendering and fallback behavior for all other values.

## Verification

Check formatting/type errors with the project's existing frontend check if available, and inspect the diff. Confirm valid JSON content becomes nested name–value rows and invalid content remains readable text.

## Non-goals

- Changing the body API or stored payloads.
- Parsing JSON scalar values embedded in prose.
- Adding expand/collapse, copy, or edit controls.
