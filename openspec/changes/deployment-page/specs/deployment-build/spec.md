## ADDED Requirements

### Requirement: Deployment page and machine role
The system SHALL provide a Deployment page under Development → System Admin → System,
visible only on the `development` page and only to admins and owners. The page's actions
SHALL depend on `[deployment] role` in the machine's `config.local.toml`: `build` offers
Build Deployment, `deploy` offers Deploy, and an unset or unknown value offers neither and
names the key to set. All page text SHALL be available in English and Chinese.

#### Scenario: Build machine
- **WHEN** an admin opens the Deployment page on a machine with `role = "build"`
- **THEN** the page shows the Build Deployment button and the build history, and no Deploy button

#### Scenario: Role not configured
- **WHEN** an admin opens the page on a machine without `[deployment] role`
- **THEN** the page shows neither button and tells the admin to set `[deployment] role` in `config.local.toml`

#### Scenario: Non-admin
- **WHEN** a user who is not an admin or owner calls any deployment API
- **THEN** the API answers 403 and starts nothing

### Requirement: Build Deployment runs the build script as a separate process
Clicking Build Deployment SHALL insert a `kb.deployment_builds` row with status `running`
and start `deploy-runner build` detached from the web server, which runs
`shell_server_build_rcp.sh`. The build SHALL continue and keep reporting when the web
server restarts during the build. Only one build SHALL be active at a time.

#### Scenario: Start a build
- **WHEN** an admin clicks Build Deployment with an optional description
- **THEN** a build row is created with the requester and description, and the API answers 202 with the build id

#### Scenario: Build already running
- **WHEN** an admin starts a build while another build is `running`
- **THEN** the API answers 409 and no new row is created

#### Scenario: Web server restarts during the build
- **WHEN** `air` restarts the web server while the build is running
- **THEN** the build keeps running, and after the restart the page shows its current steps again

### Requirement: Build steps and their status are shown while the build runs
The page SHALL show every planned build step from the start, in order: check, web
frontend, embedded prompt defaults, each binary (server, doc-processor, parser-result-converter, doc-service,
create-admin, deploy-runner), stage, documents, pdf-parser sources, archive, upload. Each
step SHALL show its status (`pending`, `running`, `succeeded`, `failed`, `skipped`), its
duration and its output, updated while the build runs.

#### Scenario: Progress
- **WHEN** the build script starts the step for `doc-processor`
- **THEN** the page shows the earlier steps as succeeded, `doc-processor` as running with its output, and the later steps as pending

#### Scenario: Successful build
- **WHEN** the script exits with status 0
- **THEN** every step that ran is succeeded, the build is `succeeded`, and the row records the archive name and finish time

### Requirement: A failing build step stops the build
When a build step fails, the build SHALL stop at once. That step SHALL be marked `failed`
with its output shown, later steps SHALL be marked `skipped`, and the build row SHALL be
marked `failed` with an error message.

#### Scenario: Go compile error
- **WHEN** the `server` binary fails to compile
- **THEN** the `server` binary step is failed and opened with the compiler output, the remaining steps are skipped, nothing is uploaded, and the build is `failed`

#### Scenario: Runner died
- **WHEN** the runner process of a `running` build no longer exists
- **THEN** the next status request marks the build `failed` with "runner exited without reporting", and a new build can start

### Requirement: The archive carries the build number
The build SHALL derive its number in UTC `YYYYmmddHHMMSS` format from the manifest's
`built_at` timestamp, captured immediately before binary compilation. The build SHALL
embed the number in the binaries that report build info and use it to name the archive
`runshen-deploy-<build_number>.tar.gz`. After upload, the runner SHALL record the archive
name and build number in the build row.

#### Scenario: Uploaded archive
- **WHEN** build `20261116093000` succeeds
- **THEN** `runshen-deploy-20261116093000.tar.gz` is on the relay and the build row's `archive_name` names it

### Requirement: Scripts stay usable by hand
`shell_server_build_rcp.sh` SHALL keep its arguments and behaviour when run from a shell.
The step markers it prints SHALL be its only addition to the output.

#### Scenario: Manual build
- **WHEN** a developer runs `shell_server_build_rcp.sh` from a terminal
- **THEN** it builds and uploads the archive as before, prints `::step::` lines between the usual output, and writes no deployment rows
