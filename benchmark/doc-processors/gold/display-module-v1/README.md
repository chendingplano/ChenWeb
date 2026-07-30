# Display module gold fixture (显示屏模块)

Synthetic, hand-authored ground truth for the pilot vertical slice of
ADR 2026072901 (`KnowledgeStore/doc-repo/adrs/202607/2026072901-adr-semantic-platform-and-adaptive-pipeline.md`).
All standard numbers, issuers, editions, and values in `gold.toml` are synthetic —
see the file header before citing anything from it as real.

## What this is

`gold.toml` defines one part class (`vent:display_module`), 9 metric definitions
(8 real + 1 near-miss distractor), 9 authority documents across 5 families
(CN national, international, EU, US, enterprise), 40 clauses, and 36 hand-derived
expected verdicts covering all 11 verdict kinds from DR21. Every expected verdict
carries a `rationale` naming the DR21 rule that produced it, so it is auditable
rather than asserted.

Validate it parses and check verdict coverage:

```sh
python3 -c "
import tomllib
from collections import Counter
with open('gold.toml', 'rb') as f:
    d = tomllib.load(f)
print('clauses:', len(d['clause']), 'verdicts:', len(d['expected_verdict']))
print(Counter(v['verdict'] for v in d['expected_verdict']))
"
```

## What is built

- **`generate.go`** (package `gold`) loads `gold.toml` and builds one
  `model.Document` per `[[authority_document]]`, one paragraph block per
  `[[clause]]`, block ID == clause ID. `generate_test.go` validates every
  generated document with `model.Validate` and, when `typst` is on `PATH`,
  round-trips the richest document (`doc:ent-q-syn-001-2026`) through the real
  CDM Typst renderer and `ExtractAnchors`/`DeriveFragments` — confirming ADR
  2026072901 DR25's "grounding is exact by construction" claim actually holds
  for content generated from this fixture, with a real compile, not a mock.
- **`server/api/ontology/comparison`** implements the DR21 strictness relation
  (`Compare`, `EvaluateFamily`) as a pure function and reproduces all 36
  `expected_verdict` rows below via `gold_fixture_test.go`.
- **`server/api/doc-benchmark/verdict_score.go`** implements `ScoreVerdictMatrix`,
  the outcome scorer the benchmark ADR §3.4 flags as not yet defined: it matches
  an actual verdict matrix against gold by (metric, family, object), reporting
  accuracy, a per-verdict-kind breakdown, and mismatched/missing/unexpected
  diagnostics. `verdict_score_gold_test.go` runs it against this fixture's full
  36-cell matrix (built via `comparison.EvaluateFamily`, same as above) and
  against an injected single-cell regression.
- **`resolve.go`** (this directory, package `gold`) is the production
  implementation of the gold-resolution logic that was previously duplicated
  test-only in two places: `gold.Resolve(File)` builds the expected verdict
  rows plus each metric's subject/reference constraints, and
  `Resolved.SimulatedActual()` computes a stand-in "actual" matrix via
  `comparison.EvaluateFamily`.
- **`server/api/doc-benchmark/corpus_dataset.go`** implements a corpus-level
  dataset kind (`CorpusDataset`/`LoadCorpusDataset`) that loads
  `manifest.json` in this directory, resolves its one case's `gold.toml`, and
  exposes `Documents()`, `Expected()`, and `SimulatedActual()`. It is a fully
  parallel type to the existing single-input-file `Dataset`/`Case` -- nothing
  in the existing dataset/orchestrator code was touched.

## What is NOT yet built

1. **Wiring `CorpusDataset` into the live execution engine.** `LoadCorpusDataset`
   loads, generates, and scores a corpus case entirely through this package's
   own path-safety helpers and `gold.Resolve`/`comparison`/`ScoreVerdictMatrix`
   -- but it stops there. It is not wired into
   `server/api/doc-benchmark`'s orchestrator/runner/store (`application.go`,
   `application_execute.go`, `runner.go`), so there is no CLI command that
   runs a corpus case against a real doc-processor pipeline. That wiring needs
   a real DB, NATS, and LLM credentials this session has no visibility into.
2. **`extract_metrics` structured value output** (comparator, normalized
   value, unit term — see ADR 2026072901's note on `threshold_or_target` free
   text) and `normalize_assertions`. Until both land, even a fully wired
   orchestrator would have nothing real to put in place of
   `CorpusCase.SimulatedActual()` — real pipeline output still can't become a
   `comparison.Constraint`.
3. **Representative selection.** `gold.Resolve` picks the enterprise subject
   clause and each family's reference clause directly from fixture markers
   (the hardcoded `SubjectDocument` id, family lookup) and errors on any
   ambiguity; DR22's real precedence policy (supersession, newest edition,
   multi-document equivalence grouping) is not implemented.

## Known finding surfaced while authoring this fixture

Several rows in the proposed application's mock screenshot show a green "一致"
(identical) verdict where the compared authority's clause is qualitative (no
numeric limit) — e.g. "触控响应时间" and "有效视角". Under DR21, comparing a
quantitative enterprise value against a qualitative requirement can never be
`identical`; the correct verdict is `qualitative_only`. This fixture reproduces
the same rows with the technically correct verdict instead, and the mock should
not be treated as gold for those cells.

## Reuse as pilot 4b module content

Once the ontology data repository exists (ADR 2026072901 DR17), the
`[[metric_definition]]` blocks here are close to direct input for that module's
`terms.toml`, and the `[[clause]]` values are candidate conformance fixtures for
its profile rules. This file intentionally uses the same identifiers
(`vent:display_brightness`, etc.) so that migration is a copy, not a rewrite.
