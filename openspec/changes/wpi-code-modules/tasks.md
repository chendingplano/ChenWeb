## 1. Backend: code modules and class rows

- [x] 1.1 `types.go`: `ElementSchemaVersion` `"0.4"`. `validate.go`: `elementAttrs` → `codeModules` keyed by module; `classAttrs` gains `code_module_id`; module-keyed `elementCode`, `element(module, …)`, `elementComplete(module, …)`
- [x] 1.2 `elementClassAttrs`: structural for every row (any well-formed name), `instance_modifiable` (boolean; `false` needs a non-null default, not action-holding); returns defaults, given, fixed
- [x] 1.3 `classRow` (active element row): `code_module_id` required and registered; required attributes of the module given; values checked for module-defined attributes only; action class rows refuse `code_module_id`. `validateElementClass(body, draft)`: drafts may omit `code_module_id` (well-formed if present), active rows need a registered one
- [x] 1.4 `classWarnings(id, body)`: one warning per attribute the row's registered module does not define

## 2. Backend: instances and page writes

- [x] 2.1 Stage 1 element instance checks become structural (ID, no class attributes, sizes, shared attributes); `actionPositions` without code lookup
- [x] 2.2 Stage 2 `againstClass` for elements: class row names the attribute (or it is a size), not fixed; merge with fixed values last; module value checks and `elementComplete` by the row's module; `withPageDefaults` asks the row and skips fixed `binding_mode`
- [x] 2.3 `loadClasses` helper shared by `BuildBundle` and `PutDescriptor`; page writes run stage 2; bundle adds `classWarnings` of used element classes
- [x] 2.4 Handlers return `warnings`: `PutDescriptor` (element), `CreateElementClass`, `UpdateElementClass`
- [x] 2.5 Go tests updated to format `0.4` and extended: shared module, unregistered module, draft module, fixed attributes (row rules, instance refusal, merge, page `binding_mode`), unsupported-attribute warnings (writes and bundle), instance setting an attribute its class does not name, page write against classes; `go test ./api/wpi/...`, `go vet`

## 3. Migration

- [x] 3.1 `project_migrations/20261028000001_wpi_element_classes_0_4.sql` (transform latest draft/active `0.3` rows; idempotent; down deletes); `seed_test.go` applies the same transform
- [x] 3.2 Dry-run the Up section against the dev database in `BEGIN … ROLLBACK`; then apply with `doc-service --migrate-only` and check `/api/v1/wpi/apps/customer-admin` (200, one warning for `label_hori_align`)

## 4. Frontend interpreter

- [x] 4.1 `elements/index.ts`: `codeModules`; `entries.ts`: `CLASS_ATTRS` gains `code_module_id`, `mergeElement` applies fixed values last, `resolveEntry` adds `module` and skips a fixed `binding_mode`; `Element.svelte` draws by module, error box names the module (message in both languages); `types.ts`, `api.ts` (warnings, `instance_modifiable`)
- [x] 4.2 Unit tests: `entries.test.ts`, `api.test.ts`

## 5. Editor

- [x] 5.1 `element-class-form.ts`: `code_module_id` class attribute (text, optional), schema `0.4`, `fixed` field carried as `instance_modifiable: false`, fixed-without-default problem; tests
- [x] 5.2 `CreateElementDialog.svelte`: Fixed checkbox (excludes Required), "fixed" tag, registered modules listed while editing `code_module_id`, "Saved" panel with warnings before closing; messages in both languages
- [x] 5.3 `PropertiesPanel.svelte`: fixed attributes read-only with "Fixed by the class"
- [x] 5.4 `bun run check`, unit tests; browser run of the editor (create a class sharing `button` with a fixed and an unsupported attribute; warnings shown; Properties panel) and of `/wpi/customer-admin`

## 6. kb.code_modules (design D9)

- [x] 6.1 Migration `20261028000002_create_kb_code_modules.sql` (table, comments, six element modules); dry-run Up twice and Down in `BEGIN … ROLLBACK`; applied to dev by the dev server's restart
- [x] 6.2 `store.go`: `storedModule` kind (active element modules), `listCodeModules`; `bundle.go`: `moduleErrors` for used element classes; `handler.go`: `checkModule` in the three class writes, `ListCodeModules`; route `GET /api/v1/wpi/code-modules`
- [x] 6.3 Go tests: store queries, list endpoint, writes refused for a missing or suspended module, a draft naming a module without code, bundle refused for a suspended module, `TestSeedCodeModules` (seed ↔ `codeModules`); seed tests use the seeded modules
- [x] 6.4 Frontend: `listCodeModules` (+ test); the class form lists the active element modules from the endpoint; the message after creating a draft says pages can use it once an admin makes it active (was "once its code is implemented"); browser run repeated

## 7. Docs

- [x] 7.1 Devdoc 2026102601 §3.5.1–§3.5.2, §6, §7, §8.1 and status: built; code module docs (row in `kb.code_modules`, "Changing this module"); reusable modules 20261026-01/-02 (version, files, description)
- [ ] 7.2 Commit ChenWeb and KnowledgeStore separately (the user commits)
