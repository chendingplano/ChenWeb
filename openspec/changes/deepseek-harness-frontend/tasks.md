## 1. Gateway (gateways/deepseek-harness)

- [x] 1.1 `acp.ts`: JSON-RPC over the stdio of `dsh --profile acp`, initialize, dsh's requests
- [x] 1.2 `sessionlog.ts`: find and read dsh session logs (v4); history, title, first prompt, folder check
- [x] 1.3 `harness.ts`: sessions, one turn at a time, idle close and resume, update mapping, approvals, models, listing
- [x] 1.4 `server.ts`: protocol-v1 routes, secret, workspace confinement, child environment
- [x] 1.5 Tests with a scripted ACP server and fixture logs; `bun run check`
- [x] 1.6 Live check on the real dsh: tool use, memory across turns, an approval, history, listing

## 2. ChenWeb

- [x] 2.1 `routes.go`: register `deepseek-harness` from `DEEPSEEK_HARNESS_GATEWAY_URL` / `_SECRET`
- [x] 2.2 Agent menu entry; `HARNESS_BY_AGENT`; gateway harnesses kept in a saved agent list; service test
- [x] 2.3 `go test`, `go vet`, `mise build-server`; `bun run check`; live Go test (`HARNESS_NAME=deepseek-harness`) through the real handler
- [ ] 2.4 Browser check on the Chat page with DeepSeek Harness selected

## 3. Docs

- [x] 3.1 Gateway README, `mise dev-deepseek-harness-gateway`, `mise.local.toml` settings
- [x] 3.2 Update devdoc 2026101802 and handoff 2026091501
