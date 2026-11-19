## ADDED Requirements

### Requirement: A successful build is a new revision
Each successful build SHALL create the release `M.m.(r+1)` from the build machine's current
version `M.m.r` and SHALL make it the current version. A build SHALL be successful only when
its archive and release file are on the relay and the release is committed. This SHALL hold
for builds started from the Deployment page and for `shell_server_build_rcp.sh` run by hand.

#### Scenario: Build after two earlier builds
- **WHEN** the current version is `01.02.05` and a build succeeds
- **THEN** release `01.02.06` exists with status released, today's date and the build number, and the current version is `01.02.06`

#### Scenario: Hand build
- **WHEN** a developer runs `shell_server_build_rcp.sh` from a terminal and it succeeds
- **THEN** a new revision is created exactly as for a page build, and no `kb.deployment_builds` row is written

### Requirement: The build takes all pending items when it starts
At the start of a build the system SHALL reserve the next revision with status `building`
and bind every pending item to it in one transaction. A build SHALL be allowed with no
pending items. The build SHALL be refused before compiling when the revision would pass 99,
with a message to create a new minor.

#### Scenario: Items bound at start
- **WHEN** three items are pending and a build starts
- **THEN** the three items show as part of the building release and are no longer editable

#### Scenario: Revision limit
- **WHEN** the current version is `01.02.99` and a build starts
- **THEN** the build fails at the check step with "create a new minor" and nothing is reserved

### Requirement: The version is inside the build
The reserved version SHALL be embedded in the build metadata of the binaries that carry
build metadata, shown on the Build Info page, and written to MANIFEST as
`release_version:`. MANIFEST SHALL also record the shared library's git revision and whether
its working copy was dirty.

#### Scenario: Build Info after deploy
- **WHEN** build `01.02.06` is running on a machine
- **THEN** the Build Info page shows release `01.02.06` next to the build number

#### Scenario: Build without the release flow
- **WHEN** `mise run build-server-linux` is run alone
- **THEN** it still builds, and the metadata and MANIFEST show no release version

### Requirement: The archive carries the release notes and history
Before the archive is made the build SHALL write a `release/` directory into the staged
build containing `release.json` (this release, its build number, git revisions and the full
release history with items) and the notes for this release as plain text and PDF in English
and Chinese. The upload SHALL also place a copy of `release.json` on the relay as
`runshen-deploy-<build_number>.release.json`.

#### Scenario: Files in the archive
- **WHEN** build `20261119063000` succeeds as `01.02.06`
- **THEN** the archive contains `release/release.json`, `release/RELEASE_NOTES.txt`, `release/RELEASE_NOTES.pdf` and their `zh-cn` versions, and the relay holds `runshen-deploy-20261119063000.release.json`

### Requirement: A failed build gives the revision back
If a build fails or is abandoned at any step, the reserved release SHALL be removed and its
items SHALL return to pending, so the next build reuses the same version number. If the
failure happens after the upload, the uploaded archive and release file SHALL be removed
from the relay first. A `building` release left behind by a killed build SHALL be reverted
by the next build before it reserves.

#### Scenario: Compile error
- **WHEN** a build reserved `01.02.06` and fails while compiling
- **THEN** no release `01.02.06` exists, its items are pending again, and the next build reserves `01.02.06`

#### Scenario: Killed build
- **WHEN** the build process was killed and left `01.02.06` as building
- **THEN** the next build reverts it and reserves `01.02.06` again

### Requirement: The build page shows the release
On the build machine the Deployment page SHALL show the version the next build will get and
the number of pending items before a build starts, and the release version of each build in
the build history.

#### Scenario: Before building
- **WHEN** an admin opens the Deployment page on the Mac with current `01.02.05` and four pending items
- **THEN** the build card shows "Next release 01.02.06 · 4 items"
