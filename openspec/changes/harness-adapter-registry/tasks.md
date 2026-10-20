## 1. Contract and registry

- [x] 1.1 `harness.go`: capability constants, `HarnessDescriptor`, `HarnessRegistry` (register/get/check), derived `RequiredHarnessCapabilities`
- [x] 1.2 `HarnessAdapter.Describe()`; `PiHarnessAdapter` declares all capabilities and its 480 KiB request limit
- [x] 1.3 `AgentServiceProfile.Harness`, normalized to `pi` on env load, admin validation and stored-version decode

## 2. Run recording

- [x] 2.1 Migration adding `harness`, `adapter_version`, `model_name` to `kb.agentic_response_attempts`
- [x] 2.2 `store.go`: `CreateRunAttempt` takes `RunHarness`; attempt scans include the new columns

## 3. Behavior

- [x] 3.1 `RunHandler` resolves the adapter per profile, checks capabilities, gates optional operations, routes Cancel/Decide by the attempt's harness, uses neutral wording
- [x] 3.2 `ConversationHandler` refuses to create conversations for an incompatible service
- [x] 3.3 Profile admin rejects activation of an incompatible profile
- [x] 3.4 `routes.go`: build the registry, register Pi, log incompatible services at startup

## 4. Verification and docs

- [x] 4.1 Tests: registry, capability checks, gated optional operations, recorded harness, legacy profile decode
- [x] 4.2 `go test`, `go vet`, `mise build-server`
- [x] 4.3 Update devdoc 2026101802 (§2.1, §2.6, §3)
