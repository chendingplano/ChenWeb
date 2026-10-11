package docprocessing

import (
	"reflect"
	"strings"
	"testing"
)

func TestReferenceTableRowsDistinctOccurrences(t *testing.T) {
	for _, form := range []string{"listed", "in_text"} {
		t.Run(form, func(t *testing.T) {
			row := func(id string) any {
				return map[string]any{
					"cited_raw": "ISO 9001", "mechanism": "standard_number", "ref_form": form,
					"list_kind": "normative", "source_line_spans": []any{"116"},
					"source_table_rows": []any{"116#" + id},
				}
			}
			raw := parseRefMentions(map[string]any{"references": []any{row("r1"), row("r2"), row("r1")}})
			got := assembleRefMentions(753, raw)
			if len(got) != 2 {
				t.Fatalf("got %d mentions, want two distinct table rows with overlap deduplicated", len(got))
			}
			if got[0].MentionKey == got[1].MentionKey {
				t.Fatal("distinct table rows have the same mention key")
			}
		})
	}
}

func TestReferenceTableRowsEvidenceAndHashes(t *testing.T) {
	lines := []Line{
		{LineNo: 115, LineType: "table-caption", Content: "Table 1 Standards"},
		{LineNo: 116, LineType: "table", Content: `<table><tr><th>Item</th><th>Standard</th></tr><tr><td>Fan</td><td>ISO 9001</td></tr><tr><td>Pump</td><td>ISO 9001</td></tr><tr><td>Motor</td><td>ISO 14001</td></tr></table>`},
		{LineNo: 117, LineType: "paragraph", Content: "See the standards above."},
	}
	cases := []struct {
		name         string
		spans        []string
		refs         []any
		cited        string
		wantRows     []string
		wantEvidence []string
	}{
		{"multiple rows and header", []string{"116"}, []any{"116#h0", "116#r1", "116#r2", "116#r99", "117#r1"}, "ISO 9001", []string{"h0", "r1", "r2"}, []string{"Table 1 Standards", "[h0]", "[r1] Item: Fan", "[r2] Item: Pump"}},
		{"evidence fallback", []string{"116"}, nil, "ISO 14001", []string{"r3"}, []string{"[r3] Item: Motor"}},
		{"mixed prose", []string{"116-117"}, []any{"116#r1"}, "ISO 9001", []string{"r1"}, []string{"See the standards above.", "[r1] Item: Fan"}},
		{"prose rejects row refs", []string{"117"}, []any{"116#r1"}, "ISO 9001", nil, []string{"See the standards above."}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			refs := tableRowRefsFromValue(c.refs)
			for i := range refs {
				refs[i].RowHash = map[string]string{"r1": "untrusted model hash"}
			}
			ms := []RefMention{{CitedRaw: c.cited, SourceLineSpans: c.spans, SourceTableRows: refs, Evidence: "See the standards above."}}
			applyReferenceTableRows(lines, ms, nil, 753)
			var rows []string
			for _, ref := range ms[0].SourceTableRows {
				rows = append(rows, ref.Rows...)
				for _, id := range ref.Rows {
					if len(ref.RowHash[id]) != 12 || ref.RowHash[id] == "untrusted model hash" {
						t.Fatalf("row %s has no system hash: %+v", id, ref)
					}
				}
			}
			if !reflect.DeepEqual(rows, c.wantRows) {
				t.Fatalf("rows=%v want %v", rows, c.wantRows)
			}
			for _, want := range c.wantEvidence {
				if !strings.Contains(ms[0].Evidence, want) {
					t.Fatalf("evidence %q missing %q", ms[0].Evidence, want)
				}
			}
			if c.name == "multiple rows and header" && strings.Contains(ms[0].Evidence, "Motor") {
				t.Fatal("evidence contains unrelated row")
			}
		})
	}
}

func TestReferenceTableRowsLargeTableUnmatched(t *testing.T) {
	html := "<table><tr><th>Item</th></tr>"
	for i := 0; i < 6; i++ {
		html += "<tr><td>Unrelated</td></tr>"
	}
	html += "</table>"
	ms := []RefMention{{CitedRaw: "ISO 9001", SourceLineSpans: []string{"116"}, SourceTableRows: []TableRowRef{{Line: 116, Rows: []string{"r99"}}}, Evidence: "Original evidence"}}
	applyReferenceTableRows([]Line{{LineNo: 116, LineType: "table", Content: html}}, ms, nil, 753)
	if len(ms[0].SourceTableRows) != 0 || ms[0].Evidence != "Original evidence" {
		t.Fatalf("unmatched rows must retain original evidence and no provenance: %+v", ms[0])
	}
}

func TestReferenceTableRowsKeyIgnoresHashesAndOrder(t *testing.T) {
	a := RefMention{InputRecordID: 753, SourceLineSpans: []string{"116"}, CitedRaw: "ISO 9001",
		SourceTableRows: []TableRowRef{{Line: 116, Rows: []string{"r2", "r1"}, RowHash: map[string]string{"r1": "old"}}}}
	b := a
	b.SourceTableRows = []TableRowRef{{Line: 116, Rows: []string{"r1", "r2"}, RowHash: map[string]string{"r1": "new"}}}
	if refMentionKey(a) != refMentionKey(b) {
		t.Fatal("row order and content hashes must not change occurrence identity")
	}
}
