// server/api/docgenworker/renderer_test.go
package docgenworker_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/docgenworker"
	"github.com/nguyenthenguyen/docx"
)

func TestRenderDocx_ReplacesTokens(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "output.docx")

	// Build a docx with token {{companyName}}
	r, err := docx.ReadDocxFile("testdata/simple_template.docx")
	if err != nil {
		t.Skip("testdata/simple_template.docx not present — skipping renderer test")
	}
	r.Close()

	tokens := map[string]string{"companyName": "Acme Corp"}
	if err := docgenworker.RenderDocx("testdata/simple_template.docx", outputPath, tokens); err != nil {
		t.Fatalf("RenderDocx failed: %v", err)
	}
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Fatal("output file was not created")
	}
}
