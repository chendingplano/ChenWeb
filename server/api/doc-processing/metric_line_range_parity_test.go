package docprocessing

import (
	"database/sql"
	"encoding/json"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

// TestMetricLineRangeOverlapParity verifies that the SQL line_range overlap
// (kb.line_spans_to_int8multirange + the int8multirange && operator, added in
// project_migrations/20260616000001_add_line_range_to_kb_metrics.sql) produces the
// same line-overlap decisions as the Go implementation used today
// (lineSetFromSpans + spansOverlapLineSet). This is the parity gate for the
// "purest form" spike on kb.metrics — see
// KnowledgeStore/Capsules/coding-capsules/llm-wiki/artifact-connections.md §2.2.
//
// It runs only when CHENWEB_PG_TEST_DSN points at a Postgres with the migration
// applied; otherwise it skips so the default (sqlmock) suite is unaffected.
func TestMetricLineRangeOverlapParity(t *testing.T) {
	dsn := os.Getenv("CHENWEB_PG_TEST_DSN")
	if dsn == "" {
		t.Skip("CHENWEB_PG_TEST_DSN not set; skipping live-DB line_range parity test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Require the migration's function; if absent, skip with guidance rather than
	// silently passing.
	var exists bool
	if err := db.QueryRow(
		`SELECT EXISTS (SELECT 1 FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
		                WHERE n.nspname = 'kb' AND p.proname = 'line_spans_to_int8multirange')`,
	).Scan(&exists); err != nil {
		t.Fatalf("probe function: %v", err)
	}
	if !exists {
		t.Skip("kb.line_spans_to_int8multirange missing; run project migrations first")
	}

	// Span-array fixtures covering singles, colon/dash ranges, multi-element sets,
	// inclusive-end boundaries, and junk that the Go parser drops.
	fixtures := [][]string{
		{"150"},
		{"153", "157"},
		{"848:851"},
		{"10-12"},
		{"12"},
		{"13"},
		{"100:110", "200"},
		{"0", "-5", "x", "3:1"}, // all invalid -> empty set
		{},
		{"5", "12:14"},
	}

	overlapSQL := func(a, b []string) bool {
		aj, _ := json.Marshal(a)
		bj, _ := json.Marshal(b)
		var ov bool
		if err := db.QueryRow(
			`SELECT kb.line_spans_to_int8multirange($1::jsonb)
			     && kb.line_spans_to_int8multirange($2::jsonb)`,
			string(aj), string(bj),
		).Scan(&ov); err != nil {
			t.Fatalf("sql overlap(%v,%v): %v", a, b, err)
		}
		return ov
	}

	for i := range fixtures {
		for j := range fixtures {
			a, b := fixtures[i], fixtures[j]
			goOverlap := spansOverlapLineSet(a, lineSetFromSpans(b))
			sqlOverlap := overlapSQL(a, b)
			if goOverlap != sqlOverlap {
				t.Errorf("overlap mismatch a=%v b=%v: go=%v sql=%v", a, b, goOverlap, sqlOverlap)
			}
		}
	}
}
