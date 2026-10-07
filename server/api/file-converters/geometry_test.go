package fileconverters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/documentgeometry"
)

func TestConverterExtractsRealPDFGeometryWithoutPhysicalCompanion(t *testing.T) {
	python, _, err := tableGeometryCommand()
	if err != nil {
		t.Skipf("Python geometry environment unavailable: %v", err)
	}
	if err = exec.Command(python, "-c", "import fitz").Run(); err != nil {
		t.Skip("PyMuPDF unavailable")
	}
	// Exercise discovery from the same directory as `go run .` for the service.
	t.Chdir("../../cmd/parser-result-converter")
	dir := t.TempDir()
	pdf := filepath.Join(dir, "doc.pdf")
	code := `import fitz,sys
with fitz.open() as doc:
 p=doc.new_page(width=300,height=400)
 for x in (40,120,240): p.draw_line((x,40),(x,100))
 for y in (40,70,100): p.draw_line((40,y),(240,y))
 for x,y,text in [(45,60,'Type'),(125,60,'Limit'),(45,90,'Machine'),(125,90,'Energy 30')]: p.insert_text((x,y),text,fontsize=10)
 doc.save(sys.argv[1])`
	if output, err := exec.Command(python, "-c", code, pdf).CombinedOutput(); err != nil {
		t.Fatalf("generate PDF: %v %s", err, output)
	}
	parser := `{"pages":[{"page_number":1,"items":[{"type":"table","bbox":[133,100,800,250],"table_body":"<table><tr><td>Type</td><td>Limit</td></tr><tr><td>Machine</td><td>Energy 30</td></tr></table>"}]}]}`
	if err = os.WriteFile(filepath.Join(dir, "doc_mineru.json"), []byte(parser), 0600); err != nil {
		t.Fatal(err)
	}
	store := &fakeStore{rec: InputRecord{ID: 416, Type: "pdf", FileName: pdf, StatusRaw: `[{"operation":"parsed","proc_status":"success"}]`}}
	service := NewService(store, slog.Default())
	operations := []string{}
	if err = service.HandleRequest(context.Background(), ConvertRequest{RecordID: 416, Force: boolPtr(true), Operations: &operations}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{documentgeometry.PhysicalPath(pdf), documentgeometry.CompanionPath(filepath.Join(dir, "doc_mineru.txt"))} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		// Both physical and canonical serialization must contain integer boxes.
		var data any
		if err = json.Unmarshal(raw, &data); err != nil {
			t.Fatal(err)
		}
		var visit func(any)
		visit = func(value any) {
			switch v := value.(type) {
			case map[string]any:
				for key, child := range v {
					if key == "coords" && child != nil {
						for _, n := range child.([]any) {
							if n.(float64) != float64(int(n.(float64))) {
								t.Fatalf("noninteger coordinate in %s", path)
							}
						}
					}
					visit(child)
				}
			case []any:
				for _, child := range v {
					visit(child)
				}
			}
		}
		visit(data)
	}
	raw, _ := os.ReadFile(documentgeometry.CompanionPath(filepath.Join(dir, "doc_mineru.txt")))
	var geometry documentgeometry.Document
	if err = json.Unmarshal(raw, &geometry); err != nil {
		t.Fatal(err)
	}
	if len(geometry.Tables) != 1 || len(geometry.Tables[0].Rows) != 2 {
		t.Fatalf("missing table row geometry: %+v", geometry)
	}
}

func TestGeometryExtractionFailureIsLoggedAndPreservesLineConversion(t *testing.T) {
	dir := t.TempDir()
	pdf := filepath.Join(dir, "doc.pdf")
	if err := os.WriteFile(pdf, []byte("PDF"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "doc_mineru.json"), []byte(`{"pages":[{"page_number":1,"items":[{"type":"text","text":"Hello","bbox":[1,2,3,4]}]}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	store := &fakeStore{rec: InputRecord{ID: 1, Type: "pdf", FileName: pdf, StatusRaw: `[{"operation":"parsed","proc_status":"success"}]`}}
	var logs bytes.Buffer
	service := NewService(store, slog.New(slog.NewTextHandler(&logs, nil)))
	service.ExtractTableGeometry = func(context.Context, string) error { return errors.New("PyMuPDF unavailable") }
	operations := []string{}
	if err := service.HandleRequest(context.Background(), ConvertRequest{RecordID: 1, Operations: &operations}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "doc_mineru.txt")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "table geometry companion unavailable") || !strings.Contains(logs.String(), "PyMuPDF unavailable") {
		t.Fatalf("missing diagnostic: %s", logs.String())
	}
	if _, err := os.Stat(documentgeometry.CompanionPath(filepath.Join(dir, "doc_mineru.txt"))); !os.IsNotExist(err) {
		t.Fatalf("unexpected companion: %v", err)
	}
}
