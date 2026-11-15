package docprocessing

import (
	"os"
	"path/filepath"
	"testing"
)

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

// The deployed fallback must explain exactly the same format as the disk prompt.
func TestCompactExtractionEmbeddedPromptsMatchFiles(t *testing.T) {
	for _, key := range []string{"EXTRACT_REFERENCES_PROMPT", "EXTRACT_METRICS_SIMPLE_PROMPT"} {
		prompt, ok := embeddedPromptDefaults[key]
		if !ok {
			t.Fatalf("missing embedded prompt %s", key)
		}
		disk, err := os.ReadFile(filepath.Join("..", "..", "..", "prompts", prompt.Ref))
		if err != nil {
			t.Fatal(err)
		}
		if string(disk) != prompt.Text {
			t.Errorf("%s embedded and disk prompts differ", key)
		}
	}
}
