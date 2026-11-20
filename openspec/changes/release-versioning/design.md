## Context

- **Releases page today** (`server/api/releaseshandler/`, `releases-view.svelte`, devdoc
  `2026111901`): free-text major/minor, a form that saves a release with its items in one
  go, items replaced wholesale on edit. Not tied to builds. `public.releases` and
  `public.release_items` hold 0 rows on the Mac (`miner`) and on prod (screenshot,
  2026-11-19).
- **Builds** run on the Mac (`[deployment] role = "build"`): the Deployment page starts
  `deploy-runner build`, which runs `shell_server_build_rcp.sh` → `mise run
  build-server-linux` (web, prompt defaults, binaries with embedded metadata, staging,
  MANIFEST) → pdf-parser sources → `tar` → `scp` to the relay → `::archive::<name>`. The
  script is also run by hand and must keep working that way (`deployment-page` spec
  "Scripts stay usable by hand"). About two builds a day.
- **Deploys** run on the China box (`role = "deploy"`): the page shows the newest relay
  archive (`shell_server_deploy.sh --newest`), then `StartDeployment {archive_name}` queues
  a row and starts `chenweb-deploy`, which runs `shell_server_deploy.sh` with
  `RELAY_ARCHIVE`. `StartDeployment` already accepts any valid archive name; only the
  listing is newest-only. Every deploy restarts the `chenweb` server.
- The relay is slow from China (down to ~19 KB/s), so the box must not download archives
  just to read their notes.
- Decisions taken with the user (2026-11-19): the build machine owns versions and items;
  other machines are reference only. A successful build is a new revision. Prod stores the
  release it runs and may skip releases or deploy an older one. `shared` ships inside the
  ChenWeb release for now. Only ChenWeb is enabled; tax comes later. Notes come as plain
  text and PDF on both machines. `00` means "not created yet".

## Goals / Non-Goals

**Goals:**
- Version numbers assigned by the system, never typed: majors and minors by an admin,
  revisions by successful builds.
- Pending items that become part of exactly one release, chosen at build time.
- Release notes in the archive, and viewable as text or PDF for one release or a range.
- The deploy page lets the admin choose a release and shows everything since the running one.

**Non-Goals:**
- Releasing `shared` on its own, or tax releases (the `project` column is ready for them).
- Editing or deleting a released release or its items.
- Linking issue numbers to GitHub, or reading items from commit messages.
- A public (non-admin) release notes page.
- Deleting old archives from the relay.

## Decisions

### D1. The build machine is the single writer
All writes (create major/minor, add/edit/delete pending items) are accepted only where
`[deployment] role = "build"`. Any other machine answers 403 `read_only` and the page hides
the editing controls. Prod's tables are a mirror filled from release files (D8).
*Alternative:* let prod own items and assign versions at deploy. Rejected by the user: fixes
are made on the Mac, and versions must exist at build time.

### D2. Storage
Three tables in `kb` (the deployment tables live there too):

- `kb.releases`: `id` BIGSERIAL; `project` TEXT (CHECK `ChenWeb` for now); `major`, `minor`,
  `revision` SMALLINT, CHECK 0–99; `kind` TEXT (`major` | `minor` | `build`); `status`
  (`building` | `released`); `description` (major/minor description, or the build's
  description); `build_number` TEXT NULL UNIQUE; `release_date` DATE; `git_rev`,
  `shared_git_rev` TEXT, `git_dirty` BOOL; `created_by`, `created_at`. UNIQUE
  (`project`, `major`, `minor`, `revision`).
- `kb.release_items`: `id` BIGSERIAL; `project`; `release_id` BIGINT NULL FK → releases
  (NULL = pending); `item_type` (`new feature` | `improvement` | `bug fix`); `component`
  (`ChenWeb` | `shared`); `description`; `issue` TEXT (digits or empty); `pull_request`,
  `notes` TEXT; `created_by`, `created_at`, `updated_at`.
- `kb.release_current`: `project` PK; `major`, `minor`, `revision`; `build_number`;
  `updated_at`. On the build machine: the latest version created (by a build, or by a new
  major/minor). On the deploy machine: the release it is running.

