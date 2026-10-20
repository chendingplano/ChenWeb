## Why

Rollout step 4 of devdoc 2026101802 adds a second harness as a vertical slice. Claude Code was chosen: the Claude Agent SDK can switch off every built-in tool and route all tool use through a host callback. That makes it possible to confine a run to SemOS's capability-checked knowledge tools, which is the compatibility test the devdoc sets.

## What Changes

- New private Bun service `gateways/claude-code`. It drives the Claude Agent SDK with built-in tools, settings, skills, plugins, other MCP servers and session persistence switched off. SemOS's six tools are the only tools, served from an in-process MCP server, and every call passes `canUseTool`.
- The gateway speaks the existing SemOS gateway protocol: the same run request, NDJSON event vocabulary and routes as the Pi gateway.
- New Go `ClaudeCodeHarnessAdapter` (harness `claude-code`). It declares streaming, cancellation, interactive approval, usage, history replay and history summary, but not context capacity. It accepts only provider `anthropic`, through a new optional `HarnessProfileValidator` hook.
- ChenWeb registers the adapter only when `CLAUDE_CODE_GATEWAY_URL` and `CLAUDE_CODE_GATEWAY_SECRET` are set. The internal tool routes accept that gateway's secret as well as Pi's.
- The profile admin API and page let an administrator pick a service's harness from the registered adapters.

## Capabilities

### Modified Capabilities
- `agent-harness-adapters`: adds the Claude Code adapter, the profile-validator hook, and harness selection on profile versions.

## Impact

- New: `gateways/claude-code/` (TypeScript, Bun), `server/api/agentservicehandler/claude_code_adapter.go`.
- Changed: `harness.go`, `tools.go`, `profile_validation.go`, `profile_admin.go`, `routes.go`, the Pi harness admin page and service types, two pairs of i18n messages, `mise.toml`, and the operations doc.
- No migration. No change to the browser conversation API.
