## 1. Schema

- [x] 1.1 Goose migration: create `kb.ref_resources`, `kb.ref_resource_versions`, `kb.ref_resource_identifiers`, `kb.ref_mentions`, `kb.reference_artifact_links` (indexes per design D4/D8)
- [x] 1.2 Goose migration: add `ref_mention_id`, `ref_input_record_id`, `ref_metric_id`, `ref_metric_key` to `kb.metrics` + partial index on `ref_mention_id`
- [x] 1.3 Apply migrations on the dev DB and verify the tables and columns

## 2. Reference model and parsing (pure code)

- [x] 2.1 `reference_vocabulary.go`: governed enums (target types + subtypes with `others`, mechanisms, relationships, scopes, contents, forms, list kinds) and `normalizeMentionLabels`
- [x] 2.2 `reference_identifiers.go`: identifier parsing/normalization (standard, DOI, URL, arXiv, ISBN, ISSN, PMID, RFC, legal citation), identity key, edition, public-kind check
- [x] 2.3 Normativity derivation: list linking, `normativity`, `list_mismatch`; numbered/author-year citations inherit list-entry identifiers
- [x] 2.4 Stable keys (`mention_key`, metric artifact key)
- [x] 2.5 Unit tests for 2.1–2.4 (standards incl. `GB/T755-2019`, `ISO 14253-1`, DOI, URL; normativity cases from the spec)

## 3. Processor

- [x] 3.1 Prompt `prompts/prompt-extract-references-v1.md` (reference model, tests for criterion vs method vs data, multi-domain examples, JSON schema, document_identifier)
- [x] 3.2 `extract-references.go`: `ReferencesProcessor` as `ChunkBatchProcessor` (model/prompt env, per-chunk LLM call, parse, de-duplicate)
- [x] 3.3 Reference store: replace mentions in a transaction, catalog upsert (resources/versions/identifiers), own-identity attachment (design D5)
- [x] 3.4 R2 holding with access (design D6)
- [x] 3.5 Register: `processor_plan.go` spec, `runtime.go`, `control.go` chunking dependency, `config.toml` `required_processors`
- [x] 3.6 Processor tests (sqlmock or fakes): parse → store flow, re-run replaces mentions — pure assembly/parse tests (`reference_metric_resolver_test.go`); store flow verified live (8.2)

## 4. Resolver contract

- [x] 4.1 `reference_resolver.go`: interface, registry, `ResolveOutgoing` (step 5), `ResolveIncoming` (step 6 / holding event), write-then-look, advisory lock, link upserts
- [x] 4.2 Wire step 5 and the holding event into `ReferencesProcessor.FinalizeChunkBatch`; withdraw before replacing mentions
- [x] 4.3 Tests: registry dispatch, Takes filtering, idempotent double resolution — dispatch/Takes unit tests; idempotent double resolution verified live (8.3)

## 5. Metric family

- [x] 5.1 `reference_metric_resolver.go`: Takes, HasArtifacts, candidate query (own metrics + dropped definitions), scoring policy `metric_reference_match/v1`, outcome states
- [x] 5.2 Apply: links + `<rec>_rfm_<n>` rows in `kb.metrics` (design D10); Withdraw by citing or target record
- [x] 5.3 `extract_metrics`: delete guard, own-rows-only read for merge/objects, step 6 call after save (WARN on failure)
- [x] 5.4 Tests: scoring cases (clear match, ambiguous, no metrics → type_doubtful, criteria_set), Apply row shape, extract_metrics delete keeps resolved rows — scoring cases unit-tested; Apply row shape and delete guard verified live (8.3)

## 6. kb.metrics readers

- [x] 6.1 Add `ref_mention_id IS NULL` to every non-test `kb.metrics` query site (design D13), except the metric list API
- [x] 6.2 Metric list API returns `ref_*` fields
- [x] 6.3 Guard test: every non-test Go `kb.metrics` query carries `ref_mention_id` or is allowlisted
- [x] 6.4 `.agents` `score_io.py` / `benchmark_io.py`: exclude resolved rows

## 7. Dashboard

- [x] 7.1 Dashboard: no code change needed — `doc-processor-dashboard-state.ts` `buildStageDefs` derives stages from `[doc-processing].required_processors` (label "Extract References"); the capsule §12.4 checklist names lists that no longer exist

## 8. Verify

- [x] 8.1 `go build ./...`, `go vet`, `go test ./server/api/doc-processing/...` and touched packages — `go build ./...`, `go vet`, `go test ./api/... ./cmd/...` all pass
- [x] 8.2 Live run: `extract_references` on records 416 and 753 (and one non-Chinese document if available); inspect mentions, normativity, catalog, R2 — records 753, 373, 362 (Chinese standards); fixes from the run: one list entry per line, `not_listed` only for publicly identified documents, no catalog entry for indirect references, no TOC lines (prompt), regional numbers like JJG(苏)263-2024 for own identity. **Not done:** a non-Chinese document — none has its artifacts on this machine (650 ISO 14068-1 is not on disk)
- [x] 8.3 Live step 5/6: a record citing another processed record; verify resolved rows and links, then re-run `extract_metrics` on both sides and verify nothing is lost or duplicated — no natural citing/cited pair with metrics in the corpus; verified with a synthetic criterion mention in 362 → 373 (GB 3053-1993): step 5 resolved to 373_mtc_1 as 362_rfm_1, step 6 replaced (still one row), extract_metrics delete kept it, deleting the mention cascaded row and links. The `extract_metrics` re-run itself (LLM) was not repeated

## 9. Docs

- [x] 9.1 Capsule: `extract-references-spec.md`, `extract-references-impl.md`, pipeline table row, status JSON subsection
- [x] 9.2 Devdoc 2026101002: status → built (v1), note D9 (deterministic matching) and the follow-ups
