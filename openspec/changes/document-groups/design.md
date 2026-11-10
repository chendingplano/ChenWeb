## Context

Today a source is one directory in `docbuilder.local.toml`; categories live in `<dir>/.docbuilder/manifest.json`; files are addressed by relative path. That cannot express multi-directory groups, shared files, per-group categories or renames.

## Is a document registry worth it?

Yes. Everything the groups need keys on "this document" independent of its path: category per (group, document), exclusions, the PDF cache, and later comments, tickets and to-dos. With paths as keys a rename silently drops all of them. The cost is one registry table set, a sync step on scan, and a move heuristic.

Limits: a file that is **renamed and edited at once** is seen as a new document (the hash differs), and one **moved outside the Document Root** is lost. Both are rare and visible (the old document goes "missing", the new one is Uncategorized). Moves done through the app can later keep identity explicitly.

Sizing: ~100 documents now, thousands at most. Sync walks only member directories and re-hashes only files whose size or mtime changed.

## Decisions

**D1 Root.** `DOCUMENT_ROOT_DIR` (absolute) is required. All stored paths are slash-separated, relative to it, never containing `..`. Files are served only from registry rows.

**D2 Registry.** `kb.doc_documents(doc_id, logical_key, kind, title, created_at)` is the logical document; `doc_id` comes from the ID manager, sequence `doc_registry_id`. `kb.doc_files(file_id BIGSERIAL, doc_id, path UNIQUE, version, hash, size, mtime, missing)` is one physical file. `logical_key` = path with the `-v<number>` removed, keeping the language suffix (`dir/name-.md` style key built as `prefix + NUL + suffix`), so `-en` and `-zh` are separate documents.

**D3 Sync** (per group, on tree request and on explicit rescan): walk the members; for every `.md`/`.pdf` file: known path → refresh hash if size/mtime changed (and delete its PDF); unknown path whose hash equals a `missing` file → **moved**: update path and logical key, keep `doc_id`; unknown path whose logical key matches a document → new version of that document; else a new document. Files no longer found are marked `missing`; a document with no present file is hidden. Hidden files and directories (including the archive and PDF cache) are never scanned.

**D4 Latest.** A document's current file is the present file with the highest version (numeric, per component); unversioned = the only file. Titles and content come from it.

**D5 Groups.** `kb.doc_groups(group_id, group_key UNIQUE, label JSONB, parent_menu_id, sort_order, access_role JSONB, enabled)`; `kb.doc_group_members(member_id, group_id, member_type dir|file, path)`; `kb.doc_group_exclusions(group_id, doc_id)`; `kb.doc_group_categories(group_id, doc_id, category, model, classified_at)`. A group's documents = documents of its members, minus exclusions. A *file* member matches the documents with the same logical key in its directory (so new versions follow); a *dir* member is recursive. Adding a member that is excluded removes the exclusion.

**D6 Classification.** Only documents without a category in that group are sent (existing `docbuilder.LLMClassifier`, batches of 25, saved per batch). `--force` clears the group's categories. On first sync of a group, categories from a legacy `.docbuilder/manifest.json` found in its member directories are imported, so nothing is paid for twice.

**D7 Menu.** `GET /doc-groups/menu` returns enabled groups the user may see, each `{group_key, label (locale resolved), parent_menu_id, sort_order}`, ordered by `(parent_menu_id, sort_order, group_key)`. The nav rail adds them as children of `parent_menu_id` (`documents` for Resources); the content panel renders `DocumentBuilderView` for ids `docgroup-<group_key>`. The hard-coded `docs-users-manual` and `docs-development` entries are replaced by seeded groups "Dev Documents" (10) and "User's Manuals" (20). One group has exactly one location.

**D8 Access.** Same model as page-config: roles in `access_role` (validated against configured roles, case-insensitive, empty = nobody), admins manage groups. Tree and file endpoints enforce the group's roles; a document is reachable only through a group the user can see.

**D9 Removal and deletion.** Remove = insert exclusion (or delete the member row if the file is itself a file member). `delete=true` additionally deletes from disk **only if** no other group contains the document (via members, minus exclusions); otherwise the request is refused with the groups named and only the membership changes if `delete` is dropped. Deleting moves **all present versions** of the document to `DOCUMENT_ARCHIVE_DIR` (default `<root>/.doc-archive/<UTC timestamp>/<relative path>`), marks them missing, and keeps the registry rows (so comments are not orphaned).

**D10 PDF.** Markdown only. `POST /doc-groups/:id/documents/:docId/pdf` converts the current file with pandoc (`DOCUMENT_PDF_ENGINE`, default `typst`) into `DOCUMENT_PDF_DIR` (default `<root>/.doc-pdf/<file_id>.pdf`) when no export row for the current file hash exists, and returns it. Sync deletes the export (row and file) when the source hash changes. A PDF document is returned as is.

**D11 API.** Authenticated read: `menu`, `:id/tree`, `:id/documents/:docId/file`, `:id/documents/:docId/pdf`. Admin: list/create/update/delete groups, list/add/remove members, remove document (with `delete`), rescan, classify. Errors are JSON `{error}`, never leak absolute paths.

**D12 Frontend.** `DocumentBuilderView` takes a `group` key; the admin page is a view under Development › System Admin. All text in Paraglide.

## Reused modules

Markdown Viewer `20261109-01` (rendering, unchanged); Document Builder `20261109-02/-03` (scan, title, version parsing, classifier, tree builder and view are extended; the TOML source loader, manifest and old handler/CLI are removed because groups supersede them); ID manager (document IDs); pageconfighandler (admin check pattern, role validation). Not extended: page-config entries themselves, because group menu entries need per-installation rows that an admin creates at run time, which the seed-based page-config does not model.

## Risks / Trade-offs

Move detection by hash can mis-link two identical files (a copy mistaken for a move when the original went missing the same sync): accepted, identical content makes it harmless. Syncing on tree requests costs a directory walk: bounded by member size and the mtime/size shortcut. Deleting a version file the editor still has open: archive move, not unlink, keeps it recoverable. Seeding replaces two hard-coded menu entries; page-config rows for them become unused.

## Migration

One goose project migration creates the tables and seeds the two groups. Startup provisions `doc_registry_id`. Legacy manifests are imported on first sync. Rollback: Down drops the `kb.doc_*` tables; the old TOML/CLI code is recoverable from jj history.
