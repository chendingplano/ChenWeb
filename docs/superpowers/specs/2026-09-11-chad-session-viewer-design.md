# Chad Session Viewer Design

Date: 2026-09-11
Status: Approved for implementation

## Goal

Add a read-only page to the `/development` dashboard at `System Admin → LLM → Chat Sessions`. The page lists every session stored in the ChenWeb server user's `~/.chad/sessions` directory. Selecting a session displays its request and model-response log.

## Scope and assumptions

- “Every session” means every readable session under the server process user's `~/.chad/sessions`, regardless of the logged-in ChenWeb user.
- Session data is local JSON written by Chad. Current files contain session metadata and a `messages` array with roles such as `system`, `user`, and `assistant`; the viewer must tolerate structured content and future message fields without exposing arbitrary files.
- The feature is read-only. It does not delete, edit, or persist session data in ChenWeb's database.
- Access follows the existing authenticated `/api/v1` middleware and protected `/development` route.

## Navigation and UI

Add a `Chat Sessions` leaf under the existing `sysadmin-llm` group in `web/src/lib/components/home3/nav-rail.svelte`, with id `sysadmin-llm-chat-sessions`. Add a matching branch in `content-panel.svelte`.

The view is a full-height two-pane operational screen consistent with other System Admin pages:

- Header: title, short description, session count, and a refresh control.
- Session list: newest updated first. Each row shows session id, updated time, working directory when available, and message count. The selected row is highlighted.
- Detail pane: selected session metadata followed by chronological message cards. User requests and assistant responses have distinct role labels and visual treatment; system messages are subdued. Content is rendered as readable text when possible and formatted JSON for arrays/objects.
- States: loading, empty directory, list error, detail error, unreadable/malformed session items, and sessions with no messages. Refresh should preserve the selected id when it still exists.

The component accepts the existing `darkMode` prop and uses local design tokens matching the dashboard. It should remain usable at narrow widths by allowing the list pane to collapse above the detail pane rather than relying on horizontal overflow.

## Backend API and data flow

Create a focused `chadsessionshandler` package and register authenticated routes:

- `GET /api/v1/chad/sessions` returns session summaries.
- `GET /api/v1/chad/sessions/:id` returns one session's metadata and messages.

The handler resolves the current process user's home directory with `os.UserHomeDir()` and appends the fixed `.chad/sessions` path. It only accepts a session id that names a direct child directory and a JSON file selected by the server-side directory scan. It must not accept arbitrary paths or use user-controlled path components without validation. Detail responses should be bounded to avoid turning a very large session file into an unbounded response; the chosen limit should be a named constant and return a clear error when exceeded.

Listing scans direct child directories, reads the session index/JSON metadata needed for summaries, sorts by update time descending, and skips individual malformed or unreadable entries while returning usable sessions. Detail lookup returns `404` for an unknown id and a structured `4xx/5xx` error for invalid or unreadable data. Do not include server filesystem paths beyond the session's stored working directory metadata.

The frontend client owns fetch/error normalization and exposes typed list/detail functions. The view loads the list on mount, selects the first session by default, fetches detail on selection, and refreshes both list and the selected detail.

## Error handling and security

- All API routes remain behind `authmiddleware.AuthMiddleware` by registering them on the existing authenticated API group.
- Reject empty ids, separators, `.`/`..`, and ids that do not match the directory entry selected by the list/detail lookup.
- Never serve the session directory as static content.
- Treat session content as untrusted text: render it as text or escaped formatted JSON, never as HTML.
- Log API failures through the repository's standard logger with a unique location, without logging full prompts or responses.
- A malformed individual file must not prevent other sessions from appearing.

## Testing and verification

Backend tests will cover:

- summary listing and newest-first ordering;
- detail retrieval and missing-session `404`;
- traversal/separator rejection;
- malformed/unreadable entries being isolated from valid entries;
- bounded detail behavior.

Frontend client tests will cover request URLs, successful typed responses, and non-JSON/HTTP error normalization. Verification will include the relevant Go tests, frontend tests/build, and a manual authenticated browser check that the new leaf appears at the requested navigation location and selecting a real session renders request/response messages.

## Non-goals

- Database ingestion, full-text search, filtering, deletion, export, or live tailing.
- Supporting session stores outside the server user's `~/.chad/sessions`.
- Replacing the existing Chat page or its database-backed `/api/v1/chatter` stubs.
