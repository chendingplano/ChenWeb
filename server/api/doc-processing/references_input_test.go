package docprocessing

import "testing"

func TestReferencesChunkInputMatchesPromptVersion(t *testing.T) {
	lines := []MarkedLine{{Mark: "o", Line: Line{LineNo: 7, PageNo: 2, LineType: "paragraph", Content: "ISO 9001"}}}
	p := &ReferencesProcessor{PromptRef: "prompt-extract-references-v2.md"}
	want := `{"doc_context":"doc","lines":[["flag","line_number","page_number","line_type","content"],["o",7,2,"paragraph","ISO 9001"]]}`
	if got := p.referenceChunkInputText(lines, "doc"); got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
	p.PromptRef = "prompt-extract-references-v1.md"
	if got := p.referenceChunkInputText(lines, "doc"); got != canonicalChunkInputText(lines, "doc") {
		t.Fatalf("legacy input changed: %s", got)
	}
}
