## Why

ChenWeb is a front end to Claude Code (change `harness-frontend-sessions`). The same front end should work for Codex, the way the Codex VS Code extension does: Codex keeps its own instructions, tools, sandbox and approval rules, memories and thread history, and ChenWeb only sends messages, shows the replies and relays approvals. The protocol and the chat page were written to be harness-neutral, so Codex needs its own gateway and little else.

## What Changes

- **New gateway** `gateways/codex`: a private Bun service that speaks harness front-end protocol v1 and drives Codex through `codex app-server` (the JSON-RPC interface the Codex IDE extension uses). It uses the machine's Codex login and works in `CODEX_WORKSPACE`. One app-server process serves every session; an idle thread is unloaded and later resumed from Codex's own history.
- **ChenWeb registration**: `routes.go` registers the `codex` harness when `CODEX_GATEWAY_URL` and `CODEX_GATEWAY_SECRET` are set (the Claude Code registration becomes a two-entry list).
- **Chat page**: `HARNESS_BY_AGENT` maps the existing **Codex** agent-menu entry to the `codex` gateway. No other page change: Codex's questions are presented in the `AskUserQuestion` shape the page already answers.
- A `mise dev-codex-gateway` task.

## Capabilities

### Modified Capabilities
- `harness-frontend`: Codex is a second harness behind the same protocol and chat page.

## Impact

- New: `gateways/codex/`, `mise dev-codex-gateway`.
- Changed: `server/api/routes.go`, `web/src/lib/services/harnessFrontendService.ts` (one map entry).
- No database migration. No change to protocol v1, to `server/api/harnessfrontend`, or to the Knowledge Desk services.
