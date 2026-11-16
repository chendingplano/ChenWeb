## ADDED Requirements

### Requirement: Deploy shows the archive before it starts
On a machine with `role = "deploy"`, the Deploy button SHALL first show the newest
`runshen-*.tar.gz` on the relay: its name, size, time and build number (parsed from the
name, "unknown" for older names). The deployment SHALL start only after the admin
confirms, and SHALL install exactly the confirmed archive.

#### Scenario: Confirm the newest archive
- **WHEN** an admin clicks Deploy
- **THEN** the page shows `runshen-deploy-20261116093000.tar.gz`, its size and time, and build number `20261116093000`, and asks for confirmation

#### Scenario: Newer archive arrives before confirmation
- **WHEN** a newer archive reaches the relay after the admin saw the confirmation but before the deploy downloads
- **THEN** the deploy downloads and installs the archive the admin confirmed, not the newer one

#### Scenario: Relay unreachable
- **WHEN** the box cannot list the relay's archives
- **THEN** the page shows the error and no deployment is started

### Requirement: Deploy runs outside the web server and survives its restart
Confirming SHALL insert a `kb.deployments` row with status `queued`, recording the archive
name, its build number, the build the web server is running and the requester, and start
the `chenweb-deploy` systemd unit, which claims the row and runs `shell_server_deploy.sh`
as `gui`. The deployment SHALL keep running and recording its steps while `chenweb` is
restarted, and the page SHALL resume showing them when `chenweb` answers again. Only one
deployment SHALL be queued or running at a time.

#### Scenario: chenweb restarts mid-deploy
- **WHEN** the deploy reaches the `server` binary step and restarts `chenweb`
- **THEN** the page shows that the server is restarting, the deployment continues, and the page shows the following steps once `chenweb` is back

#### Scenario: Deployment already active
- **WHEN** an admin confirms a deployment while another is queued or running
- **THEN** the API answers 409 and no new row is created

#### Scenario: Unit cannot be started
- **WHEN** `sudo -n systemctl start chenweb-deploy` fails
- **THEN** the deployment row is marked `failed` with the command's error, and the page shows it

### Requirement: Deploy steps and their status are shown while it runs
The page SHALL show every planned deploy step from the start, in order: check, fetch,
extract, documents, migrations, prompts, configs, each binary in the archive, pdf-parser.
Each step SHALL show its status, duration and output, updated while the deploy runs.

#### Scenario: Binary step
- **WHEN** the deploy installs `doc-processor`
- **THEN** its step is running and its output shows the checksum check, the backup, the service stop and start, and the health check

#### Scenario: Binary not in the archive
- **WHEN** the archive holds no `doc-service-linux` and the deploy succeeds
- **THEN** the `doc-service` step is shown as skipped

### Requirement: A failing deploy step aborts the deployment
When a deploy step fails, the deployment SHALL stop at once. The step SHALL be marked
`failed` and flagged with its output, later steps SHALL be `skipped`, and the deployment
row SHALL be `failed` with an error message. The existing per-binary rollback of
`deploy-server-china.sh` SHALL still happen before the deployment stops.

#### Scenario: Health check fails
- **WHEN** `doc-processor` does not become active after the swap
- **THEN** the script restores the previous binary, the `doc-processor` step is failed with the journal output, later steps (including pdf-parser) are skipped, and the deployment is `failed`

#### Scenario: Missing sudoers entry
- **WHEN** `gui` may not run `systemctl` for `pdf-parser`
- **THEN** the `check` step fails before anything is changed, naming the sudoers file to fix

#### Scenario: Runner died
- **WHEN** a deployment is `running` but `chenweb-deploy` is no longer active, or is `queued` for more than 30 seconds without being claimed
- **THEN** the next status request marks it `failed` with "runner exited without reporting"

### Requirement: Deploy runs without root
`shell_server_deploy.sh` SHALL run as `gui`, using only the passwordless sudoers rule for
`systemctl` and the `docker` group. It SHALL still work when run as root by hand.

#### Scenario: Run as gui
- **WHEN** the `chenweb-deploy` unit runs the deploy as `gui`
- **THEN** every step, including the pdf-parser image build and restart, completes without root

#### Scenario: Manual run
- **WHEN** an operator runs `shell_server_deploy.sh` by hand as `gui` or root
- **THEN** it deploys the newest relay archive as before and prints `::step::` lines, and writes no deployment rows
