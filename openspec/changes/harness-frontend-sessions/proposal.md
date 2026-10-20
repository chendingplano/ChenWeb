## Why

ChenWeb should be a front end for an agent harness, the way VS Code is a front end for Claude Code. The harness keeps its own system prompt, loop, tools, memory, skills, fallback handling and conversation history. ChenWeb sends the user's message, relays what the harness streams back, and passes the user's approvals to it. ChenWeb does not store or rebuild history, and it does not replace the harness's tools. The first harness is Claude Code, signed in with the machine's own Claude Code login and working in the configured workspace folder. Coding is the first use, but nothing in the package limits it to coding.

The Knowledge Desk services (`agentservicehandler`, Pi) are not changed by this change.

## What Changes

- **New gateway** `gateways/claude-code`: a private Bun service that runs the **full** Claude Code harness through the Claude Agent SDK. It uses Claude Code's own system prompt and tools, user/project/local settings, and persisted sessions. It runs in `CLAUDE_CODE_WORKSPACE`, with the user's Claude Code login (API-key variables are removed from its environment). Each session stays alive between turns, like a terminal; an idle session is closed and later resumed from Claude Code's own transcript.
- **Harness front-end protocol v1**, served by the gateway:
  - list sessions
  - read a session's messages
  - send a turn (NDJSON event stream)
  - interrupt
  - answer a permission request (allow or deny, optionally with updated input, for example answers to `AskUserQuestion`)
- **New Go package** `server/api/harnessfrontend`. It holds harness gateways by name, speaks protocol v1 to them, and exposes admin-only routes under `/api/v1/harness-frontend/:harness/...`, relaying each turn as SSE. No ChenWeb table is needed: sessions and history are read from the harness.

## Capabilities

### New Capabilities
- `harness-frontend`: ChenWeb as a front end to an external agent harness.

## Impact

- New: `gateways/claude-code/`, `server/api/harnessfrontend/`, a `mise dev-claude-code-gateway` task.
- Changed: `server/api/routes.go` (route registration when the gateway is configured).
- No database migration. No front-end page yet; that comes in a later change.
