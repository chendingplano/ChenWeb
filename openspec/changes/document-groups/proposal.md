## Why

The Document Builder (modules 20261109-02/-03) publishes one directory as one menu. Administrators need to build menus from documents spread over several directories and individual files, share a file between menus, manage them from a page instead of editing TOML and code, and keep identity, categories and (later) comments when files are renamed or moved.

## What Changes

- Add **document groups**: named collections whose members are directories (recursive) and individual files under one Document Root Directory (`DOCUMENT_ROOT_DIR`). A file may be in several groups.
- Register every document in the database with a stable ID from the shared ID manager, mapped to its path(s); detect renames and moves by content hash.
- Treat a document as **logical**: it always means its highest `-v<number>` version, for every group.
- Store categories per group (LLM classification, still once per document per group).
- Give each group one menu location (parent menu, localized label, position number) and access roles; the Resources page builds its Documents menu from groups instead of hard-coded entries.
- Remove from a group by excluding the file; optionally delete it from disk (refused while another group holds it) with the file moved to an archive.
- Export Markdown as PDF on demand, cached by source hash, deleted when the source changes.
- Admin page for groups, members, classification and menu settings.
- **Replaces** the TOML sources, per-directory manifests, `docbuilder-classify` and the `/docbuilder/:source/*` API. Existing categories are imported from the old manifests.
- Phase 2 (tracked here, not implemented in the first pass): comments, tickets and a to-do list on documents; upload into member directories. Online editing and file types other than Markdown/PDF are out of scope.

## Capabilities

### New Capabilities

- `document-groups`: groups, membership, document registry, per-group categories, menu binding, access, deletion with archive, PDF export.

### Modified Capabilities

None (no existing spec covers the Document Builder).

## Impact

ChenWeb server (`server/api/docbuilder`, routes), a goose project migration (`kb.doc_*` tables, seed groups), the Resources navigation and content panel, a new admin view, English/Chinese messages, `mise build-docs`, the Document Builder module docs and catalogue rows, the user manual.

**Reused modules** (searched with find-reusable-module: "document", "menu tree", "markdown", "id"): Markdown Viewer `20261109-01` (rendering Markdown); Document Builder backend `20261109-02` and frontend `20261109-03` (scan, title, version parsing, LLM classifier, tree, view are extended, not rewritten); shared ID manager `20261108-*` ("Always use" list) for document IDs, sequence `doc_registry_id` provisioned by this change; page-config access model (`pageconfighandler`: admin check, role validation) reused for group roles. Nothing else in the catalogue covers file registries, groups or PDF export.
