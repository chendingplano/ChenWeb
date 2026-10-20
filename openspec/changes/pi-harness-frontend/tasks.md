## 1. Gateway (gateways/pi)

- [x] 1.1 `rpc.ts`: `pi --mode rpc` over stdio, LF-only framing, command ids and responses, events, extension UI requests
- [x] 1.2 `sessionfile.ts`: Pi's session folder for the workspace, v3 files, active branch, history, title, first prompt
- [x] 1.3 `harness.ts`: one Pi process per session, one turn at a time, idle stop and resume, event mapping, extension dialogs, models, listing
- [x] 1.4 `server.ts`: protocol-v1 routes, secret, workspace confinement, child environment
- [x] 1.5 Tests with a scripted Pi and fixture session files; `bun run check`
- [x] 1.6 Live check on the real Pi: tool use, memory across turns, an extension approval, history, listing, resume in a new process

## 2. ChenWeb

- [x] 2.1 `routes.go`: register `pi` from `PI_HARNESS_GATEWAY_URL` / `_SECRET`
- [x] 2.2 `HARNESS_BY_AGENT` maps the existing `pi` menu entry; service test
- [x] 2.3 `go test`, `go vet`, `mise build-server`; live Go test (`HARNESS_NAME=pi`) through the real handler
- [ ] 2.4 Browser check on the Chat page with `pi` selected
- [ ] 2.5 Decide whether to remove System Admin → LLM → Pi Sessions (reads the old database copy)

## 3. Pi package (ThirdParty/pi)

- [x] 3.1 Remove the session-copy extension (`harness-session-persistence.ts`, its test) and the `postgres` dependency

## 4. Docs

- [x] 4.1 Gateway README, `mise dev-pi-harness-gateway`, `mise.local.toml` settings
- [x] 4.2 Update devdoc 2026101802 and handoff 2026091501
