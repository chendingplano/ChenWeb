package semrules

import "testing"

func TestAnalyzeOverlap(t *testing.T) {
	tests := []struct {
		name           string
		left           Document
		right          Document
		wantMayOverlap bool
		wantAnalyzable bool
		wantReason     string
	}{
		{
			name: "equal conjunctions overlap",
			left: allFacts(
				fact("document.doc_kind", "eq", "standard"),
				fact("document.domain", "in", []any{"mechanical", "electrical"}),
			),
			right: allFacts(
				fact("document.doc_kind", "eq", "standard"),
				fact("document.domain", "in", []any{"mechanical", "electrical"}),
			),
			wantMayOverlap: true,
			wantAnalyzable: true,
			wantReason:     "equal_constraints",
		},
		{
			name: "disjoint conjunctions do not overlap",
			left: allFacts(
				fact("document.doc_kind", "eq", "standard"),
				fact("document.domain", "eq", "mechanical"),
			),
			right: allFacts(
				fact("document.doc_kind", "eq", "standard"),
				fact("document.domain", "eq", "electrical"),
			),
			wantMayOverlap: false,
			wantAnalyzable: true,
			wantReason:     "disjoint_path_values",
		},
		{
			name: "intersecting conjunctions may overlap",
			left: allFacts(
				fact("document.doc_kind", "eq", "standard"),
				fact("document.domain", "in", []any{"mechanical", "electrical"}),
			),
			right: allFacts(
				fact("document.doc_kind", "in", []any{"standard", "manual"}),
				fact("document.domain", "eq", "mechanical"),
			),
			wantMayOverlap: true,
			wantAnalyzable: true,
			wantReason:     "intersecting_constraints",
		},
		{
			name: "unconstrained path still may overlap",
			left: allFacts(
				fact("document.doc_kind", "eq", "standard"),
			),
			right: allFacts(
				fact("document.doc_kind", "eq", "standard"),
				fact("document.domain", "eq", "mechanical"),
			),
			wantMayOverlap: true,
			wantAnalyzable: true,
			wantReason:     "unconstrained_path",
		},
		{
			name: "ordered comparison is outside analyzable subset",
			left: allFacts(
				fact("document.numeric_unit_density", "gt", 3),
			),
			right: allFacts(
				fact("document.numeric_unit_density", "lt", 5),
			),
			wantMayOverlap: true,
			wantAnalyzable: false,
			wantReason:     "unsupported_operator",
		},
		{
			name: "any is outside analyzable subset",
			left: Document{Version: 1, Expression: Predicate{Kind: "any", Items: []Predicate{
				{Kind: "fact", Path: "document.doc_kind", Op: "eq", Value: "standard"},
				{Kind: "fact", Path: "document.doc_kind", Op: "eq", Value: "manual"},
			}}},
			right:          factDocument("document.doc_kind", "eq", "standard"),
			wantMayOverlap: true,
			wantAnalyzable: false,
			wantReason:     "unsupported_kind",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AnalyzeOverlap(tt.left, tt.right)
			if got.MayOverlap != tt.wantMayOverlap {
				t.Fatalf("MayOverlap = %v, want %v", got.MayOverlap, tt.wantMayOverlap)
			}
			if got.Analyzable != tt.wantAnalyzable {
				t.Fatalf("Analyzable = %v, want %v", got.Analyzable, tt.wantAnalyzable)
			}
			if got.Reason != tt.wantReason {
				t.Fatalf("Reason = %q, want %q", got.Reason, tt.wantReason)
			}
		})
	}
}

// TestAnalyzeOverlapNeverReportsFalseExceptGenuinelyDisjoint pins the
// conservative contract AnalyzeOverlap must hold: it may only ever report
// MayOverlap:false when it has proven the constrained value sets are
// disjoint. Every other analyzable or unanalyzable shape must return
// MayOverlap:true (P5 review 2026080302 finding P5-27: the analyzer must
// stay conservative -- over-reject at compile time, never under-reject).
func TestAnalyzeOverlapNeverReportsFalseExceptGenuinelyDisjoint(t *testing.T) {
	cases := []Document{
		allFacts(fact("document.doc_kind", "eq", "standard")),
		allFacts(fact("document.doc_kind", "eq", "standard"), fact("document.domain", "eq", "mechanical")),
		allFacts(fact("document.doc_kind", "in", []any{"standard", "manual"})),
		allFacts(fact("document.numeric_unit_density", "gt", 3)),
		Document{Version: 1, Expression: Predicate{Kind: "any", Items: []Predicate{
			{Kind: "fact", Path: "document.doc_kind", Op: "eq", Value: "standard"},
		}}},
		factDocument("document.doc_kind", "eq", "manual"),
	}
	for i, left := range cases {
		for j, right := range cases {
			got := AnalyzeOverlap(left, right)
			if !got.MayOverlap && got.Reason != "disjoint_path_values" {
				t.Fatalf("cases[%d] vs cases[%d]: MayOverlap=false with reason %q, want MayOverlap=true unless reason is disjoint_path_values", i, j, got.Reason)
			}
		}
	}
}

func allFacts(items ...Predicate) Document {
	return Document{Version: 1, Expression: Predicate{Kind: "all", Items: items}}
}

func fact(path, op string, value any) Predicate {
	return Predicate{Kind: "fact", Path: path, Op: op, Value: value}
}
