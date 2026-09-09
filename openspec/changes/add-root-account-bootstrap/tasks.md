## 1. Shared library: Kratos create-with-username helper

- [ ] 1.1 In `shared/go/api/auth/kratos.go`, add `KratosCreateIdentityWithUsernameAndPassword(logger, username, email, firstName, lastName, plaintextPassword string) (*ApiTypes.UserInfo, error)`, modeled on `KratosCreateIdentityWithPassword`: same `POST /admin/identities` flow, 201/409 handling, and activity log; `traits` includes `username` always and `email` + `name` + a verified `verifiable_addresses` entry only when the respective args are non-empty.
- [ ] 1.2 Add a short doc comment noting it is additive and that `KratosCreateIdentityWithPassword` (email-only) is unchanged.
- [ ] 1.3 `cd shared/go && go build ./... && go vet ./... && go test ./...` — all pass.
- [ ] 1.4 From workspace root: `go work sync`.

## 2. ChenWeb: rootaccount package

- [ ] 2.1 Create `server/api/rootaccount/config.go`: `rootAccountConfig{Username, Email, Password string}` and `loadRootAccountConfig() rootAccountConfig` reading `CHENWEB_ROOT_ACCOUNT_USERNAME` (default `root`), `CHENWEB_ROOT_ACCOUNT_EMAIL` (default `root@chenweb.local`), `CHENWEB_ROOT_ACCOUNT_PASSWORD` (no default), each `strings.TrimSpace`d.
- [ ] 2.2 Create `server/api/rootaccount/rootaccount.go` with `EnsureRootAccount(ctx context.Context, logger ApiTypes.JimoLogger) error` implementing D4 steps 1–5: skip when `AUTH_USE_KRATOS != "true"` (info log), skip when password empty (warn log), no-op when an identity exists for the username **or** the email identifier (info log with id), otherwise create via the new shared helper then `auth.KratosUpdateIdentity` to set `metadata_public.admin = true`, `metadata_public.is_owner = true`, and `roles = ["root", "admin"]` (D7).
- [ ] 2.3 Use a logger created with `loggerutil.CreateDefaultLogger` only if `main.go` does not already pass one in; prefer accepting the caller's logger. Give every log line a `loc` tag.
- [ ] 2.4 Return `nil` for all skip/exists paths; return a wrapped error only for genuine Kratos create/update/lookup failures. Ensure a failed create leaves no partial identity (rely on Kratos atomicity; do not attempt compensating deletes).
- [ ] 2.5 Create `server/api/rootaccount/config_test.go`: table-driven test covering defaults applied, overrides applied (with surrounding whitespace trimmed), and empty-password detection.

## 3. ChenWeb: wire into startup

- [ ] 3.1 In `server/cmd/deepdoc/main.go`, after the `config.RunMigrations` block, add `if err := rootaccount.EnsureRootAccount(bootstrapCtx, logger); err != nil { logger.Error("root account bootstrap failed; continuing startup", "error", err, "loc", "CWB_DDM_<n>") }` and the import.
- [ ] 3.2 Confirm placement is before `e.Start(pp)` and after DB init + migrations; reuse the existing `bootstrapCtx` (3-minute timeout) or add a dedicated short context.
- [ ] 3.3 `mise build-server` succeeds.

## 4. Configuration & docs

- [ ] 4.1 Add to `ChenWeb/.env.example`: `CHENWEB_ROOT_ACCOUNT_USERNAME`, `CHENWEB_ROOT_ACCOUNT_PASSWORD` (placeholder value), `CHENWEB_ROOT_ACCOUNT_EMAIL`, with comments — including the note that changing the password after first boot has no effect (delete the identity or use Kratos recovery to re-seed).
- [ ] 4.2 Add the real `CHENWEB_ROOT_ACCOUNT_USERNAME=root` and `CHENWEB_ROOT_ACCOUNT_PASSWORD=...` to the local `ChenWeb/.env` (gitignored; not part of the commit).
- [ ] 4.3 Update the doc comment at the top of `server/cmd/create-admin/main.go` to cross-reference the automatic startup bootstrap (`server/api/rootaccount`).
- [ ] 4.4 Add a short "Root account bootstrap" paragraph to `ChenWeb/CLAUDE.md` near the auth notes: what it does, the `.env` keys, and that the interim login identifier is `root@chenweb.local`.
- [ ] 4.5 Coding-best-practice checklist (`ChenWeb/CLAUDE.md`): record what knowledge changed, which docs were updated, and that username-based UI login is intentionally left for a separate change.

## 5. Verification

- [ ] 5.1 Fresh-store check: ensure no `root` identity exists (`curl "$KRATOS_ADMIN_URL/admin/identities?credentials_identifier=root"` → `[]`), start the server, confirm the info log "root account created" with an id, and re-run the curl to see the identity with both `username` and `email` identifiers and `metadata_public.admin=true`, `metadata_public.is_owner=true`, `roles` containing `root` + `admin`.
- [ ] 5.2 Idempotency check: restart the server; confirm the "already exists" info log and that no `POST`/`PUT` is made (identity `updated_at` unchanged).
- [ ] 5.3 Password-change no-op check: change `CHENWEB_ROOT_ACCOUNT_PASSWORD` in `.env`, restart, confirm the stored credential is unchanged (old password still logs in).
- [ ] 5.4 Skip checks: with `CHENWEB_ROOT_ACCOUNT_PASSWORD` unset → warn log, no identity; with `AUTH_USE_KRATOS` unset → info "skipped" log, server starts.
- [ ] 5.5 Non-fatal check: point `KRATOS_ADMIN_URL` at a dead port, start the server, confirm it logs the error and still reaches "http server started".
- [ ] 5.6 Login check: through the ChenWeb UI, log in with `root@chenweb.local` + the configured password; confirm admin-only user management (`/useradmin` or equivalent) is accessible.
- [ ] 5.6a Owner-protection check: as the root user (or another admin), attempt to delete the root account and to remove its admin flag via user management; both are rejected ("Owner accounts cannot be deleted" / "Owner accounts must remain admin-enabled").
- [ ] 5.7 Regression check: confirm an existing `tax` (Mirai) email login still works against the shared Kratos instance.
- [ ] 5.8 Grep the tree to confirm no root password literal is present in committed code or committed config (only `.env` / `.env.example` placeholder).
