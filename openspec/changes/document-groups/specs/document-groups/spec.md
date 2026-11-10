## ADDED Requirements

### Requirement: Document Root Directory

The system SHALL take the Document Root Directory from `DOCUMENT_ROOT_DIR` and SHALL store and accept document paths only as slash-separated paths relative to it. A path containing `..`, an absolute path, or a path outside the root SHALL be rejected. Hidden files and directories SHALL never be registered or served.

#### Scenario: Path escape refused
- **WHEN** an administrator adds a member path `../../etc`
- **THEN** the request fails and nothing is stored

### Requirement: Document registry with stable IDs

Every `.md` and `.pdf` file found under a group's members SHALL belong to a registered document whose `doc_id` is allocated by the shared ID manager. A file that disappears and a file that appears in the same sync with the same content hash SHALL be treated as a move: the document keeps its `doc_id`, memberships, categories and exclusions.

#### Scenario: Rename keeps category
- **WHEN** a classified file is renamed without editing and the group is rescanned
- **THEN** it keeps its `doc_id` and category and no classification call is made

#### Scenario: Rename with edit
- **WHEN** a file is renamed and edited before the rescan
- **THEN** a new document is registered, uncategorized, and the old one is hidden as missing

### Requirement: Logical documents show the latest version

Files whose paths differ only by a `-v<number>` version SHALL be versions of one document, kept separate per language suffix. Every group SHALL show and serve the highest version, comparing numeric components (`1.10` > `1.9`). Adding a newer version file SHALL change what all groups show without any group edit.

#### Scenario: Newer version appears
- **WHEN** `guide-v1.3-en.md` is added beside `guide-v1.2-en.md`
- **THEN** every group containing the document lists and serves v1.3

### Requirement: Groups, members and exclusions

A group SHALL consist of members that are directories (recursive) or files, and SHALL contain the documents of those members except those excluded from the group. A document MAY be in several groups. A file member SHALL cover the versions of that document in the same directory.

#### Scenario: File in two groups
- **WHEN** a directory is a member of group A and one of its files is a member of group B
- **THEN** the document appears in both groups

#### Scenario: Remove from a directory member
- **WHEN** an administrator removes a file that sits inside a member directory
- **THEN** the file is excluded from that group only and stays on disk and in other groups

### Requirement: Deletion with archive

Removing a document from a group MAY also delete it from disk. The deletion SHALL be refused, naming the other groups, while any other group contains the document. When allowed, every present version file SHALL be moved to the archive directory, not unlinked, and the document and registry rows SHALL be kept.

#### Scenario: Shared file cannot be deleted
- **WHEN** a document in groups A and B is removed from A with delete requested
- **THEN** the request is refused and the file and both memberships remain

#### Scenario: Delete archives
- **WHEN** a document that is in one group is removed with delete requested
- **THEN** its files are in the archive and no longer in the document tree

### Requirement: Per-group categories

Categories SHALL be stored per group and document. Classification SHALL send only documents without a category in that group and SHALL save progress after each batch. Re-classifying everything SHALL require an explicit force.

#### Scenario: Same document, two categories
- **WHEN** a document is in two groups classified separately
- **THEN** each group may hold a different category for it

### Requirement: Menu location, order and access

Each group SHALL have one parent menu, a localized label, a position number and access roles. The Documents menu SHALL list the enabled groups the user may see under their parent, ordered by position then key. Users without a matching role SHALL neither see the group nor read its documents.

#### Scenario: Order by position
- **WHEN** "Dev Documents" has position 10 and "User's Manuals" 20 under the same parent
- **THEN** Dev Documents is listed first, and swapping the numbers swaps them

#### Scenario: Role required
- **WHEN** a user lacks every role of a group and requests its tree or a file
- **THEN** the response is forbidden or not found and no content is returned

### Requirement: PDF export of Markdown

Exporting a Markdown document as PDF SHALL convert the current version only when no export exists for its current content, and SHALL reuse the stored export otherwise. When the source file's content changes, its stored export SHALL be deleted. A PDF document SHALL be returned unchanged. Other types SHALL be refused.

#### Scenario: Cached export
- **WHEN** the same unchanged Markdown document is exported twice
- **THEN** the converter runs once

#### Scenario: Source edited
- **WHEN** the Markdown file changes and the group is rescanned
- **THEN** its stored PDF is deleted and the next export converts again

### Requirement: Group overview page

Development › System Admin › Resources › Documents SHALL list every group as a card showing its label, key, members (flagging those not found on disk), document and uncategorised counts, and whether its page exists. A group's page is its entry at its menu location; it SHALL exist exactly when the group is enabled.

- **Sync** SHALL rescan the group and then classify its documents that have no category, as `mise build-docs <group>` does. When no classification model is available, the rescan SHALL still be kept and the card SHALL say that `DOCBUILDER_MODEL` must be set.
- **Build Page** SHALL create the group's page (enable the group). It SHALL be disabled while the page exists.
- **New group** SHALL take a key, English and Chinese labels, a position (default: after the last group under Documents), roles and at least one member. The group SHALL be created without a page under Documents. When a member is refused, the group SHALL NOT be kept and the reason SHALL be shown.

#### Scenario: Build Page once
- **WHEN** an administrator presses Build Page on a group without a page
- **THEN** the group appears in Resources › Documents for its roles, and the card's Build Page button is disabled

#### Scenario: Sync without a model
- **WHEN** Sync is pressed and the server has no classification model
- **THEN** new files are registered, nothing is classified, and the card asks for `DOCBUILDER_MODEL`

#### Scenario: Refused member
- **WHEN** a new group names a path that does not exist under the Document Root Directory
- **THEN** no group is left behind and the form shows why the path was refused

### Requirement: Comments, tickets, to-dos and upload (phase 2)

Groups SHALL later support comments, tickets and to-dos attached to a document (not to a group) and uploads into member directories. These are specified when implemented.

#### Scenario: Deferred
- **WHEN** the first pass is verified
- **THEN** these remain unchecked tasks in `tasks.md`
