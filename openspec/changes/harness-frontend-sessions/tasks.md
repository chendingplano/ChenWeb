## 1. Gateway (gateways/claude-code)

- [x] 1.1 `protocol.ts`: request validation, event types, history message mapping
- [x] 1.2 `session.ts`: live session (streaming input, one turn at a time, idle close, resume), event mapping, permission requests
- [x] 1.3 `server.ts`: routes, secret, workspace confinement, child environment
- [x] 1.4 Tests with a scripted `query()`; `bun run check`
- [x] 1.5 Live check: two turns on the real harness, the second remembering the first

## 2. ChenWeb (server/api/harnessfrontend)

- [x] 2.1 Protocol-v1 client and registry
- [x] 2.2 Admin-only routes, SSE relay
- [x] 2.3 `routes.go` registration from `CLAUDE_CODE_GATEWAY_URL` / `CLAUDE_CODE_GATEWAY_SECRET`
- [x] 2.4 Go tests; `go vet`; `mise build-server`

## 3. Docs

- [x] 3.1 Gateway README, `mise dev-claude-code-gateway`
- [x] 3.2 Update devdoc 2026101802
