## Why

Deploying ChenWeb is two manual shell sessions. On the Mac, `shell_server_build_rcp.sh`
builds the binaries, packs a tar file and copies it to the relay (192.168.29.96). On the
China box, `shell_server_deploy.sh` downloads the newest tar file, installs it and
restarts the services, and it must be run as root. Progress is only visible as scrolling
terminal output, failures are easy to miss, and nothing records which build was made or
deployed, or when. Restarting a single service also needs a root shell on the box.

## What Changes

- New page **Development → System Admin → System → Deployment** (admin only, like System
  Monitor). What it offers depends on the machine it runs on, set by a new
  `[deployment] role` key in the machine's `config.local.toml` (`build` on the Mac,
  `deploy` on the box).
- **Build Deployment button** (build machine): runs the existing build script as a
  separate process, shows each step and its status while it runs, stops at the first
  failing step, and records the build in a new table.
- **Deploy button** (deployment machine): shows the newest tar file on the relay and its
  build number, then on confirmation runs the existing deploy script in a separate systemd
  unit, shows each step and its status, aborts at the first failing step, and records the
  deployment in a new table. The deploy keeps running and reporting while chenweb itself
  is restarted.
- **Process list** with each service's state, its running build number and its log
  monitor window (the System Monitor window, embedded). On the deployment machine each
  service has a **Restart** button. The Mac lists its processes without Restart, because
  nothing supervises them there.
- Build and deploy scripts emit machine-readable step markers. They keep working when run
  by hand.
- `shell_server_deploy.sh` no longer requires root. Every step runs as `gui`, using the
  existing passwordless sudoers rule for `systemctl`. A one-time root setup adds
  `pdf-parser` and the new `chenweb-deploy` unit to that rule and installs the unit.
- The build names its tar file after the build number, so the box can show which build it
  will deploy without opening the archive.
- The System Monitor log endpoint accepts a `source` filter, so an embedded window fetches
  only its own log.

## Capabilities

### New Capabilities

- `deployment-build`: building a deployment archive from the page on the build machine,
  with step-by-step status and a build record.
- `deployment-deploy`: deploying the newest relay archive from the page on the deployment
  machine, with step-by-step status, abort on failure and a deployment record.
- `deployment-processes`: listing the services with state, build number and log monitor,
  and restarting a service on the deployment machine.

### Modified Capabilities

None. System Monitor has no spec; its `source` filter is an internal extension.

## Impact

- **Backend (ChenWeb):** new package `server/api/deployment/` (handlers, step plans,
  process list); new command `server/cmd/deploy-runner/` (runs a script, records steps);
  `server/api/systemmonitor/handler.go` gains the `source` filter; routes in
  `server/api/routes.go`.
- **Database:** new tables `kb.deployment_builds`, `kb.deployments`,
  `kb.deployment_steps` (goose migrations in `project_migrations/`). The existing
  `kb.build_activities` (one row per built executable, written by
  `mise build-server-linux`) is unchanged; `build_number` links the two.
- **Frontend:** new `deployment-view.svelte`, a menu entry in `nav-rail.svelte`, routing
  in `content-panel.svelte`, `dep_*` and `nav_*` keys in `en.json` and `zh-cn.json`;
  `system-monitor-view.svelte` requests only its own source when embedded.
- **Scripts:** step markers in `shell_server_build_rcp.sh`, `mise build-server-linux`,
  `shell_server_deploy.sh` and `scripts/deploy-server-china.sh`; the build exports
  `BUILD_NUMBER` and names the archive `runshen-deploy-<build_number>.tar.gz`; the build
  ships `deploy-runner-linux`.
- **China box (one-time root setup):** install `chenweb-deploy.service` (User=gui); add
  `pdf-parser` and `chenweb-deploy` to `/etc/sudoers.d/gui-chenweb-services`; authorize
  `gui`'s SSH key on the relay (the deploy now downloads as `gui`, not root); set
  `[deployment] role = "deploy"` in the box's `config.local.toml`.
- **Docs:** the production runbook
  (`KnowledgeStore/doc-repo/devdocs/202609/2026090701-devdoc-deploy-start-system-onto.md`)
  gets the one-time setup and the button workflow; a new devdoc describes the feature.
- **Reusable modules:** searched the catalogue for step runners, job status, process
  control, log monitors and run IDs; the only match is `20261108-01` Shared ID Manager,
  which this change does not need (every new ID is its table's own `BIGSERIAL` primary
  key; `build_number` stays the existing UTC timestamp string). Reused uncatalogued code:
  `system-monitor-view.svelte` (embedded log window, second use after the Doc Processor
  dashboard, so it is registered as a module) and `server/buildinfo` (running build
  number per process). Nothing generic exists for running scripts step by step, so the
  runner is new and specific to deployment.
