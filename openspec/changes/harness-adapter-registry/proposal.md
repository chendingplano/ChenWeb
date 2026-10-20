## Why

SemOS's agentic services run only on Pi. The handler boundary already uses harness-neutral names (`HarnessAdapter`, `AgentRunRequest`), but `RunHandler` holds exactly one adapter. It calls every optional operation (approval, context-window lookup, history summary) as if every harness supported it, and it does not record which harness ran a turn. Before a second harness (Claude Code, Codex, ...) can be added, SemOS needs rollout steps 2–3 of devdoc 2026101802: adapters registered by name, a capability declaration that is checked before a run, and a per-run record of how the run was executed.

## What Changes

- Each `HarnessAdapter` describes itself: harness name, adapter version, declared capabilities, and the largest run request it accepts.
- A `HarnessRegistry` holds adapters by name. Startup registers Pi and checks every active service profile against it.
- `AgentServiceProfile` gains a server-owned `harness` field. Profiles stored without it are read as `pi`.
- A service's required capabilities are derived from its profile. SemOS rejects a profile whose harness is unregistered or lacks a required capability. This happens at startup (logged), at activation on the Pi admin page (rejected), and at conversation creation and run start (a clear "unavailable" result).
- Optional operations run only when the adapter declares them. Without context-capacity reporting the fallback history budget is used. Without history summary, folding is skipped and the turn trims instead. Without interactive approval, `ask` permission mode and permission decisions are refused.
- `kb.agentic_response_attempts` records the effective `harness`, `adapter_version` and `model_name` of each run. Cancel and permission decisions are routed to the adapter recorded on the run.
- Pi-specific wording is removed from run-handler logs and the public start-failure message.

## Capabilities

### New Capabilities
- `agent-harness-adapters`: registering harness adapters, declaring and checking capabilities, and recording the effective harness per run.

## Impact

- Code: `server/api/agentservicehandler/` (new `harness.go`; `run_handler.go`, `handler.go`, `gateway.go`, `profiles.go`, `profile_store.go`, `profile_validation.go`, `profile_admin.go`, `store.go`), `server/api/routes.go`.
- Database: one goose migration adding three columns to `kb.agentic_response_attempts` (existing rows default to `pi`).
- No browser API change other than the wording of the start-failure error. No Pi gateway protocol change.
