## Why

A fresh ChenWeb deployment has no administrator: someone has to remember to run the
`server/cmd/create-admin` CLI by hand before anyone can log in and manage users. If
the Kratos identity store is ever wiped or rebuilt, the system silently comes back
up with zero admins. We want the ChenWeb server to guarantee a known-good root
account exists every time it starts, with no manual step.

## What Changes

- On ChenWeb server startup (`server/cmd/deepdoc`), after migrations, the server
  ensures a **root account** exists in Kratos and creates it if it does not.
- The root account is a single Kratos identity carrying **both** a `username`
  (`root`) and a synthetic email (`root@chenweb.local` by default) as password
  login identifiers, with `metadata_public.admin = true`,
  `metadata_public.roles = ["root", "admin"]`, and `metadata_public.is_owner =
  true` (delete- and de-admin-protected via `useradminhandler`).
- Credentials come from `.env` (gitignored): `CHENWEB_ROOT_ACCOUNT_USERNAME`,
  `CHENWEB_ROOT_ACCOUNT_PASSWORD`, and optional `CHENWEB_ROOT_ACCOUNT_EMAIL`.
- Behavior is **create-if-missing only**: if the identity already exists the server
  leaves it untouched (it never auto-resets the password or rewrites roles).
- If `CHENWEB_ROOT_ACCOUNT_PASSWORD` is empty, or `AUTH_USE_KRATOS != "true"`, the
  bootstrap logs a warning and is skipped. Any Kratos error during bootstrap is
  logged loudly but is **non-fatal** — the server still starts.
- New shared helper `auth.KratosCreateIdentityWithUsernameAndPassword(...)` in
  `shared/go/api/auth` (additive; the existing email-only
  `KratosCreateIdentityWithPassword` is unchanged).
- `.env.example` gains the three new keys; the `create-admin` CLI doc comment is
  updated to point at the automatic bootstrap.

Not in scope: adding username-based login to the shared login handler or the
ChenWeb login form. Until that lands, the root account is logged in with the
synthetic email; the `root` username identifier is created now so it works the
moment username login is added.

## Capabilities

### New Capabilities

- `root-account-bootstrap`: On ChenWeb server startup, idempotently ensure a
  privileged root account exists in the Kratos identity store, sourced from
  environment configuration, without overwriting an existing account.

### Modified Capabilities

<!-- None. No existing openspec capability spec covers auth/startup behavior. -->

## Impact

- **Code**
  - `shared/go/api/auth/kratos.go` — new `KratosCreateIdentityWithUsernameAndPassword`
    helper (additive).
  - `ChenWeb/server/api/rootaccount/` — new package: `EnsureRootAccount(ctx, logger)`
    plus pure env-config parsing.
  - `ChenWeb/server/cmd/deepdoc/main.go` — one call site after
    `config.RunMigrations` / alongside `seed.EnsureCuratedModules`.
  - `ChenWeb/server/cmd/config/config.go` — none required (env-only knobs).
- **Config / ops**
  - `ChenWeb/.env` (gitignored, on each host) — must define
    `CHENWEB_ROOT_ACCOUNT_USERNAME` / `CHENWEB_ROOT_ACCOUNT_PASSWORD`.
  - `ChenWeb/.env.example` — documents the new keys.
- **External systems**
  - Ory Kratos Admin API (`KRATOS_ADMIN_URL`, already required by ChenWeb). One
    extra `GET /admin/identities?credentials_identifier=` on every boot, and one
    `POST /admin/identities` only on first boot / after an identity wipe.
  - Shared Kratos instance is also used by `tax` (Mirai); the new helper is
    additive and changes no shared behavior, so `tax` is unaffected.
- **Docs**
  - `ChenWeb/CLAUDE.md` (or `ChenWeb/docs/`) — note the startup bootstrap.
  - `server/cmd/create-admin/main.go` doc comment — cross-reference.
