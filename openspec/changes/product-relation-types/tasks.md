## 1. Database

- [x] 1.1 Goose migration: create `kb.product_relation_types` (columns, CHECK for approved descriptions) and seed the 20 codes + `oth` with en/zh descriptions and weights per design D2
- [x] 1.2 Nav entry for a standalone page: superseded. The review is a Consistency Check card, and migration `20261118000003` removes the entry `20261118000002` seeded

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

## 5. Consistency Check card

- [x] 5.1 Client module and `product-relation-types-check.svelte` card (Check lists proposals; approve form with en/zh descriptions and weight default 0.5; delete of unused proposals)
- [x] 5.2 Embed the card in `db-consistency-view.svelte`; add `prt_*` keys to `en.json` and `zh-cn.json`
- [x] 5.3 `bun run check` passes and the view compiles without warnings
- [ ] 5.4 Open the Consistency Check page in the browser and verify Check, approve (both descriptions required, weight 0.5) and delete against a live server

## 6. Docs and close-out

- [x] 6.1 Correct bug 2026111703 (`stor` already covers transport; fix is this change). The GitHub issue #31 comment is not posted: needs the user's go-ahead
- [x] 6.2 Devdoc for the relation vocabulary (create-devdoc skill)
- [x] 6.3 Registering a reusable module: not needed (copied page pattern, see design "Reused modules")
- [ ] 6.4 `go vet` and `go test` for the touched packages; answer the CLAUDE.md "coding best practice" questions in the final report