Parts are stored as integers and formatted `%02d` everywhere (`01.02.03`), so ordering is
plain `ORDER BY major, minor, revision` and `12.2` vs `12.02` cannot both exist.
*Alternative:* derive "current" from `MAX` over releases. Rejected: on prod "current" is what
is installed, which history cannot tell; on the Mac a separate row keeps the counter
update explicit and lockable (`SELECT … FOR UPDATE`).

Majors and minors are rows in `kb.releases` (revision `00`, kind `major`/`minor`) so their
descriptions sit in the same ordered history as builds and appear in range notes.

No new cross-table IDs: releases are identified across machines by project + version, and
build numbers already exist. The ID manager is not needed.

### D3. Version arithmetic
- Start: no row → `00.00.00`.
- New major: `M.m.r` → `(M+1).00.00`. New minor: `M.m.r` → `M.(m+1).00`. Build:
  `M.m.r` → `M.m.(r+1)`. A build may follow a new major directly (`02.00.01`).
- Any part that would pass 99 is refused with a message naming what to create instead
  (revision 99 → "create a new minor"). At two builds a day `.99` arrives in about seven
  weeks.
- Creating a major or minor is refused while a release is `building` (D5).

### D4. Item life cycle
Pending (`release_id` NULL) → bound to a `building` release when a build starts → released
when that build commits. Pending items can be edited and deleted; bound and released items
cannot. Items added while a build runs stay pending for the next one. Binding at build start
(not at the end) is what keeps the notes in the archive equal to what the database records.

### D5. Build flow
`shell_server_build_rcp.sh` drives it, so page builds and hand builds behave the same:

1. `check` step: the script builds `release-tool` once and runs `release-tool reserve --pid $$`
   (under `mise exec`, which supplies the environment). In one transaction it reverts a stale
   `building` row (see Risks), inserts the next revision as `building` with the script's host
   and pid (`builder_host`, `builder_pid`), binds all pending items, and prints
   `RELEASE_VERSION=<v>`. The shared logger also writes to stdout, so the script picks that
   line out by pattern. The script exports `RELEASE_VERSION`.
2. `mise run build-server-linux` embeds `release_version` in `buildinfo` metadata and adds
   `release_version:` and `shared_git_rev:` / `shared_git_dirty:` lines to MANIFEST.
3. New `release` step (after pdf-parser, before archive): `release-tool notes --version V
   --build-number N --out /tmp/chenweb-deploy/release` records the build number and git
   revisions, then writes `release.json`, `RELEASE_NOTES.txt`, `RELEASE_NOTES.pdf` (and the
   `zh-cn` pair, D10).
4. `archive`, then `upload` sends the archive and the sidecar
   `chenweb-deploy-<build>.release.json` (a copy of `release.json`) to the relay.
5. New `release-commit` step: `release-tool commit --version V` sets `released`, the release
   date, and `kb.release_current`, and prints `::release::V`. For page builds, `deploy-runner`
   reads that marker and stores it in `kb.deployment_builds.release_version` (the row's
   build number is only filled when the run finishes, so the tool cannot find it).
6. A shell `trap` on any failure runs `release-tool abort --version V`: deletes the
   `building` row, which returns its items to pending. If the failure is after upload, the
   script first removes the uploaded archive and sidecar from the relay, so a version never
   appears on the relay without being committed.

*Upload before commit:* upload failures (network) are common and need no clean-up this way;
commit failures (a local UPDATE) are rare and are the only case that touches the relay.
*Alternative:* do all of this inside `deploy-runner`. Rejected: hand builds bypass
`deploy-runner`, and the version must exist before the binaries are compiled.

`mise run build-server-linux` run alone (no `RELEASE_VERSION`) still works; its metadata says
no release version, and it is not a release.

### D6. `release-tool` command
`server/cmd/release-tool` (like `deploy-runner`: loads `.env`, config, shared DB pool;
logger via `CreateDefaultLogger`). Subcommands: `reserve`, `notes`, `commit`, `abort`,
`add-item`, `import <release.json>`. All writing subcommands except `import` refuse unless
`role = "build"`. `add-item` is how agents record a fix:
`release-tool add-item --type "bug fix" --component ChenWeb --issue 123 "Description"`.
`ChenWeb/CLAUDE.md` gets a rule to run it for every fix.

