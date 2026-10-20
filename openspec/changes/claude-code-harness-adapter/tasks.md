## 1. Gateway (gateways/claude-code)

- [x] 1.1 `protocol.ts`: SemOS gateway run/summary validation; provider must be `anthropic`
- [x] 1.2 `tools.ts`: in-process MCP server, `canUseTool` gate (tool boundary, approval, call limit, tool-use ID), ChenWeb tool calls, sources
- [x] 1.3 `run.ts`: confined query options, transcript history, event mapping, output/time limits, summaries
- [x] 1.4 `server.ts`: routes, secret, per-run temp directory, one active run per conversation
- [x] 1.5 `bun test`, `bun run check`; live smoke run (tool loading verified; model call blocked by API key credit)

## 2. ChenWeb

- [x] 2.1 `ClaudeCodeHarnessAdapter` and the `HarnessProfileValidator` hook
- [x] 2.2 Internal tool routes accept each configured gateway secret
- [x] 2.3 `routes.go` registers the adapter when configured
- [x] 2.4 Admin API and page: harness selection, registered harness list, i18n
- [x] 2.5 Go tests; `go vet`; `mise build-server`; `bun run check` in web

## 3. Docs

- [x] 3.1 Gateway README, operations doc section, `mise dev-claude-code-gateway`
- [x] 3.2 Update devdoc 2026101802

## 4. Open

- [ ] 4.1 Live round-trip with a funded credential
- [ ] 4.2 Rollout step 5: run the same evaluation cases against Pi and Claude Code
