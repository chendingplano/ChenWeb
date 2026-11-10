## 1. Registry and groups (backend)

- [x] 1.1 Goose migration: `kb.doc_documents`, `doc_files`, `doc_groups`, `doc_group_members`, `doc_group_exclusions`, `doc_group_categories`, `doc_pdf_exports`; seed groups Dev Documents (10) and User's Manuals (20).
- [x] 1.2 Provision `doc_registry_id`; registry sync (known/moved/new version/new document/missing), latest-version resolution, hash/mtime shortcut.
- [x] 1.3 Groups: members, exclusions, documents of a group, access check, menu resolution and ordering.
- [x] 1.4 Per-group classification with legacy-manifest import; remove TOML sources, manifest, old handler and old CLI.
- [x] 1.5 Remove with optional delete: other-group refusal, archive move.
- [x] 1.6 PDF export (pandoc), cache by hash, delete on change.
- [x] 1.7 Endpoints and route registration; `mise build-docs <group>`.
- [x] 1.8 Unit tests and PostgreSQL integration tests (TEST_DATABASE_URL, never miner).

## 2. Frontend

- [x] 2.1 `DocumentBuilderView` takes a group key; PDF export button for Markdown.
- [x] 2.2 Nav rail builds Documents children from `/doc-groups/menu`; content panel renders groups; remove hard-coded entries.
- [x] 2.3 Admin page: groups, labels, parent, order, roles, members, remove/delete, classify, rescan.
- [x] 2.4 Paraglide messages (en, zh-cn), selection, state restore, type check, i18n check.

## 3. Verification and records

- [ ] 3.1 Migration applied through the service and groups registered/classified on real data (done). STILL OPEN: restart the server with DOCUMENT_ROOT_DIR, then check in a browser: menu order, reader, admin page, PDF export, delete/archive.
- [x] 3.2 Update module docs and catalogue (versions, files, config), user manual, add devdoc; commit through jj.

## 4. Phase 2 (not started)

- [ ] 4.1 Comments on documents.
- [ ] 4.2 Tickets on documents.
- [ ] 4.3 To-do list.
- [ ] 4.4 Upload into member directories.