### D7. Release files
`release.json` = `{project, version, build_number, release_date, git_rev, shared_git_rev,
history: [every release up to and including this one, each with its items]}`. Carrying the
whole history means the box needs only the newest file to know every release, and importing
an older file never loses anything (D8). Size grows by roughly 1–2 KB per release.

The archive's `release/` directory holds `release.json` and the notes files for this
release. The sidecar on the relay is the same `release.json`.

### D8. The deploy machine's mirror
`import` upserts by (project, version) and never deletes or rewrites a `released` row, so it
is idempotent and safe in any order. It runs:
- at `chenweb` server start where `role = "deploy"`, from the installed
  `<ChenWeb>/release/release.json`, then sets `kb.release_current` from that file's own
  version and build number;
- when the Deploy picker lists archives, from the newest sidecar on the relay (so releases
  not yet deployed can be shown and noted).

*Why server start, not `deploy-runner`:* during the deploy that first ships this feature the
running `deploy-runner` is the old binary; the server restarts on every deploy, so it always
sees the new file. `shell_server_deploy.sh` gains a `release` step (before
`deploy-server-china.sh` restarts anything) that replaces `<ChenWeb>/release/` with the
archive's copy, or removes it for an archive without one. A server that starts without the
file clears its current release.

After a successful deploy, `deploy-runner` also sets the current release from the queued row
(or clears it when the archive had no release). This covers a rollback to a build from
before this change: that server cannot record anything, but the runner doing the deploy is
always the newer binary.

### D9. Choosing a release to deploy
- `shell_server_deploy.sh --list` prints every `*-deploy-*.tar.gz` as `<name> <size> <mtime>`
  (newest first); `--release-file <name>` prints one sidecar. `--newest` stays for hand use.
- New `GET /api/v1/deployment/archives` (deploy role): lists archives, fetches and imports
  the newest sidecar, and returns each archive with its build number, version (or none for
  archives built before this change), date and item count, plus the running release.
  Replaces `next-archive` on the page.
- The page shows the list (newest selected), and for the selected release the notes from
  the running release to it. When the selection is older than the running release, it shows
  a rollback warning listing the releases that will be removed. "View PDF" opens the same
  range as PDF. Confirm calls the existing `StartDeployment`.
- `kb.deployments` gains `release_version` and `previous_release_version`, filled at queue
  time; history rows show the version next to the build number.

### D10. Notes rendering
Go builds the notes from rows (no LLM, no prompt file): a Markdown document and a plain text
document. Range = releases with version in (from, to], newest first; each build release lists
its items grouped New features / Improvements / Bug fixes; a major or minor inside the range
appears as a heading with its description. Item line: `- <description> (#<issue>)` for
ChenWeb, `(shared#<issue>)` for shared, `[shared]` prefix for shared items without an issue.
Headings exist in English and Chinese (a small Go table; this is server-generated text, so
the API takes `lang` per `ChenWeb/CLAUDE.md` §3). The build writes both languages into
`release/`.

PDF: the same renderer writes Typst source (every piece of text as a quoted Typst string,
so item text can never be read as markup) and runs `typst compile`, caching the result as
`.cache/release-notes/<sha256 of the source>.pdf`. Released history never changes, so the
cache never needs invalidating. The font list (Helvetica, Arial, PingFang SC, SimHei,
SimSun) plus Typst's automatic fallback covers Chinese on both machines, the same way the
doc-review reports do.

*Typst directly, not pandoc (changed 2026-11-19):* the first version reused Document
Builder's `docbuilder.PandocConvert`, but pandoc is not installed on the China box, and
Ubuntu 18.04's package is too old to drive Typst. The box already has `typst` and Chinese
fonts (runbook §6.1), so rendering with Typst alone needs nothing new on any machine. Only
basic Typst features are used (`set text`, `heading`, `list`, `strong`), which the box's
0.14.2 supports, like the doc-review template.

Endpoint: `GET /api/v1/releases/notes?project=ChenWeb&from=01.02.03&to=01.02.06&format=text|pdf&lang=en|zh-cn`;
`from` empty = from the beginning. Notes for a single release run from the previous *build*
release, so the first build of a new major or minor also shows that major's or minor's
heading (`PreviousBuild`); the build's `RELEASE_NOTES.*` files use the same range.

