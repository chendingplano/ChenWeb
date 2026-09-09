# Implementation notes — analysis-node-related-metrics

## Coding Best Practice (CLAUDE.md) — knowledge questions

**What knowledge changed?**
- The Metric Ontology Explorer's `Analysis` satellite is no longer a leaf: it unfolds a
  two-node chain (`Metrics of Same Class`, `Metrics of Similar Classes`).
- New read API: `GET /api/v1/kb/metrics/:metric_id/related-metrics?scope=same_class|similar_class&limit=`.
- New admin API: `POST /api/v1/kb/ontology/class-contracts/backfill-search?limit=&reembed_all=`.
- New table `kb.ontology_class_contract_search` — a hybrid-search index (tsvector + `vector(1536)`)
  over governed class contracts, one row per `term_kind='class'` header.
- New package `server/api/ontology/classcontractsearch` (`Reindex`, `MatchSimilar`,
  `BackfillClassContractSearch`).
- New Phase D side effect: `project_semantics` (`phase_d.go`) now refreshes the class-contract
  search rows for the record's classes, post-commit, best-effort.
- Frontend: `model.ts` `ChainNode.related`; `metricOntologyExplorerService.getRelatedMetrics` +
  two `PROJECT` entries; `content-viewer.svelte` lazy per-`(metricId,scope)` branch with
  clickable rows that deep-link `?metric_id=`.

**Which docs / specs / ADRs / tests are affected?**
- Specs: this change's `specs/metric-analysis-neighbors/spec.md` and
  `specs/class-contract-hybrid-search/spec.md` (new capabilities).
- Tests added: `server/api/kbhandler/related_metrics_handler_test.go`,
  `server/api/ontology/classcontractsearch/store_test.go` (helpers + migration shape),
  `web/.../metric-ontology-explorer/model.test.ts` (analysis chain + PROJECT).
- Related ADRs (context, unchanged): `2026081701` (class contract machinery), `2026082203`
  (class resolution), `metric-class-contracts` change.

**Which docs were updated?**
- The two new spec files and this notes file. `HowTo.md` was inspected — it is a
  dev-environment / neovim how-to with **no** KB-API or Metric Ontology Explorer section, so
  there is nothing there to update (the sibling `metric-scoped-explorer` change added no HowTo
  entry for `/graph` either). The endpoint contracts live in `design.md` + the spec files.

**Which docs are now stale?**
- `openspec/changes/metric-scoped-explorer` (still unarchived): its promoted-pending
  `metric-ontology-explorer` spec does not yet mention the `Analysis` chain. Not stale in a
  wrong sense — just incomplete until reconciled at archive (see below).

**What was intentionally left undocumented?**
- Tuning constants (`similarClassCandidateK = 30`, RRF `k = 60`, `candidateLimit = 200`,
  `instanceSampleRows = 20`, default N=20 / 200) are named consts in code, not surfaced as
  config — promote to env only if tuning demands it (design Open Questions).
- `class_label` currently equals `class_term_id` (no reliable human label for synthesized
  class terms today) — design Open Question, not a doc.

## Spec lineage (task 9.2)

`metric-scoped-explorer` owns the `Analysis` satellite and the record-tab renderer but is **not
yet promoted** to `openspec/specs/`. This change therefore adds **new** capabilities rather than
a MODIFIED delta. When `metric-scoped-explorer` (and/or `metric-ontology-explorer-page`) is
archived, the promoted `metric-ontology-explorer` spec must absorb the `Analysis` two-node chain
and the lazy record-tab source as a **non-conflicting addition** — add this to that change's
archive checklist.

## Post-run correction (found by running the backfill against `miner`)

- **`term_kind` assumption was wrong.** Synthesized metric classes (the `measurement:kwc_*`
  terms an assertion's `instance_of_term_id` points at) carry `term_kind = 'metric_definition'`,
  not `'class'`. `term_kind = 'class'` is only the ~20 abstract framework classes (`core:*`,
  `da:*`, `mea:*`), which have **zero** metric instances. The first backfill picked up only
  those 20. Fixed: `classcontractsearch.classTermPredicate` now selects a term with a
  `current_contract_revision_id` OR one referenced as an assertion `instance_of_term_id`.
  `miner` now: 78 indexed (56 metric classes + 20 base + 2 `semantic:can_*` concept terms),
  77 with embeddings.
- **New CLI `server/cmd/class-contract-search-backfill`** — the `/api/v1` group is behind
  `authmiddleware` (Kratos session), so the HTTP `backfill-search` endpoint needs a logged-in
  cookie. The CLI talks straight to the DB (precedent: `server/cmd/metric-contract-backfill`).
  Run: `mise exec -- go run ./server/cmd/class-contract-search-backfill` (`--reembed-all`,
  `--lexical-only` flags). Needs `docprocessing.EmbedSearchQuery` (added — exported wrapper
  over the unexported `embedQueryText`).
- Benign: the search embedder's read path logs `WARN ... llm usage event missing mandatory
  call_reason/call_loc` per embed call — pre-existing gap in `embedQueryText`, not introduced
  here; telemetry only.

## Deferred / needs a live environment

- `1.2` apply migration (air auto-applies on restart); `5.3` run `backfill-search` until
  `remaining=0`; `2.6` / `3.3` DB-backed integration tests; `8.1`–`8.4` browser walkthrough.
  All code builds (`go build ./...`), `go vet` clean, unit tests pass, `bun run check` shows no
  new errors/warnings, `bun test model.test.ts` green.
