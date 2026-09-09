## Context

ChenWeb runs with `AUTH_USE_KRATOS="true"` (`ChenWeb/.env`). Every account is an
Ory Kratos identity; there is no local users table. Today the only way to get an
admin is to run `server/cmd/create-admin` by hand, which creates an **email-only**
Kratos identity via `auth.KratosCreateIdentityWithPassword(...)` and then calls
`auth.KratosUpdateIdentity(...)` to set `metadata_public.admin = true` /
`roles = ["admin"]`.

Relevant existing pieces:

- **Startup sequence** — `server/cmd/deepdoc/main.go`: load `.env` → load
  `config.toml` → `databaseutil.InitDB` → `sysdatastores.CreateSysTables` +
  `database.CreateTables` → `config.RunMigrations` → `seed.EnsureCuratedModules`
  → background workers → `promptoptimizerhandler.SeedBuiltinTemplates` (logged,
  non-fatal) → `e.Start(pp)`. There is clear precedent for "ensure/seed X on
  boot, log and continue on failure".
- **Kratos identity schema** — `Kratos/kratos/identity.schema.json` already
  defines a `username` trait (`^[a-zA-Z0-9_-]+$`, len 3–50) as a `password`
  identifier, and `traits.anyOf` already allows `{required: ["username"]}`. An
  identity with both `username` and `email` identifiers is schema-valid. This
  schema is shared with `tax` (Mirai) on the same Kratos instance.
- **Existence lookup** — `auth.KratosGetIdentityByEmail(logger, x)` queries
  `GET /admin/identities?credentials_identifier=<x>`; despite the name it matches
  **any** credential identifier, so it works for a username too.
- **Login path** — `auth.HandleEmailLoginKratos` rejects any identifier failing
  `isValidEmail(...)`, and the ChenWeb login form is email-only. A pure `username`
  identity therefore cannot log in through the current UI.
- **Roles** — `config.local.toml [system].access_roles` already includes `root`;
  `useradminhandler.canManageAllUsers` already grants full access to role `root`.
  The role catalog labels `root` "reserved", which this change activates.
- **Config hygiene** — `ChenWeb/.env` and `config.local.toml` are gitignored;
  `config.toml` is committed. `ChenWeb/CLAUDE.md` §1.2 "Simplicity First" and
  "NEVER hard-code" (prompts, and by extension secrets).

## Goals / Non-Goals

**Goals:**

- Guarantee a privileged root account exists after every ChenWeb server start,
  with no manual step.
- Match the requested credentials: username `root`, the given password.
- Idempotent and safe: never disturb an account that already exists.
- No secret in committed code or committed config.
- A Kratos outage at boot must not stop the server.

**Non-Goals:**

- Username-based login in the shared handler or the ChenWeb login form. Out of
  scope; tracked separately. Until then the account logs in with its synthetic
  email.
- Rotating or reconciling the root password from `.env` on restart.
- Any change to `tax` / Mirai behavior.
- A migration or DB schema change (this is an identity-store concern, not a
  Postgres one).
- Wiring the bootstrap into `server/cmd/dataservice` (batch worker, not the web
  server).

## Decisions

### D1 — One identity, two identifiers (`username` + synthetic email)

Create a single Kratos identity whose `traits` are
`{username: "<user>", email: "<email>", name: {first: "root"}}`, with a
`password` credential and the email listed in `verifiable_addresses` as
`verified: true` (so Kratos attempts no verification mail to the unroutable
`chenweb.local`).

- **Why:** The email identifier makes the account usable **today** through the
  existing email login form; the `username` identifier is created now so it
  "just works" the moment username login is added, with no data backfill.
- **Alternatives:**
  - *Username only* — literal match to the request but produces a non-loginable
    account until a larger frontend/handler change ships. Rejected.
  - *Synthetic email only* — simplest, but drops the `root` identifier the user
    asked for. Rejected.
  - *Also build username login end-to-end now* — larger, touches shared/go and
    the SvelteKit login form; deferred to its own change.

### D2 — New shared helper `KratosCreateIdentityWithUsernameAndPassword`

