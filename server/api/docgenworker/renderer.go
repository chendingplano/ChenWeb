// server/api/docgenworker/renderer.go
package docgenworker

import (
	"fmt"

	"github.com/nguyenthenguyen/docx"
)

// RenderDocx opens templatePath, replaces every {{key}} placeholder using tokens,
// and writes the result to outputPath.
func RenderDocx(templatePath, outputPath string, tokens map[string]string) error {
	r, err := docx.ReadDocxFile(templatePath)
	if err != nil {
		return fmt.Errorf("failed to open template %q (CWB_DGW_020): %w", templatePath, err)
	}
	defer r.Close()

	doc := r.Editable()
	for key, val := range tokens {
		if err := doc.Replace("{{"+key+"}}", val, -1); err != nil {
			return fmt.Errorf("failed to replace token %q (CWB_DGW_025): %w", key, err)
		}
	}
	if err := doc.WriteToFile(outputPath); err != nil {
		return fmt.Errorf("failed to write output %q (CWB_DGW_030): %w", outputPath, err)
	}
	return nil
}
