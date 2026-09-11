# Session JSON Display Design

Date: 2026-09-11  
Status: Approved for implementation

## Goal

Make structured content in Pi and Chad session messages easier to scan. JSON objects should display as readable name/value pairs, and string values should render control characters such as `\\n` as actual line breaks when the value is decoded.

## Scope and behavior

- Apply the same display behavior to `message.content` and `message.toolCallParameters`.
- Render objects recursively as indented key/value rows.
- Render arrays recursively with indexed rows such as `[0]` and `[1]`.
- Render primitive values as text with existing dark/light theme colors.
- Preserve whitespace in string values with `white-space: pre-wrap`.
- When a value is a string containing valid JSON, decode that JSON for structured rendering; otherwise display the string as-is.
- Escape rendered text so session content cannot be interpreted as HTML.

## Implementation

Keep the change localized to `chad-sessions-view.svelte` by replacing the serialized `JSON.stringify` presentation with a recursive renderer. The renderer will return escaped HTML for the existing Svelte `{@html ...}` display, using the component's existing design tokens. The raw fallback remains available for non-JSON content and malformed JSON.

## Testing

Add focused tests for the formatter covering nested objects, arrays, multiline string values, primitive values, invalid JSON, and HTML escaping. Verify the frontend type check/build after the change and manually inspect both a message body and a tool-call parameter in the Pi Sessions view.

## Non-goals

- Changing session storage or API response shapes.
- Adding expand/collapse controls, editing, searching, or copying behavior.
- Changing the separate LLM Usage Logs viewer.
