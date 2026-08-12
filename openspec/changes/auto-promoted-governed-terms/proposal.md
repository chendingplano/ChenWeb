## Why

`kb.ontology_terms` has zero `metric_definition` rows, and the only path that
can create one (`CandidateStore.PromoteToContent`) hard-requires human
approval of a candidate from `extract_metric_definitions` — a harvester that
only fires when a document explicitly defines a metric in prose, which most
documents don't do for most metrics. At the volume this system must handle
(plausibly hundreds of thousands to millions of distinct metrics — "a domain
has hundreds of metrics" turns out to be an unfounded ADR assumption, not a
real bound), mandatory human review of every new governed term is not a
slower version of the same workflow, it's an infeasible one. ADR
`2026081201` decides that `metric_definition` term creation must become
automatic, with human review as an optional, reactive quality mechanism
instead of a gate — extending the keyword module's already-shipped D11
auto-first policy (concept auto-creation) one layer up, to governed terms.

## What Changes

- Fix `KEYWORD_RESOLVER_MODE` so resolved answers (`keyword_concept_id`)
  actually reach consumers (`ResolvingMetricsStore`) — currently `"off"` by
  default and unwired even when set (`"on"` mode is a known dead end per
  spec `2026080403` D9; `0` of `7,040` `kb.metrics` rows carry a resolved
  identifier today).
- Add a new `kb.ontology_terms.status` value, `'auto-promoted'` (migration:
  extend `ontology_terms_status_check`). A term with this status is live and
  usable immediately, distinguished from `included_in_release` only for
  attribution/sampling, not to block usage.
- When a `kb.keyword_concepts` row has no accepted `aligns_to_term`
  alignment to an existing governed term, auto-create a new
  `metric_definition` term for it (`status='auto-promoted'`) and
  auto-accept the alignment, instead of leaving `metric_definition_term_id`
  empty until a human promotes a candidate.
- Synthesize the auto-created term's payload structurally from the
  triggering `kb.metrics` row's already-extracted fields (`value_data_type`,
  `value_range_type`, `metric_unit` resolved against the released QUDT
  `quantity` module, `formula_or_definition` when present) and the
  concept's own label/aliases — no new LLM authoring step.
- **BREAKING (policy, not schema):** `ComparisonStore.validateMetricKey`
  (`server/api/ontology/comparison/store.go`) currently accepts only
  `status='included_in_release'` as a valid comparison-matrix row key.
  Extend it to also accept `'auto-promoted'`, so the Document Review app can
  use auto-promoted terms immediately.
- Retire `extract_metric_definitions` from the default routed pipeline
  (remove from pipeline-policy/`config.toml` processor selection). Code,
  tests, and prompt stay in the repo, unmodified; the capsule doc
  (`extract-metric-definitions-spec.md`) is marked retired, pointing to ADR
  `2026081201`, not deleted. `+CAPSULE.md`'s pipeline table, §7.1
  processor-category lists, and §9.11 status subsection are updated to
  reflect retired-by-default status.

## Capabilities

### New Capabilities
- `keyword-resolver-serving`: makes `KEYWORD_RESOLVER_MODE` actually serve
  resolved `keyword_concept_id` answers to consumers end-to-end (diagnosing
  and fixing the "on"-mode/observe-mode consumer gap spec `2026080403` D9
  flags as broken). Prerequisite for everything else in this change.
- `governed-term-auto-promotion`: concept→term resolution with
  auto-create-on-no-match, the `auto-promoted` status, intrinsic-property
  term synthesis, and `ComparisonStore.validateMetricKey` accepting
  auto-promoted terms as comparison-matrix row keys.

### Modified Capabilities
(none — no pre-existing `openspec/specs/` capabilities in this repo yet to
modify; the doc-processor pipeline's processor-selection behavior is
governed by the doc-processor capsule, not an OpenSpec capability spec, so
`extract_metric_definitions`'s retirement is covered under
`governed-term-auto-promotion`'s spec rather than as a separate modified
capability.)

## Impact

- **Schema:** new goose migration adding `'auto-promoted'` to
  `kb.ontology_terms`'s status CHECK constraint.
- **Code:** `server/api/ontology/keywords/{mode.go,keywordfamily.go,alignment.go}`,
  `server/api/doc-processing/extract-metrics.go` (`ResolvingMetricsStore`),
  `server/api/ontology/terms/` (term creation), `server/api/ontology/comparison/store.go`
  (`validateMetricKey`), `server/cmd/doc-processor/main.go` / pipeline-policy
  config (removing `extract_metric_definitions` from the default selection).
- **Docs:** `KnowledgeStore/Capsules/coding-capsules/doc-processor/extract-metric-definitions-spec.md`
  (marked retired), `+CAPSULE.md` (pipeline table, §7.1, §9.11).
- **Downstream:** Document Review app's comparison matrix can start
  returning auto-promoted-term rows once this ships — no app-side change
  required by this OpenSpec change itself, but its UI should eventually be
  able to distinguish auto-promoted from curator-released rows (out of scope
  here; flagged in the ADR as a consequence, not a task of this change).
