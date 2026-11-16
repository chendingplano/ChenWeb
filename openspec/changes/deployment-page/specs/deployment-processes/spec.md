## ADDED Requirements

### Requirement: Process list
The Deployment page SHALL list the processes: ChenWeb, Doc Processor, Doc Service,
parser-result-converter, Kratos, NATS and pdf-parser, plus Caddy and deepseek-watcher on
the build machine. On the deployment machine each row SHALL show the systemd unit's
state, PID and time since it started. On both machines the Go executables SHALL show the
build number they are running, or "unreported" when they do not answer.

#### Scenario: Deployment machine
- **WHEN** an admin opens the page on the box
- **THEN** each of the seven processes shows its unit state (for example `active (running)`), PID, uptime and, for the Go executables, its build number

#### Scenario: Process stopped
- **WHEN** `doc-service` is not running on the box
- **THEN** its row shows the unit's inactive or failed state

### Requirement: Log window under each process
Each process row SHALL show that process's log window, the System Monitor window for its
log source, with the same search, pause, filter and size controls. An embedded window
SHALL request only its own log source.

#### Scenario: Embedded log
- **WHEN** the page shows the Doc Processor row
- **THEN** the Doc Processor log window below it updates every second, and its requests ask for the `doc-processor` source only

#### Scenario: Log file missing
- **WHEN** a process has no log file on this machine
- **THEN** its window shows the System Monitor "missing" message

### Requirement: Restart a process on the deployment machine
On the deployment machine each process row SHALL have a Restart button. After the admin
confirms, the system SHALL run `sudo -n /bin/systemctl restart <unit>` for that process's
unit and refresh its state. Only units in the process list SHALL be accepted. The build
machine SHALL show no Restart button and its API SHALL refuse restarts. Every restart
SHALL be logged with the user and the unit.

#### Scenario: Restart doc-processor
- **WHEN** an admin clicks Restart on Doc Processor and confirms
- **THEN** the unit restarts and the row shows the new PID and a fresh uptime

#### Scenario: Restart chenweb
- **WHEN** an admin restarts ChenWeb
- **THEN** the API answers before the restart happens, the page shows that ChenWeb is restarting, and the page reloads itself once `chenweb` answers again

#### Scenario: Unknown process
- **WHEN** a restart request names a process that is not in the list
- **THEN** the API answers 400 and runs no command

#### Scenario: Restart during a deployment
- **WHEN** an admin tries to restart a process while a deployment is queued or running
- **THEN** the API answers 409 and the page explains that a deployment is in progress

#### Scenario: Restart fails
- **WHEN** `systemctl restart` exits with an error
- **THEN** the page shows the error and the unit's current state
