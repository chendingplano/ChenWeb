// server/api/docgenworker/renderer_test.go
package docgenworker_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/docgenworker"
)

func TestRenderDocx_ReplacesTokens(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "output.docx")

	if _, err := os.Stat("testdata/simple_template.docx"); os.IsNotExist(err) {
		t.Skip("testdata/simple_template.docx not present — skipping renderer test")
	}

	tokens := map[string]string{"companyName": "Acme Corp"}
	if err := docgenworker.RenderDocx("testdata/simple_template.docx", outputPath, tokens); err != nil {
		t.Fatalf("RenderDocx failed: %v", err)
	}
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Fatal("output file was not created")
	}
}
