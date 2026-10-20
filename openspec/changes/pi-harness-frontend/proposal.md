## Why

ChenWeb is a front end to Claude Code, Codex and DeepSeek Harness (changes `harness-frontend-sessions`, `codex-harness-frontend`, `deepseek-harness-frontend`). Pi (installed globally as `pi`, with the workspace package in `ThirdParty/pi`) should work the same way: Pi keeps its own instructions, tools, extensions, skills, settings, login and session files, and ChenWeb only sends messages, shows the replies and relays the user's answers.

The earlier Pi session integration did the opposite. A Pi extension (`ThirdParty/pi/extensions/harness-session-persistence.ts`) copied every Pi session into ChenWeb's database (`kb.harness_sessions`, `kb.harness_messages`) after each run, and the chat page's `pi` entry was not connected to Pi at all. In the front-end model ChenWeb stores no copy of the conversation; when a page needs the history, it asks the harness.

## What Changes

- **New gateway** `gateways/pi`: a private Bun service that speaks harness front-end protocol v1 and drives Pi through `pi --mode rpc`, Pi's headless JSON protocol over stdio, one Pi process per open session. It works in `PI_HARNESS_WORKSPACE` and uses Pi's own login (`~/.pi/agent`). RPC mode reports only the session it runs, so the gateway reads history, titles and the session list from Pi's own session files.
- **ChenWeb registration**: `routes.go` registers the `pi` harness when `PI_HARNESS_GATEWAY_URL` and `PI_HARNESS_GATEWAY_SECRET` are set.
- **Chat page**: the existing `pi` entry in the agent menu is mapped to the `pi` gateway in `HARNESS_BY_AGENT`.
- **Pi package** (`ThirdParty/pi`, its own repository): the session-copy extension and its `postgres` dependency are removed; `extensions/index.ts` stays as an empty entry point.
- A `mise dev-pi-harness-gateway` task.

## Capabilities

### Modified Capabilities
- `harness-frontend`: Pi is a fourth harness behind the same protocol and chat page.

## Impact

- New: `gateways/pi/`, `mise dev-pi-harness-gateway`.
- Changed: `server/api/routes.go` (one list entry), `web/src/lib/services/harnessFrontendService.ts` (one map entry), `server/api/harnessfrontend/live_test.go` (comment).
- No database migration. No change to protocol v1, to `server/api/harnessfrontend`, or to the Knowledge Desk services (`agentservicehandler`, `ThirdParty/pi/gateway`).
- Left as is: `kb.harness_sessions` / `kb.harness_messages` (Chad still writes them) and the System Admin → LLM → **Pi Sessions** page, which now shows only the rows copied before this change. Whether to remove that page is open.
