# Summary Benchmark Implementation Plan

**Goal:** Freeze an auditable, offline summary benchmark before evaluating candidate algorithms.

**Architecture:** Versioned source fixtures and evidence units, a pure scorer and
small JSON CLI, baseline controls, and a distinct semantic-review rubric.

**Tech stack:** Python 3.10+ standard library; no database, network or model dependency.

## Tasks

- [x] Write failing scorer tests for evidence, exceptions, incorrect numbers,
  budget gates, missing cases, duplicates, invalid spans and fixture hashes.
- [x] Implement `benchmark/summaries/benchmark.py` with strict fixture validation,
  exact extract scoring, grouped reports and baseline/control output.
- [x] Author `benchmark/summaries/datasets/1.0.0/benchmark.json` with multilingual
  diagnostics and real-source pilot excerpts; keep source provenance and rationale.
- [x] Add `README.md`, review rubric and candidate format; document measurements
  and the boundary between citation coverage and semantic quality.
- [x] Independently review design, annotations and scorer; correct findings.
- [x] Freeze dataset digest, run tests and CLI controls, save validation report.
- [x] Commit only new benchmark and plan/spec paths through jj; check linear log.

## Verification commands

From `ChenWeb/benchmark/summaries`:

```sh
python3 -m unittest discover -s tests -v
python3 benchmark.py validate
python3 benchmark.py baseline --method gold-control --output /tmp/summary-gold.json
python3 benchmark.py score /tmp/summary-gold.json
```

Gold control must reach full evidence coverage within budget. Adversarial controls
must fail the relevant gates. Passing these checks validates the benchmark machinery,
not the production suitability of a summarizer.

## Completed verification

2026-10-21: 26 automated tests pass; all CLI control/export/scaling checks pass.
Independent agent audit corrected treatment applicability in eight table units and
confirmed freeze readiness. See `benchmark/summaries/datasets/1.0.0/audit.md` and
`validation.json`. Candidate algorithms and human semantic review remain the next phase.
