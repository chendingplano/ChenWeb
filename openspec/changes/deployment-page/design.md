## Context

A deployment today:

1. **Mac (build machine).** `shell_server_build_rcp.sh` runs `mise run build-server-linux`
   (embedded frontend, five linux/amd64 binaries, migrations, prompts, configs into
   `/tmp/chenweb-deploy`), stages Document Builder files and pdf-parser sources, packs
   `/tmp/chenweb-deploy-<YYYYmmdd-HHMM>.tar.gz` and copies it with `scp` to the relay
   `cding@192.168.29.96:/home/cding/Backups`.
2. **China box (deployment machine).** As root, `shell_server_deploy.sh`
   downloads the newest `*-deploy-*.tar.gz` from the relay (reached from outside as
   `47.189.245.217:8896`), extracts it and calls `deploy-server-china.sh`. That script
   syncs migrations, prompts and configs, then for each binary verifies the checksum,
   backs it up, stops the service, swaps the binary, starts the service, runs a health
   check and rolls back on failure. Back in `shell_server_deploy.sh`, the pdf-parser
   Docker image is rebuilt and verified, and `pdf-parser` is restarted.

Facts that shape the design:

- On the box every service is a systemd unit (`chenweb`, `doc-processor`,
  `parser-result-converter`, `doc-service`, `kratos`, `nats-server`, `pdf-parser`).
  `chenweb` runs as `gui`. `gui` has a passwordless sudoers rule
  (`/etc/sudoers.d/gui-chenweb-services`) for `systemctl start|stop|restart|status` on
  every unit except `pdf-parser`, and is in the `docker`, `adm` and `systemd-journal`
  groups. `deploy-server-china.sh` already runs as `gui`. `shell_server_deploy.sh`
  requires root only because of `systemctl restart pdf-parser` and `rsync --chown`.
- A deploy stops and restarts `chenweb`. When systemd stops a unit it kills every process
  in the unit's control group, including children started with `setsid`.
- On the Mac, `chenweb` runs under `air`, which watches `server/` for `.html` files among
  others. The build rewrites `server/api/webbuild/`, so `air` restarts `chenweb` in the
  middle of a build.
- Nothing supervises the Mac's services; they run in terminals.
- The Mac and the box have separate databases.
- The System Monitor page reads log files named by environment variables. On the box,
  `system-monitor-journal@<unit>` followers copy each unit's journal into those files.
  `system-monitor-view.svelte` can already show one source (`sourceId` prop); the Doc
  Processor dashboard embeds it this way for `pdf-parser`.
- `server/buildinfo` reports each running Go executable's build number over NATS.

Decisions already taken with the user: services stay under systemd and everything runs as
`gui` (no root at deploy time); Restart exists only on the box; Deploy installs the newest
archive on the relay.

## Goals / Non-Goals

**Goals:**

- One button builds and uploads an archive on the Mac; one button deploys it on the box.
- Each step's status and output are visible while it runs; the first failure stops the
  run and is shown as the failed step with its output.
- Every build and every deployment leaves a database record with its steps.
- Deploy progress survives `chenweb` restarting during the deploy.
- The box's services can be restarted from the page, and each service's log is visible
  next to it.
- The scripts remain usable by hand and stay the single description of the procedure.

**Non-Goals:**

- Cancelling a running build or deployment.
- Choosing an older archive or rolling back a deployment from the page.
- Building on the box or deploying from the Mac.
- Restarting the Mac's processes.
- Retention or cleanup of build and deployment records.
- Changing what the build packs or what the deploy installs (other than the archive name
  and shipping `deploy-runner-linux`).

## Decisions

### D1. The machine's role comes from `[deployment] role` in `config.local.toml`

`role = "build"` on the Mac, `role = "deploy"` on the box. `config.local.toml` is
machine-specific and never shipped, so each machine keeps its own value. With the key
unset the page explains which key to set, offers no Build or Deploy action, and lists the
processes without Restart.

*Alternatives:* detecting the role from the OS is implicit and wrong for a Linux dev
machine; offering both buttons everywhere lets someone press Build on the box, which has
no Go or mise.

### D2. Work runs in a separate `deploy-runner` process, never inside `chenweb`

New command `server/cmd/deploy-runner` with two subcommands:

- `deploy-runner build --build-id <id>` (Mac). `chenweb` inserts the build row, then
  starts the runner detached in its own session (`Setsid`), so `air` restarting `chenweb`
  does not stop it.
