## 1. Database

- [x] 1.1 Goose migration: create `kb.product_relation_types` (columns, CHECK for approved descriptions) and seed the 20 codes + `oth` with en/zh descriptions and weights per design D2
- [x] 1.2 Goose migration: seed `kb.page_config` nav entry for the new page; apply with `doc-service --migrate-only` and verify the seed

## 2. Extractor

- [x] 2.1 Store layer: load/cache the table, insert-proposed (`ON CONFLICT DO NOTHING`), bump `seen_count`, behind an interface with a fake for tests
- [x] 2.2 `products_v2.go`: make `r` a plain string in the response schema; resolve code → value through the store; keep the empty-code contract failure
- [x] 2.3 `MAX_PRODUCT_RELATION_TYPES` (default 50) and the limit WARN; one WARN per new proposed code
- [x] 2.4 Tests: known code, unknown code (reproduces #31 `pkg` without retry), repeat proposed code, empty code, limit exceeded, concurrent insert

## 3. Product review

- [x] 3.1 `docscope.go`: weight from the table by value, falling back to TOML then 0.3; mark the TOML table deprecated in `product-review.local.toml`
- [x] 3.2 Tests: approved weight used, fallback order

## 4. API

- [x] 4.1 Handlers + routes under `/kb/product-relation-types`: list with counts, approve, edit, delete-unused-proposed, with validation per design D6
- [x] 4.2 Handler tests: 400 on blank description or weight out of range, delete refused when in use

## 5. Page

- [x] 5.1 Client module and view modeled on the Object Types page; approve form with en/zh descriptions and weight default 0.5
- [x] 5.2 Wire `nav-rail.svelte` and `content-panel.svelte`; add `prt_*` keys to `en.json` and `zh-cn.json`
- [ ] 5.3 `bun run check` passes; open the page in the browser and verify list, approve and validation

## 6. Docs and close-out

- [x] 6.1 Correct bug 2026111703 (`stor` already covers transport; fix is this change). The GitHub issue #31 comment is not posted: needs the user's go-ahead
- [x] 6.2 Devdoc for the relation vocabulary (create-devdoc skill)
- [x] 6.3 Registering a reusable module: not needed (copied page pattern, see design "Reused modules")
- [ ] 6.4 `go vet` and `go test` for the touched packages; answer the CLAUDE.md "coding best practice" questions in the final report
