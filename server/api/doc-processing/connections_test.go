package docprocessing

import (
	"reflect"
	"testing"
)

func TestBlockLinesToSpans(t *testing.T) {
	cases := []struct {
		name string
		in   []int
		want []string
	}{
		{"empty", nil, []string{}},
		{"single", []int{5}, []string{"5"}},
		{"contiguous run", []int{10, 11, 12}, []string{"10:12"}},
		{"unsorted + duplicates", []int{12, 10, 11, 11}, []string{"10:12"}},
		{"disjoint runs", []int{1, 2, 5, 7, 8, 9}, []string{"1:2", "5", "7:9"}},
		{"drops non-positive", []int{0, -3, 4}, []string{"4"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lines := make([]BlockLine, len(c.in))
			for i, n := range c.in {
				lines[i] = BlockLine{LineNumber: n}
			}
			got := blockLinesToSpans(lines)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("blockLinesToSpans(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestArtifactRefFromRegistry(t *testing.T) {
	// String spans (metric/provision form).
	r := artifactRefFromRegistry("metric", "42_mtc_1", []byte(`["3","5:6"]`))
	if r.Type != "metric" || r.ID != "42_mtc_1" {
		t.Fatalf("unexpected type/id: %q / %q", r.Type, r.ID)
	}
	if !reflect.DeepEqual(r.Spans, []string{"3", "5:6"}) {
		t.Errorf("unexpected spans: %v", r.Spans)
	}

	// Numeric line list (summary/topic form) collapses to merged spans.
	r2 := artifactRefFromRegistry("topic", "7_tpc_2", []byte(`[3,4,5]`))
	if !reflect.DeepEqual(r2.Spans, []string{"3:5"}) {
		t.Errorf("unexpected merged spans: %v", r2.Spans)
	}

	// Empty / null spans yield no spans (and therefore no edges downstream).
	r3 := artifactRefFromRegistry("topic", "x", []byte(`[]`))
	if len(r3.Spans) != 0 {
		t.Errorf("expected no spans, got %v", r3.Spans)
	}
	r4 := artifactRefFromRegistry("topic", "x", nil)
	if len(r4.Spans) != 0 {
		t.Errorf("expected no spans for nil, got %v", r4.Spans)
	}
}

func TestEncodeJSONB(t *testing.T) {
	// nil-ish values encode to nil so the column stays NULL.
	if v, err := encodeJSONB(nil); err != nil || v != nil {
		t.Fatalf("nil should encode to nil, got %v / %v", v, err)
	}
	if v, err := encodeJSONB((*OverlapInfo)(nil)); err != nil || v != nil {
		t.Fatalf("typed-nil overlap should encode to nil, got %v / %v", v, err)
	}
	if v, err := encodeJSONB(map[string]any{}); err != nil || v != nil {
		t.Fatalf("empty map should encode to nil, got %v / %v", v, err)
	}

	v, err := encodeJSONB(&OverlapInfo{OverlapCount: 3, OverlapLines: []int{3, 5, 6}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, ok := v.([]byte)
	if !ok {
		t.Fatalf("expected []byte, got %T", v)
	}
	if string(b) != `{"overlap_count":3,"overlap_lines":[3,5,6]}` {
		t.Errorf("unexpected json: %s", b)
	}
}

func chunk(index int, fromLine, toLine int) Block {
	lines := make([]BlockLine, 0, toLine-fromLine+1)
	for n := fromLine; n <= toLine; n++ {
		lines = append(lines, BlockLine{Flag: "n", LineNumber: n})
	}
	return Block{Index: index, Lines: lines}
}

func TestDeriveLineOverlapConnections_singleChunkSingleTarget(t *testing.T) {
	chunks := []Block{chunk(1, 1, 10)}
	targets := []ArtifactRef{{Type: "metric", ID: "42_1", Spans: []string{"3", "5:6"}}}

	got := DeriveLineOverlapConnections(42, "metric", "has-metrics", chunks, targets)

	if len(got) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(got))
	}
	c := got[0]
	if c.SourceType != "chunk" || c.SourceID != "42_1" {
		t.Errorf("unexpected source: %q / %q", c.SourceType, c.SourceID)
	}
	if c.TargetType != "metric" || c.TargetID != "42_1" {
		t.Errorf("unexpected target: %q / %q", c.TargetType, c.TargetID)
	}
	if c.RelationName != "has-metrics" || c.RelationMethod != "line_overlap" {
		t.Errorf("unexpected relation: %q / %q", c.RelationName, c.RelationMethod)
	}
	if c.SourceRecordID != 42 || c.TargetRecordID != 42 {
		t.Errorf("unexpected record ids: source=%d target=%d", c.SourceRecordID, c.TargetRecordID)
	}
	if c.SourceDesc != "chunk:42_1" || c.TargetDesc != "metric:42_1" {
		t.Errorf("unexpected endpoint descs: %q / %q", c.SourceDesc, c.TargetDesc)
	}
	if c.Overlap == nil || c.Overlap.OverlapCount != 3 {
		t.Fatalf("expected overlap count 3, got %+v", c.Overlap)
	}
	if !reflect.DeepEqual(c.Overlap.OverlapLines, []int{3, 5, 6}) {
		t.Errorf("unexpected overlap lines: %v", c.Overlap.OverlapLines)
	}
}

func TestDeriveLineOverlapConnections_noOverlapNoEdge(t *testing.T) {
	chunks := []Block{chunk(1, 1, 10)}
	targets := []ArtifactRef{{Type: "topic", ID: "t1", Spans: []string{"20:25"}}}

	got := DeriveLineOverlapConnections(7, "topic", "has-topic", chunks, targets)

	if len(got) != 0 {
		t.Fatalf("expected no connections, got %d", len(got))
	}
}

func TestDeriveLineOverlapConnections_spanAcrossTwoChunks(t *testing.T) {
	chunks := []Block{chunk(1, 1, 10), chunk(2, 11, 20)}
	targets := []ArtifactRef{{Type: "provision", ID: "p1", Spans: []string{"9:12"}}}

	got := DeriveLineOverlapConnections(5, "provision", "has-provision", chunks, targets)

	if len(got) != 2 {
		t.Fatalf("expected 2 connections (one per chunk), got %d", len(got))
	}
	bySource := map[string]*OverlapInfo{}
	for i := range got {
		bySource[got[i].SourceID] = got[i].Overlap
	}
	if o := bySource["5_1"]; o == nil || !reflect.DeepEqual(o.OverlapLines, []int{9, 10}) {
		t.Errorf("chunk 1 overlap wrong: %+v", o)
	}
	if o := bySource["5_2"]; o == nil || !reflect.DeepEqual(o.OverlapLines, []int{11, 12}) {
		t.Errorf("chunk 2 overlap wrong: %+v", o)
	}
}

func TestDeriveLineOverlapConnections_skipsEmptySpans(t *testing.T) {
	chunks := []Block{chunk(1, 1, 10)}
	targets := []ArtifactRef{{Type: "metric", ID: "m1", Spans: nil}}

	got := DeriveLineOverlapConnections(1, "metric", "has-metrics", chunks, targets)

	if len(got) != 0 {
		t.Fatalf("expected no connections for target without spans, got %d", len(got))
	}
}