- `deploy-runner deploy` (box). Runs as the systemd unit `chenweb-deploy.service`
  (`User=gui`, `Type=simple`, `WorkingDirectory=/home/gui/Workspace/ChenWeb`, the same
  `EnvironmentFile=` and `SHARED_LIB_CONFIG_DIR` as `chenweb`; `PROJECT_WORKSPACE_DIR` comes
  from the box's `.env`). The unit file is `scripts/chenweb-deploy.service`. `chenweb` inserts the
  deployment row as `queued` and runs `sudo -n /bin/systemctl start chenweb-deploy`, which
  returns as soon as the process starts. The runner claims the oldest `queued` row with
  `SELECT … FOR UPDATE SKIP LOCKED`. Being its own unit, it is not killed when the deploy
  restarts `chenweb`.

The runner opens the project database with `databaseutil.InitDB` as other ChenWeb
commands do. It does not run goose migrations: migrations stay the job of the services
the deploy restarts, exactly as today.

*Alternatives:* a goroutine in `chenweb` dies with every restart; `setsid` on the box does
not leave `chenweb`'s control group; `systemd-run --user` needs lingering and a user bus
for `gui` (root setup anyway, and fragile on Ubuntu 18.04); a template unit
`chenweb-deploy@<id>` would need a sudoers wildcard.

### D3. The runner wraps the existing scripts; steps come from markers

The scripts stay the procedure. They print one marker line when a step begins:

```
::step::<key>            e.g. ::step::web
::step::<key>::<arg>     e.g. ::step::binary::doc-processor
```

The runner holds a planned step list per run kind (below), creates every step as
`pending` so the page shows the whole plan at once, and reads the script's combined
stdout and stderr:

- a marker starts that step and marks the running step `succeeded`;
- every other line is appended to the running step's output;
- exit status 0 marks the running step `succeeded` and any step that never started
  `skipped`;
- a non-zero exit marks the running step `failed` (the first step, if no marker was seen),
  later steps `skipped`, and the run `failed` with the last output lines as its error.

The scripts already use `set -euo pipefail`, so they stop at the first failing command.
A marker whose key is not in the plan is appended as an extra step, so a script change
never breaks a run. A Go test reads the scripts and fails when a marker key and the plan
disagree. Step output is written to the database at most once a second and capped at the
last 256 KB per step, with `output_truncated` set when lines were dropped.

Markers carry keys, not text, so the page translates each step (`dep_step_<key>`, with
`<arg>` as a parameter).

**Build plan:** `check` (arguments, workspace, database settings) → `web` (embedded
frontend) → `prompt-defaults` (doc-processor's embedded prompt defaults) → `binary::<name>` for `server`, `doc-processor`, `parser-result-converter`,
`doc-service`, `create-admin`, `deploy-runner` → `stage` (migrations, prompts, reviewer
configs, config, templates) → `documents` (Document Builder files) → `pdf-parser` (parser
sources) → `archive` → `upload` (scp to the relay).

**Deploy plan:** `check` (workspace, sudoers rule incl. `pdf-parser`) → `fetch` (download
the confirmed archive) → `extract` → `documents` → `migrations` → `prompts` → `configs`
(reviewer and app config) → `binary::<name>` per binary in the archive (verify, back up,
stop, swap, start, health check, rollback) → `pdf-parser` (image build, verify, restart,
verify running). `create-admin` and `deploy-runner` are installed without restarting a
service, and `deploy-runner-linux` is installed by rename because it is the running
deploy's own executable.

*Alternatives:* rewriting the steps in Go gives two copies of the deploy logic that drift
apart, and manual runs would stop matching button runs; parsing the scripts' existing
`==>` lines ties the page to prose.

### D4. The archive is named after the build number; the deploy installs the confirmed name

`mise run build-server-linux` captures the build machine's local time before binary
compilation. The manifest records `built_at` with the UTC offset and uses the same local
time for the build number (`YYYYmmddHHMMSS`).
`shell_server_build_rcp.sh` reads the number from the manifest and names the archive
`chenweb-deploy-<build_number>.tar.gz`. After upload it reports the archive name to the
runner, which records the name and number together in the build row.

Before Deploy, the page asks the box for the newest archive on the relay
(`shell_server_deploy.sh --newest`, so the relay address stays in the script) and shows its name, size, time and the build number
parsed from the name ("unknown" for older names). On confirmation the deployment row
stores that name, and the runner passes it to `shell_server_deploy.sh` as
`RELAY_ARCHIVE`, which then downloads exactly that file instead of the newest. A newer
archive landing in between is therefore never installed by surprise. The row also stores
`previous_build_number`, the build `chenweb` was running when the deploy was requested.

### D5. The deploy runs entirely as `gui`

`shell_server_deploy.sh` drops its root check, restarts `pdf-parser` with
`sudo -n /bin/systemctl restart pdf-parser`, and uses `rsync --chown` only when run as
root. It still works as root, so the old manual way remains available. The box runs its
own copy of the script, so the build ships `shell_server_deploy.sh` in the archive and
each deploy installs it (by rename, leaving the running copy untouched) for the next deploy. Its `check` step
fails fast with a clear message when the sudoers rule lacks `pdf-parser`. The relay
download now uses `gui`'s SSH key instead of root's.

One-time root setup on the box: install `chenweb-deploy.service`; add `pdf-parser` and
`chenweb-deploy` to `/etc/sudoers.d/gui-chenweb-services` in the same form as the
existing entries, which list exact commands (`/bin/systemctl restart pdf-parser`, …); check
with `visudo -c`; `systemctl daemon-reload`. Earlier root deploys left directories that `gui`
cannot replace (`ChenWeb/chenweb-deploy`, `ChenWeb/deploy_staging`, `~/Workspace/deploy`),
so the setup also gives them to `gui`.

### D6. Processes: fixed list, `systemctl` for state and restart, `buildinfo` for builds

The process list is a constant in `server/api/deployment/` mapping each process to its
systemd unit and its System Monitor log source:

| Process | Unit (box) | Log source |
|---|---|---|
| ChenWeb | `chenweb` | `chenweb` |
| Doc Processor | `doc-processor` | `doc-processor` |
| Doc Service | `doc-service` | `doc-service` |
| parser-result-converter | `parser-result-converter` | `parser-result-converter` |
| Kratos | `kratos` | `kratos` |
| NATS | `nats-server` | `nats` |
| pdf-parser | `pdf-parser` | `pdf-parser` |

On the box, state comes from `systemctl show -p ActiveState,SubState,MainPID,ActiveEnterTimestamp <unit>`
(no sudo needed). On the Mac, the list also includes Caddy and deepseek-watcher (they
have log sources there) and shows no unit state. On both, the running build number comes
from `buildinfo` for the Go executables.

Restart (box only) accepts only a process from this list, asks for confirmation and runs
`sudo -n /bin/systemctl restart <unit>`. It is refused while a deployment is queued or
running. For `chenweb` the handler answers first and issues the restart a moment later,
so the response reaches the browser. systemd stops `chenweb` (and with it the caller) and
starts it again; the page shows that ChenWeb is restarting and reloads itself once the API
answers with a new PID or after it has seen the server down. Each restart is logged with the user and unit.

### D7. Log windows reuse System Monitor with a `source` filter

Each process row embeds `<SystemMonitorView sourceId=…>`. Today every instance polls
`/api/v1/system-admin/system-monitor` once a second and receives all nine sources (up to
256 KB each), so seven embedded windows would move megabytes per second. The handler
gains an optional `source` query parameter returning only that source, and the view sends
it when `sourceId` is set. The full System Monitor page is unchanged.

### D8. Data model

Three tables in `kb` (goose migrations):

- `kb.deployment_builds`: `id BIGSERIAL PK`, `build_number TEXT UNIQUE`, `build_name`,
  `target_machine`, `os_info`, `description`, `status` (`running|succeeded|failed`),
  `archive_name`, `error_message`, `requested_by`, `runner_host`, `runner_pid`,
  `started_at`, `finished_at`.
- `kb.deployments`: `id BIGSERIAL PK`, `archive_name`, `build_number`,
  `previous_build_number`, `status` (`queued|running|succeeded|failed`), `error_message`,
  `requested_by`, `runner_pid`, `requested_at`, `started_at`, `finished_at`.
- `kb.deployment_steps`: `id BIGSERIAL PK`, `build_id` → `kb.deployment_builds`,
  `deployment_id` → `kb.deployments` (exactly one set, `CHECK (num_nonnulls(…) = 1)`,
  `ON DELETE CASCADE`), `seq`, `step_key`, `step_arg`, `status`
  (`pending|running|succeeded|failed|skipped`), `started_at`, `finished_at`, `output`,
  `output_truncated`; unique `(build_id, seq)` and `(deployment_id, seq)`.

At most one active run per machine is enforced by the database: a unique index on a
constant, restricted to active rows (`ON kb.deployments ((true)) WHERE status IN
('queued','running')`, and likewise for builds). The API turns the conflict into 409.

A run whose runner died without reporting (machine reboot, killed process) would block
every later run. When the API reads an active row it checks the runner: on the Mac,
whether `runner_pid` is alive on `runner_host`; on the box, whether `chenweb-deploy` is
active (a `queued` row gets 30 seconds to be claimed). A dead runner's row becomes
`failed` with "runner exited without reporting" and its running step `failed`.

Every ID is its row's own `BIGSERIAL` key; `build_number` keeps its timestamp format.
No ID from the ID manager is needed. `kb.build_activities` (one row per executable,
written by `mise build-server-linux`) is untouched; `build_number` links it to
`kb.deployment_builds`.

### D9. API

All under `/api/v1/system-admin/deployment`, admin or owner only (the System Monitor
check):

| Method and path | Purpose |
|---|---|
| `GET /status` | role, the active run with steps (or none), last 20 runs |
| `GET /builds/:id`, `GET /deployments/:id` | one run with its steps |
| `POST /builds` `{description}` | start a build (role `build`); 409 if one is active |
| `GET /next-archive` | newest relay archive (role `deploy`) |
| `POST /deployments` `{archive_name}` | start a deployment (role `deploy`); 409 if active |
| `GET /processes` | process list with state and build number |
| `POST /processes/:name/restart` | restart one unit (role `deploy`) |

The page polls `/status` every 2 seconds while a run is active and every 10 seconds
otherwise. Failed requests during a deploy show "server restarting" instead of an error.

### D10. Page

One page with three sections:

1. **Build** (Mac) or **Deploy** (box): the action button (Deploy first shows the archive
   to confirm), then the current or selected run as a vertical step list: status icon,
   translated label, duration, and expandable output. The failed step opens on its own and
   shows its output; the run's error is shown above the list.
2. **History:** recent builds or deployments (build number, status, requested by, start,
   duration); selecting one shows its steps in section 1.
3. **Processes:** one row per process with state, PID, uptime, build number and (box)
   Restart, and its embedded log window below it.

Menu: `sysadmin-system-deployment` under System, on the `development` page only, like
System Monitor. All text through Paraglide (`dep_*`, `nav_sysadmin_system_deployment`).

## Reused modules

- **Catalogue:** searched for step runners, job and progress status, process control and
  restart, log monitors and run IDs. The only match, `20261108-01` Shared ID Manager, is
  not needed (D8).
- **`web/src/lib/components/home3/system-monitor-view.svelte`** (uncatalogued): reused for
  the per-process log windows and extended with the `source` request (D7). This is its
  second embedding, so it is registered with `create-reusable-module`.
- **`server/api/systemmonitor`** (uncatalogued): extended with the `source` filter.
- **`server/buildinfo`** (uncatalogued): `CurrentProcess` and `ListProcesses` give each
  process's running build number; reused as is.
- **`benchmark-step-card.svelte`** was looked at: it is a benchmark card with a per-step
  Run button, not a status list, so it is not reused.
- **New:** `server/api/deployment/` and `server/cmd/deploy-runner/`. Nothing catalogued or
  in the code runs a script and records its steps; both are specific to deployment and are
  not registered as reusable.

## Risks / Trade-offs

- [A web admin can deploy whatever archive is on the relay] → Same trust as today's
  manual deploy; the page is admin only and the deploy runs as `gui`, not root.
- [Restarting `chenweb` through `sudo` from inside `chenweb` kills the caller] → This is
  the intended self-restart: systemd runs the restart job in PID 1 and completes it after
  the caller dies, and the page reloads when ChenWeb is back.
- [No cancel button; a hung step (e.g. a stalled `scp`) stays running] → The step and its
  output show where it hangs. Killing the runner (`kill <pid>` on the Mac,
  `sudo systemctl stop chenweb-deploy` on the box) makes the API mark the run failed.
- [Markers drift from the plans] → The Go test fails; at run time an unknown marker
  becomes an extra step instead of breaking the run.
- [`tar -v` and `docker build` print thousands of lines] → The per-step cap keeps the tail,
  where errors are.
- [A manual script run at the same time as a button run] → Not prevented, as today; the
  scripts print markers but do not write to the database when run by hand.
- [The runner's environment differs from an interactive shell] → On the Mac the runner
  starts the scripts in `ChenWeb/`, where mise loads the same environment as a manual
  run; the `check` step reports a missing `PG_*` or `PROJECT_WORKSPACE_DIR` by name. On
  the box the unit uses `chenweb`'s environment file.

## Migration Plan

1. Release this change once with the current manual procedure. Both machines' `chenweb`
   startups create the tables; the box receives `deploy-runner-linux`.
2. One-time root setup on the box (D5): install `chenweb-deploy.service`, extend the
   sudoers rule, `daemon-reload`. Authorize `gui`'s public key for `cding` on the relay.
   Set `[deployment] role = "deploy"` in the box's `config.local.toml` and restart
   `chenweb`.
3. On the Mac set `[deployment] role = "build"` in `config.local.toml`.
4. First button build and deploy, watched against the logs.

Rollback: remove the `role` key to hide the actions. The scripts still run by hand, as
`gui` or root. The tables are additive.

## Open Questions

- Whether `gui`'s key is already authorized on the relay (checked during the rollout).

Resolved during implementation: `PROJECT_WORKSPACE_DIR` is set in the box's `.env`, which
the unit loads; the sudoers rule lists exact commands, so the additions are
`/bin/systemctl {start,stop,restart,status} pdf-parser` and `/bin/systemctl start chenweb-deploy`.
