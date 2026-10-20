## Context

Devdoc 2026101802 (harness-neutral agentic services) sets out the architecture. This change implements rollout steps 2 and 3. Step 4 (a second adapter) needs a harness and deployment model chosen first and is out of scope.

## Decisions

**Capabilities are a small fixed vocabulary.** `streaming`, `cancellation`, `interactive_tool_approval`, `usage_reporting`, `history_replay`, `context_capacity_reporting` and `history_summary`. They are typed constants, so a misspelt capability fails to compile instead of silently never matching.

**Requirements are derived, not configured.** Every service requires `streaming` and `cancellation`. A service whose default permission is `ask` requires `interactive_tool_approval`. A service that saves and resumes conversations requires `history_replay`. `usage_reporting`, `context_capacity_reporting` and `history_summary` are optional: SemOS has a fallback for each. This needs no new admin field and no universal config schema (devdoc §2.4).

**Harness is server-owned on the profile.** `harness` is part of the immutable profile version JSON. The Pi admin page always writes `pi`. Older stored versions have no field and are normalized to `pi` on read, so pinned conversations keep working without a data migration.

**Effective execution is recorded per run, not per conversation.** A conversation already pins its profile version, and the version pins the harness. The adapter version can still change between turns, so `kb.agentic_response_attempts` gets `harness`, `adapter_version` and `model_name`, set when the attempt is created. Cancel and Decide read `harness` from the attempt, so they reach the adapter that started the run even if the profile has changed since.

**Request size is adapter-declared.** The 480 KiB history-trimming bound was the Pi gateway's body limit. It moves to `HarnessDescriptor.MaxRequestBytes`.

**Failure is explicit.** Startup does not abort on an incompatible profile. It logs the reason, and that service becomes unavailable. Admin activation of an incompatible profile is rejected with 400.

## Risks

- Changing `RunStore.CreateRunAttempt` breaks test fakes. They are updated in the same change.
- The public start-failure message changes from "Pi gateway unavailable; retry later" to "Agent runtime unavailable; retry later". The frontend shows the server text and does not match on it.
