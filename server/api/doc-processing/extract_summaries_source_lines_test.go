package docprocessing

import (
	"reflect"
	"testing"
)

// source_line_spans must hold the chunk's full line set (overlap + regular), not
// only the lines cited by selected sentences; those go in source_sentence_spans.
func TestExtractSummariesStoresChunkLineSetAsSourceLineSpans(t *testing.T) {
	p, _, sink, ctx := exSummaryFixture(t)
	storeChunksInContext(ctx, []Chunk{{SeqNo: 2, Lines: []MarkedLine{
		{Line: Line{LineNo: 6, Content: "Overlap context line.", LineType: "text"}, Mark: "o"},
		{Line: Line{LineNo: 7, Content: "Storage shall remain below 8°C.", LineType: "text"}, Mark: "n"},
	}}})
	if err := p.HandleEvent(ctx, []byte(`{"record_id":42}`)); err != nil {
		t.Fatal(err)
	}
	if len(sink.items) != 1 {
		t.Fatalf("items=%+v", sink.items)
	}
	if got, want := sink.items[0].SourceLineNos, []int{6, 7}; !reflect.DeepEqual(got, want) {
		t.Fatalf("SourceLineNos=%v, want %v", got, want)
	}
	if cited := sink.items[0].Sentences[0].Spans[0].Line; cited != 6 {
		t.Fatalf("cited line=%d, want 6 (evidence stays in sentence spans)", cited)
	}
}
