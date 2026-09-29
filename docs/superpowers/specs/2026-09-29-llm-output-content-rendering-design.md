# LLM Usage Logs Output Content Rendering

Date: 2026-09-29  
Status: Approved for implementation

## Goal

Make JSON returned as a chat message's `content` value readable in the LLM Usage Logs body dialog.

## Behavior

- Keep the existing recursive name–value rendering for the outer response JSON.
- When rendering a chat message object (an object with a string-valued `role` property), parse its string-valued `content` as JSON if valid and render the parsed value recursively.
- Keep malformed JSON content as a literal string. Leave all other string values unchanged.
- Continue escaping text through the existing HTML renderer.
- Use a smaller indentation step for nested objects and arrays.
- Allow the body dialog to be resized while keeping it within the viewport.

## Implementation

Make the narrow change in `llm-usage-logs-view.svelte`'s existing `renderJsonHtml` helper. Preserve current rendering and fallback behavior for all other values.

## Verification

Check formatting/type errors with the project's existing frontend check if available, and inspect the diff. Confirm valid JSON content becomes nested name–value rows and invalid content remains readable text.

## Non-goals

- Changing the body API or stored payloads.
- Parsing JSON-looking strings outside `content` fields.
- Adding expand/collapse, copy, or edit controls.
