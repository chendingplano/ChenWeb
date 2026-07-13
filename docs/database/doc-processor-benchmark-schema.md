# Benchmark schema and retention

The benchmark migration creates datasets, runs, case runs, attempts, results, scores, and artifacts. Run/case/attempt ownership and leases prevent duplicate claims; result and score evidence is immutable after verification. Canonical chunk and metric queries are kept in the adapter package and are ordered deterministically.

Retain verified evidence and reports. Cleanup may remove only explicitly discarded, unverified attempt inputs/workspaces; rollback removes the complete benchmark schema in dependency order. Never use broad deletes against production `kb.chunks` or `kb.metrics` rows.

## Out of scope for v1

MLflow, LangSmith, Promptfoo, web UI/public API, CI/release gating, PDF parsing/OCR, human-annotated production documents, LLM-as-judge/open semantic grading/arbitrary unit conversion, combined pipeline-wide score, automatic prompt optimization, automatic retention/purge of verified evidence, processors beyond `chunking`/`extract_metrics`, and multi-host distributed scheduling.