Add to `shared/go/api/auth/kratos.go`:

```go
func KratosCreateIdentityWithUsernameAndPassword(
    logger ApiTypes.JimoLogger,
    username, email, firstName, lastName, plaintextPassword string,
) (*ApiTypes.UserInfo, error)
```

Body mirrors `KratosCreateIdentityWithPassword` (same Admin API `POST
/admin/identities`, same 201/409 handling, same activity log) but puts both
`username` and `email` in `traits`. `email`/`firstName`/`lastName` may be empty,
in which case those trait keys / the `verifiable_addresses` entry are omitted.

- **Why shared:** genuinely reusable — `tax` also runs on this Kratos instance —
  and additive: the existing email-only helper is untouched, so no dependent
  project is affected (`shared/` backward-compat rule).
- **Alternative:** call the Kratos Admin API inline from ChenWeb. Rejected —
  duplicates ~70 lines of request/response/activity-log handling that already
  exists in the shared package.

### D3 — Existence check reuses `KratosGetIdentityByEmail`

Call `auth.KratosGetIdentityByEmail(logger, username)`; if not found, call it
again with the email. Create only if **both** miss.

- **Why:** that function already does an identifier-based `credentials_identifier`
  lookup; no new shared code needed for the read path. Checking both identifiers
  avoids a duplicate-identity error if a prior partial run created one of them.
- **Alternative:** add `KratosGetIdentityByIdentifier` as a rename + back-compat
  wrapper. Deferred — pure cosmetics, and "surgical changes" argues against
  touching a widely-used shared function for naming alone. A `// identifier, not
  only email` comment at the call site suffices.

### D4 — New ChenWeb package `server/api/rootaccount/`

```
server/api/rootaccount/
  rootaccount.go        // EnsureRootAccount(ctx, logger) error
  config.go             // rootAccountConfig{} + loadRootAccountConfig() from env
  config_test.go        // table test for the pure env parsing / skip logic
```

`EnsureRootAccount`:

1. If `os.Getenv("AUTH_USE_KRATOS") != "true"` → log info "skipped (kratos auth
   disabled)", return nil.
2. `cfg := loadRootAccountConfig()` — reads `CHENWEB_ROOT_ACCOUNT_USERNAME`
   (default `root`), `CHENWEB_ROOT_ACCOUNT_EMAIL` (default `root@chenweb.local`),
   `CHENWEB_ROOT_ACCOUNT_PASSWORD` (no default), all `strings.TrimSpace`d.
3. If `cfg.Password == ""` → log warn "skipped (no CHENWEB_ROOT_ACCOUNT_PASSWORD
   set)", return nil.
4. If `KratosGetIdentityByEmail(logger, cfg.Username)` or
   `...(logger, cfg.Email)` returns an identity → log info "root account already
   exists" with the id, return nil.
5. Else `KratosCreateIdentityWithUsernameAndPassword(logger, cfg.Username,
   cfg.Email, "root", "", cfg.Password)`, then `KratosUpdateIdentity(logger, id,
   KratosIdentityUpdate{MetadataPublic: {"admin": true, "is_owner": true,
   "roles": ["root", "admin"]}})`. Log the created id.

### D7 — `metadata_public.is_owner = true` on the root identity

The root identity is marked an owner. `useradminhandler` already special-cases
owners: `DeleteUser` returns 400 "Owner accounts cannot be deleted" and
`UpdateUser` returns 400 if an owner would be left non-admin. `is_owner` is read
back from `metadata_public["is_owner"]` in `auth.KratosIdentityToUserInfo` /
`KratosListAllIdentities`.

- **Why:** this is the break-glass administrator; it should not be removable or
  demotable through the normal user-management UI by accident.
- **Trade-off:** the account can then only be removed via the Kratos Admin API
  directly (or by another owner). Acceptable and arguably the point.
- **Alternative:** leave `is_owner` unset and rely on `roles` alone. Rejected —
  the request explicitly asked for owner protection.

