// Package comparison implements the DR21 requirement-strictness relation from
// ADR 2026072901 (KnowledgeStore/doc-repo/adrs/202607/
// 2026072901-adr-semantic-platform-and-adaptive-pipeline.md):
//
//	after normalizing two assertions to the same property, quantity kind,
//	assertion kind, condition set, and unit dimension, each constraint denotes
//	a satisfying set of values; comparing the sets yields a directional
//	verdict (identical, equivalent, stronger, weaker, conflict, incomparable,
//	qualitative_only, limit_absent) or, one layer up, an absence/applicability
//	verdict (standard_absent, not_applicable, indeterminate).
//
// This package is a pure function library: no database, no pipeline, no LLM.
// It takes normalized Constraint values and returns a Verdict plus a
// human-readable rationale. It does not itself normalize free-text metric
// output, resolve which document is the "representative" for a family, or
// decide applicability/closed-dimension declarations — those are DR22
// concerns (representative selection, review-scope) that a caller supplies
// as already-resolved inputs to EvaluateFamily.
//
// The unit registry in units.go is a deliberately small, linear-only
// placeholder. DR13 supersedes it with the 4a `quantity` module (QUDT import)
// once that exists; this package's public API (Constraint, Compare,
// EvaluateFamily) does not change when that happens, only the registry's
// implementation.
package comparison
