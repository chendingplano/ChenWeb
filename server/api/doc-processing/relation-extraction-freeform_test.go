package docprocessing

import (
	"encoding/json"
	"testing"
)

func TestNormalizeRelationRows_CapturesV2EndpointLines(t *testing.T) {
	raw := []any{
		map[string]any{
			"subject": "Acme", "predicate": "supplies", "object": "Globex",
			"subject_lines": []any{"10"}, "predicate_lines": []any{"12"}, "object_lines": []any{"20-21"},
		},
	}
	rows := normalizeRelationRows(raw, 0)
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	r := rows[0]
	if got := toStringSlice(r["subject_lines"]); len(got) != 1 || got[0] != "10" {
		t.Errorf("subject_lines = %v, want [10]", got)
	}
	if got := toStringSlice(r["object_lines"]); len(got) != 1 || got[0] != "20-21" {
		t.Errorf("object_lines = %v, want [20-21]", got)
	}
	// No top-level "lines" -> line_spans is the union of endpoint lines.
	spans := toStringSlice(r["line_spans"])
	if len(spans) != 3 || spans[0] != "10" || spans[1] != "12" || spans[2] != "20-21" {
		t.Errorf("line_spans = %v, want [10 12 20-21] (union)", spans)
	}
}

func TestBuildChunkRelationInputJSON_FlagsAndFields(t *testing.T) {
	lines := []MarkedLine{
		{Mark: "o", Line: Line{LineNo: 9, PageNo: 1, LineType: "text", Content: "prev-overlap"}},
		{Mark: "r", Line: Line{LineNo: 10, PageNo: 2, LineType: "", Content: "in-chunk"}},
	}
	out := buildChunkRelationInputJSON(lines)
	var got []map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(got) != 2 {
		t.Fatalf("got %d line objects, want 2", len(got))
	}
	if got[0]["flag"] != "o" {
		t.Errorf("overlap-marked line flag = %v, want o", got[0]["flag"])
	}
	if got[1]["flag"] != "n" {
		t.Errorf("regular line flag = %v, want n", got[1]["flag"])
	}
	if got[1]["line_type"] != "text" {
		t.Errorf("empty line_type should default to text, got %v", got[1]["line_type"])
	}
	if int(got[1]["line_number"].(float64)) != 10 {
		t.Errorf("line_number = %v, want 10", got[1]["line_number"])
	}
}
