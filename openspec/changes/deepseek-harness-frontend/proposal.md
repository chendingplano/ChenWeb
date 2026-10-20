## Why

ChenWeb is a front end to Claude Code and Codex (changes `harness-frontend-sessions` and `codex-harness-frontend`). DeepSeek Harness (`dsh`, checked out in `ThirdParty-2/deepseek-harness`) should work the same way: dsh keeps its own instructions, tools, sandbox and approval rules, skills, settings and session history, and ChenWeb only sends messages, shows the replies and relays approvals.

## What Changes

- **New gateway** `gateways/deepseek-harness`: a private Bun service that speaks harness front-end protocol v1 and drives dsh through `dsh --profile acp`, dsh's Agent Client Protocol (ACP) server over stdio. One dsh process serves every session. It works in `DEEPSEEK_HARNESS_WORKSPACE` and uses dsh's own login (`~/.dsh`). Because dsh's ACP server does not replay history, the gateway reads a session's history, title and first prompt from dsh's own session log (`~/.dsh/sessions`).
- **ChenWeb registration**: `routes.go` registers the `deepseek-harness` harness when `DEEPSEEK_HARNESS_GATEWAY_URL` and `DEEPSEEK_HARNESS_GATEWAY_SECRET` are set.
- **Chat page**: a **DeepSeek Harness** entry in the agent menu, mapped to the `deepseek-harness` gateway in `HARNESS_BY_AGENT`. Gateway-served harnesses missing from a user's saved agent list are added to the menu, so a saved list does not hide them.
- A `mise dev-deepseek-harness-gateway` task.

## Capabilities

### Modified Capabilities
- `harness-frontend`: DeepSeek Harness is a third harness behind the same protocol and chat page.

## Impact

- New: `gateways/deepseek-harness/`, `mise dev-deepseek-harness-gateway`.
- Changed: `server/api/routes.go` (one list entry), `web/src/lib/services/harnessFrontendService.ts` (one map entry), `web/src/lib/components/shared-ui/chatter-01.svelte` (menu entry).
- No database migration. No change to protocol v1, to `server/api/harnessfrontend`, or to the Knowledge Desk services.
- On this machine: dsh's own login was set up once as `~/.dsh/.env` (mode 600), from the key already in the dsh checkout's `.env`.