### D11. API (replaces the current four routes)
| Method and path | Role | Purpose |
|---|---|---|
| `GET /api/v1/releases?project=` | any | current, next build version, editable flag, history with items, pending items |
| `POST /api/v1/releases/majors` `{project, description}` | build | new major |
| `POST /api/v1/releases/minors` `{project, description}` | build | new minor |
| `POST /api/v1/releases/items` | build | add pending item |
| `PUT /api/v1/releases/items/:id` | build | edit pending item |
| `DELETE /api/v1/releases/items/:id` | build | delete pending item |
| `GET /api/v1/releases/notes` | any | D10 |
| `GET /api/v1/deployment/archives` | deploy | D9 |

All require an admin (same `authorize` rule as today). Log locations follow
`CreateDefaultLogger` with new `YYYYMMDD-DDD` codes.

### D12. Page UI rules (docs/frontend-rules.md)
The Releases page uses inline panels under their buttons (new major/minor, add/edit item,
delete confirmation) instead of dialogs, so no new dialog component was needed. The deploy
release picker extends the Deployment page's existing anchored, draggable, resizable dialog.
The Releases page keeps its drafts, open panels, expanded releases and scroll position in
`sessionStorage` (restored during component set-up, before the save effect first runs).

## Reused modules

- **20261109-02 Document Builder (backend)** — its `PandocConvert` was used first and then
  dropped (see D10): it needs pandoc, which the China box lacks. The other `typst compile`
  callers (doc-review reports, CDM publishing) run the command inline inside their own
  workflows; there is no shared helper to reuse, and the call is one line.
- **20261109-01 Markdown Viewer** — considered; not used. The pages show the plain-text form
  in a `<pre>`, so no Markdown endpoint was needed.
- **20261114-06 PDF View Window** — considered; not used. It is built around highlighted
  evidence lines; a release-notes PDF opens in a new tab with the browser's viewer.
- Nothing catalogued covers version numbers, release items or relay listing. The new code
  stays in ChenWeb (`releaseshandler`, `release-tool`, `deployment`). When tax adopts it, the
  version/item/notes core moves to `shared/go` and is registered with
  `create-reusable-module` then.

## Risks / Trade-offs

- [The build machine's `miner` database is a dev database] → every archive (and the relay)
  holds the full history; `release-tool import <release.json>` restores it. Never point
  `TEST_DATABASE_URL` at `miner`.
- [A build killed hard leaves a `building` row and locked items] → the row records the build
  script's host and pid; the next `reserve` on the same host reverts it when that process is
  gone, and refuses while it is alive. This works for hand builds, which have no
  `kb.deployment_builds` row. The Releases page shows a building release with its age.
- [typst missing on a machine] → the PDF endpoint answers 503 naming it; text notes still
  work. Both current machines have it.
- [Two-digit limit] → a clear refusal at `reserve`, before any compile; the page shows how
  many revisions are left once fewer than 10 remain.
- [Sidecar fetch over the slow relay link] → one small file per listing (tens of KB after a
  year); listing failures show the error and still allow nothing to start, as today.
- [Commit fails after upload] → the script removes the uploaded files before aborting (D5).

## Migration Plan

1. Goose migration: abort if `public.release_items` or `public.releases` has rows; otherwise
   drop them and create the three `kb` tables; add `release_version` to
   `kb.deployment_builds`, and `release_version` / `previous_release_version` to
   `kb.deployments`. Apply on the Mac (`doc-service --migrate-only`, since air ignores
   `.sql`).
2. On the Mac: create major `01` (and a minor if wanted), add items, run a build. Check the
   archive's `release/` and the sidecar on the relay.
3. Deploy that build to prod with the current page (newest archive). The new server
   migrates, imports `release/release.json` and records the running release. From the next
   deploy the picker is used.
4. Nothing to install on the box: `typst` and Chinese fonts are already there (runbook §6.1).
5. Rollback: deploying a pre-change archive works (old server ignores the new tables); the
   picker lists such archives without a version. Do not run the migration's Down on prod.

## Open Questions

- **Starting version.** The plan starts at `00.00.00`, so the first major is `01`. If ChenWeb
  should continue an existing numbering (the old form's placeholder showed 12.2), add a
  one-time "set starting version" while the project has no releases. Default: not built.
- **Who adds items on the page.** Admins only, as today. The CLI works for anyone with the
  build machine's database credentials.
