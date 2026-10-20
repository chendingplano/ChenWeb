## 1. Gateway (gateways/codex)

- [x] 1.1 `appserver.ts`: JSON-RPC over the stdio of `codex app-server`, initialize, server requests
- [x] 1.2 `items.ts`: Codex items to tool calls, results and session history
- [x] 1.3 `harness.ts`: threads, one turn at a time, idle unload and resume, event mapping, approvals and questions, models, sessions
- [x] 1.4 `server.ts`: protocol-v1 routes, secret, workspace confinement, child environment
- [x] 1.5 Tests with a scripted app-server; `bun run check`
- [x] 1.6 Live check on the real Codex: tool use, memory across turns, an approval, history, listing

## 2. ChenWeb

- [x] 2.1 `routes.go`: register `codex` from `CODEX_GATEWAY_URL` / `CODEX_GATEWAY_SECRET`
- [x] 2.2 `HARNESS_BY_AGENT`: Codex → `codex`; service test
- [x] 2.3 `go test`, `go vet`, `mise build-server`; `bun run check`; live Go test (`HARNESS_NAME=codex`) through the real handler
- [ ] 2.4 Browser check on the Chat page with Codex selected

## 3. Docs

- [x] 3.1 Gateway README, `mise dev-codex-gateway`
- [x] 3.2 Update devdoc 2026101802 and handoff 2026091501