- **Why a package, not a function in `main`:** keeps `main.go` to a one-line call
  (matches how `seed`, `promptoptimizerhandler` are wired), and lets the pure
  config/skip logic be unit-tested without a live Kratos.
- **Why `firstName = "root"`:** `useradminhandler` / the user list UI render
  `name.first`; without it the row shows blank.

### D5 — Call site: after `config.RunMigrations`, non-fatal

In `server/cmd/deepdoc/main.go`, immediately after the migrations block (before or
right after `seed.EnsureCuratedModules`):

```go
if err := rootaccount.EnsureRootAccount(bootstrapCtx, logger); err != nil {
    logger.Error("root account bootstrap failed; continuing startup",
        "error", err, "loc", "CWB_DDM_2xx")
}
```

`EnsureRootAccount` itself returns `nil` for the skip cases and only returns a
non-nil error for genuine Kratos failures; the call site logs and proceeds either
way — matching `SeedBuiltinTemplates`.

- **Why non-fatal:** a transient Kratos hiccup should not wedge the whole server,
  and `create-admin` remains as a manual fallback. The loud `logger.Error` plus
  the next-restart retry make a persistent failure visible without an outage.
- **Alternative:** `os.Exit(1)` on failure (like `EnsureCuratedModules`).
  Rejected — that couples web availability to the identity service being up at
  the exact moment of deploy.

### D6 — Credentials in `.env`, `.env.example` documents them

`.env` (gitignored, per host) carries the real values; `.env.example` (committed)
lists the keys with comments and a placeholder password. No `config.toml` /
`config.local.toml` entry, no Go constant.

- **Why:** `.env` already holds the other auth secrets (`KRATOS_*`, OAuth). Keeps
  the secret out of git and satisfies the "no hard-coded" rule.

## Risks / Trade-offs

- **Root account not loginable via `root` until username login ships** → The
  synthetic email is a working identifier today; the change is explicit that
  `root@chenweb.local` is the interim login. Documented in `.env.example` and
  `ChenWeb/CLAUDE.md`.
- **Synthetic email can't do password recovery / verification mail** → Acceptable
  for a break-glass admin. Operators who want recovery set
  `CHENWEB_ROOT_ACCOUNT_EMAIL` to a real mailbox in `.env`.
- **Shared Kratos instance also serves `tax`** → The new helper is additive and
  the identity schema already has `username`; no `tax` code path changes. Task
  list includes a smoke check that `tax` email login still works.
- **`.env` password changed but account already exists** → By design the restart
  is a no-op; the new password does **not** take effect. This is called out in
  `.env.example` ("changing this after first boot has no effect; use the Kratos
  recovery/settings flow or delete the identity to re-seed").
- **Kratos password policy rejects the configured password** → Create fails,
  logged at error, no partial identity (Kratos `POST /admin/identities` is
  atomic). Operator fixes the password in `.env` and restarts.
- **Two identities created by a race / earlier partial run** → Mitigated by
  checking both identifiers before create (D3); Kratos also enforces identifier
  uniqueness and returns 409, which the helper surfaces as a logged error.

## Migration Plan

1. Ship shared helper (`shared/go/api/auth`) — additive, `cd shared/go && go test
   ./...`, then `go work sync` from workspace root.
2. Ship ChenWeb package + call site + `.env.example`.
3. On each ChenWeb host: add `CHENWEB_ROOT_ACCOUNT_USERNAME` /
   `CHENWEB_ROOT_ACCOUNT_PASSWORD` to `.env`, restart the server.
4. Verify: `curl "$KRATOS_ADMIN_URL/admin/identities?credentials_identifier=root"`
   returns the identity; log in through the UI with `root@chenweb.local`.
5. **Rollback:** revert the call site (the helper is dormant if unused).
   Optionally delete the seeded identity via the Kratos Admin API. No schema or
   data migration to undo.

## Open Questions

- Preferred home for the operator-facing note: a short section in
  `ChenWeb/CLAUDE.md` vs. a new `ChenWeb/docs/root-account.md`. Defaulting to a
  paragraph in `ChenWeb/CLAUDE.md` next to the auth notes.
