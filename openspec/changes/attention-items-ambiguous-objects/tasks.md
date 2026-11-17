## 1. Attention items table and store

- [x] 1.1 Goose migration creating `kb.attention_items` with the status CHECK, the partial unique index on (`item_type`, `dedupe_key`) for `open`/`dismissed`, and the type/status/last_seen index (use the `db-migration` skill); apply it to the dev database
- [x] 1.2 Package `server/api/attention/`: item type constants (`ambiguous-object-name`), `Item` type, `Recorder` interface, SQL store with `Record` (upsert per design D2), `Get`, `List`, `Resolve`, `Dismiss`; loggers from `CreateDefaultLogger`
- [x] 1.3 Store tests against a test database: insert, repeat of open, reopen after resolve and after dismiss (history kept, closing columns cleared), concurrent record, unknown type rejected, resolve/dismiss only from open

## 2. Attention items API

- [x] 2.1 Handlers for list, get, resolve, dismiss (`resolved_by` from the request user, as `structureActor` does); register the routes in `server/api/routes.go`
- [x] 2.2 Handler tests: filters and order, resolve with note, closing a non-open item gives 4xx, unknown ID gives 404

## 3. Reconciliation ties

- [x] 3.1 `ReconcileOne`: sort candidates by score (stable), detect ties over every top-scored candidate, return the tied set; unit tests for "1.0 listed after 0.85" and "tie behind a lower score"
- [x] 3.2 Add the optional `Attention attention.Recorder` field to `ObjectReconciler`; set it in the constructors on the tie path (extract metrics, provisions, inventory items; `artifact_postprocess_indexing.go` only runs entity matching and needs none)
- [x] 3.3 `reconcileArtifactObjectsWithLLM` final pass: link remaining ties to `pickTieBreakCandidate` over the tied set (`ambiguous_resolved`, `tie_break_deterministic`), build and record the `ambiguous-object-name` item (dedupe key, title, details with `llm_outcome` and capped occurrences), replace the WARN with INFO `object reconciliation tie broken`, log ERROR and continue if recording fails
- [x] 3.4 Update `object_nodes_test.go` and the resolution tests for the new behaviour; add tests for: no LLM configured, LLM declines, LLM resolves (no item), same tie from two records gives one item with two occurrences, recorder failure still links

## 4. Admin page

- [x] 4.1 `GET /api/v1/kb/objects/ambiguous-items/:id/artifact-object` in `kbhandler`: newest occurrence that still has a `kb.artifact_objects` row; test it
- [x] 4.2 `resolve-ambiguous-objects-client.ts`: attention item types and calls (list, get, resolve, dismiss, representative artifact object)
- [x] 4.3 `resolve-ambiguous-objects-view.svelte`: left list from open items; occurrences panel and recorded tied set; Resolve/Dismiss with note; stored-evidence-only view when no row remains; select the next item after closing
- [x] 4.4 Paraglide keys for all new text in `web/messages/en.json` and `web/messages/zh-cn.json` (real Chinese); `bun run check` passes
- [x] 4.5 Remove `GET /api/v1/kb/objects/ambiguous`, `ListAmbiguousObjects` and `ListAmbiguousSummaries`, and any code left unused by that

## 5. Verification

- [x] 5.1 `go build ./server/...`, `go vet` and `go test` for the touched packages; `bun run check` and `bun test` for the page
- [x] 5.2 Dev run (real reconcile path on dev artifact object 28107, 保护接地端子 tied between obj_811/obj_812, row not saved): confirm the artifact objects are linked, the attention item exists with the expected details, and no WARN is logged
- [ ] 5.3 Browser check of the page on the dev server: list, select, occurrences, merge then Resolve, Dismiss, both locales

## 6. Documentation and catalogue

- [x] 6.1 Amend ADR 2026070701: DR1 superseded by this change (tie-break link plus attention item); link the change
- [x] 6.2 Devdoc for attention items and the new tie path (`create-devdoc` skill)
- [x] 6.3 Register `server/api/attention/` with the `create-reusable-module` skill and run the catalogue check
- [ ] 6.4 Update issue #18 and the 2026-11-17 alert review doc with the change (review doc updated; the GitHub issue comment is left until the change is pushed)

## 7. Deployment (operator)

- [ ] 7.1 After deploy, confirm production logs no new `object reconciliation ambiguous` warnings, then close issue #18
