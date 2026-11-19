## ADDED Requirements

### Requirement: The deploy machine mirrors release history
The deploy machine SHALL import release history from release files. An import SHALL add
releases it does not have and SHALL never change or delete a release it already has, so
imports are safe to repeat and to apply in any order. The `chenweb` server SHALL import the
installed `release/release.json` when it starts on a machine with `role = "deploy"`, and the
deploy script SHALL install `release/` from the archive.

#### Scenario: Import after deploy
- **WHEN** the server restarts after deploying build `01.02.06`
- **THEN** the Releases page on the deploy machine lists every release up to `01.02.06` with its items

#### Scenario: Older file imported later
- **WHEN** a release file for `01.02.04` is imported after one for `01.02.06`
- **THEN** releases `01.02.05` and `01.02.06` are still present and unchanged

### Requirement: The deploy machine records the release it runs
When the server starts on the deploy machine it SHALL set its current release to the version
and build number in the installed `release/release.json`. A machine running a build from
before this change SHALL show no current release.

#### Scenario: Skipped releases
- **WHEN** the box ran `01.02.03` and build `01.02.06` is deployed, skipping `01.02.04` and `01.02.05`
- **THEN** the box's current release is `01.02.06`

### Requirement: The admin chooses which release to deploy
On a machine with `role = "deploy"` the Deploy button SHALL list every `runshen-*.tar.gz`
archive on the relay, newest first, with its build number, release version (or "no release
information" for archives without a release file), date, size and item count, and SHALL mark
the running release. The newest archive SHALL be selected by default. The deployment SHALL
install exactly the archive the admin confirmed. Listing SHALL read the newest release file
from the relay and SHALL NOT download archives.

#### Scenario: Deploy an older build
- **WHEN** the relay holds `01.02.06` and `01.02.07` and the admin selects `01.02.06` and confirms
- **THEN** the deployment installs the `01.02.06` archive and the history row shows `01.02.06`

#### Scenario: Relay unreachable
- **WHEN** the box cannot list the relay
- **THEN** the page shows the error and no deployment can be started

### Requirement: Notes since the running release
For the selected archive the page SHALL show the notes for every release after the running
release up to the selected one, so items from skipped releases are included, and SHALL offer
the same range as PDF. When the selected release is older than the running release, the page
SHALL warn that it is a rollback and list the releases that would be removed.

#### Scenario: Skipping releases
- **WHEN** the box runs `01.02.03` and the admin selects `01.02.06`
- **THEN** the page lists the items of `01.02.04`, `01.02.05` and `01.02.06`

#### Scenario: Rollback
- **WHEN** the box runs `01.02.06` and the admin selects `01.02.04`
- **THEN** the page warns of a rollback and lists `01.02.05` and `01.02.06` as the releases being removed

#### Scenario: Running build predates releases
- **WHEN** the box has no current release and the admin selects `01.02.06`
- **THEN** the page lists all releases up to `01.02.06`

### Requirement: Deployments record release versions
Each deployment row SHALL record the selected release version and the release that was
running when it was queued, and the deployment history SHALL show the release version next
to the build number.

#### Scenario: History row
- **WHEN** `01.02.06` is deployed over `01.02.03`
- **THEN** the history row shows release `01.02.06` and replaces `01.02.03`
