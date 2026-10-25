## 1. Search effort

- [x] 1.1 Prompts `prompt-search-effort-analyze-v1.md`, `prompt-search-effort-judge-v1.md`
- [x] 1.2 `search_effort.go`: effort parsing, limits from env, LLM client, analyse → attempt → judge loop, RRF fusion, trace
- [x] 1.3 `effort` parameter on the registry search routes; response fields
- [x] 1.4 Unit tests with stubbed LLM and search

## 2. Benchmark

- [x] 2.1 Migration: run `effort`, `max_efforts`; result effort columns
- [x] 2.2 Run option, worker mode `effort`, scores
- [x] 2.3 Page: Effort option, run viewer, Effort column with attempts and queries

## 3. Verification and docs

- [x] 3.1 Live benchmark of v3 questions at low/medium/high
- [x] 3.2 Spec 2026060201, devdoc 2026102501


Notes: `/kb/metrics/search` is a separate full-text search over `kb.metrics`, so effort applies to the registry (hybrid) routes only. Live check after giving the judge each candidate's search-document text and relaxing its matching rule: high effort put 10 of 12 v3 targets at rank 1 (worst 44 → 7).
