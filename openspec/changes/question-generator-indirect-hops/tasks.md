## 1. Prompt

- [x] 1.1 `prompt-question-generator-search-metrics-v3.md`: indirect hop definitions, one target per question, `metric_name_en`
- [x] 1.2 Default prompt constant → v3; local `mise.local.toml` → v3

## 2. Generator

- [x] 2.1 Load `metric_name_en` per sampled metric (best effort) into the model input
- [x] 2.2 `checkMetricNameLeak` in `validateMetricQuestions` (two-hop, multi-hop)
- [x] 2.3 Unit tests (leak check; default prompt name)

## 3. Verification

- [x] 3.1 Live generation, record 753, 4 metrics: 12 questions, all two/multi-hop indirect and single-target
- [x] 3.2 Benchmark of those 12 (keywords on): hybrid ranks 1–3 for one-hop, up to 11 (two-hop) and 44 (multi-hop)

## 4. Docs

- [x] 4.1 Update devdocs 2026102401 (question generator) and 2026102501 (hybrid search benchmark)
