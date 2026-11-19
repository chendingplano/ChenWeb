## ADDED Requirements

### Requirement: Version format and per-project versions
A release version SHALL have three parts, major, minor and revision, each an integer from 0
to 99, and SHALL always be displayed and exchanged as `MM.mm.rr` with each part padded to two
digits (for example `01.02.03`). Versions SHALL be kept per project; the only accepted
project is `ChenWeb`. Changes to `shared` SHALL be recorded and released as part of a
ChenWeb release.

#### Scenario: Padded display
- **WHEN** a release has major 1, minor 2 and revision 3
- **THEN** the page, the API, the MANIFEST and the notes show `01.02.03`

#### Scenario: Unknown project
- **WHEN** a request names project `tax`
- **THEN** it is rejected with 400 and no row is written

### Requirement: Each machine stores its current release
Each machine SHALL store one current version per project. On the build machine it SHALL be
the latest version created, by a new major, a new minor or a committed build. On the deploy
machine it SHALL be the release that machine is running. Before anything is created the
current version SHALL be `00.00.00`.

#### Scenario: Fresh build machine
- **WHEN** no release exists for ChenWeb
- **THEN** the Releases page shows current `00.00.00` and next build `00.00.01`

### Requirement: Create a new major or minor
On the build machine an admin SHALL be able to create a new major or a new minor with a
description. A new major SHALL change the current version `M.m.r` to `(M+1).00.00`; a new
minor SHALL change it to `M.(m+1).00`. `00` SHALL mean that no minor or revision has been
created yet. The new major or minor SHALL be recorded with its description, date and
creator, and SHALL appear in the release history.

#### Scenario: New major
- **WHEN** the current version is `01.02.05` and an admin creates a new major with description "Knowledge graph rewrite"
- **THEN** the current version becomes `02.00.00`, the history shows `02.00.00` with that description, and the next build version is `02.00.01`

#### Scenario: New minor
- **WHEN** the current version is `01.02.05` and an admin creates a new minor
- **THEN** the current version becomes `01.03.00` and the next build version is `01.03.01`

#### Scenario: Part would pass 99
- **WHEN** the current major is 99 and an admin creates a new major
- **THEN** the request is refused with a message and nothing changes

#### Scenario: Build in progress
- **WHEN** a release is being built and an admin creates a new major or minor
- **THEN** the request is refused with 409 saying a build is running

### Requirement: Release items wait as pending until a build takes them
An admin on the build machine SHALL be able to add a release item with a type (`new
feature`, `improvement` or `bug fix`), a component (`ChenWeb` or `shared`), a non-empty
description, and optionally a GitHub issue number (digits only), a pull request and notes.
A new item SHALL be pending. Pending items SHALL be editable and deletable. Items that a
build has taken SHALL NOT be editable or deletable.

#### Scenario: Record a bug fix with an issue
- **WHEN** an admin adds a `bug fix` item for component `ChenWeb` with issue `142` and description "Cited table evidence scrolled out of view"
- **THEN** it is listed under Pending items with the issue shown as `#142`

#### Scenario: Invalid issue number
- **WHEN** an item is submitted with issue `ChenWeb-142`
- **THEN** it is refused with 400 naming the issue field

#### Scenario: Edit a released item
- **WHEN** an admin tries to edit or delete an item that belongs to a built release
- **THEN** the request is refused with 409 and the item is unchanged

#### Scenario: Item added during a build
- **WHEN** an item is added while a build is running
- **THEN** it stays pending and is not part of that build's release

### Requirement: Agents can add items from the command line
`release-tool add-item` SHALL add a pending item on the build machine with the same fields
and validation as the page, without a browser login. It SHALL refuse to run unless the
machine's `[deployment] role` is `build`.

#### Scenario: Agent records a fix
- **WHEN** `release-tool add-item --type "bug fix" --component shared --issue 26 "DeepSeek span repair corrupted quotes"` runs on the Mac
- **THEN** the item appears under Pending items on the Releases page

#### Scenario: Wrong machine
- **WHEN** `release-tool add-item` runs where role is `deploy`
- **THEN** it exits non-zero with a message and writes nothing

### Requirement: Only the build machine accepts release changes
Creating majors and minors and adding, editing or deleting items SHALL be accepted only
where `[deployment] role = "build"`. Elsewhere these requests SHALL be refused with 403
`read_only`, and the Releases page SHALL show the history, the current release and the notes
without editing controls. All release routes SHALL require an admin.

#### Scenario: Production page is read-only
- **WHEN** an admin opens Releases on the deploy machine
- **THEN** the page shows the running release and the history, and shows no New major, New minor or Add item controls

#### Scenario: Non-admin
- **WHEN** a logged-in user without the admin role calls any release route
- **THEN** the response is 403

### Requirement: Releases page layout on the build machine
On the build machine the Releases page SHALL show the current version and the next build
version, New major and New minor actions (each asking for a description and showing the
resulting version before confirming), the pending items with add, edit and delete, a
release that is being built with its items and age, and the history newest first. Each
built release SHALL expand to its items and offer its notes as text and as PDF. All page
text SHALL be in English and Chinese.

#### Scenario: Confirm a new minor
- **WHEN** an admin clicks New minor while current is `01.02.05`
- **THEN** a dialog asks for a description and shows "New version: 01.03.00" before the admin confirms

#### Scenario: Few revisions left
- **WHEN** the current revision is 90 or higher
- **THEN** the page shows how many builds remain before a new minor is needed
