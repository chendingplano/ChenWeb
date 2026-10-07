package fileconverters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/documentgeometry"
)

type geometryPublisher struct {
	t     *testing.T
	calls int
}

func (p *geometryPublisher) Publish(_ context.Context, _ string, payload []byte) error {
	p.t.Helper()
	var event LineFileGeneratedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		p.t.Fatal(err)
	}
	raw, err := os.ReadFile(documentgeometry.CompanionPath(event.LineFileFilename))
	if err != nil {
		p.t.Fatal("geometry must exist before publish", err)
	}
	var geometry documentgeometry.Document
	if err = json.Unmarshal(raw, &geometry); err != nil {
		p.t.Fatal(err)
	}
	if len(geometry.Tables) != 1 || geometry.Tables[0].Line != 1 || len(geometry.Tables[0].Rows) != 2 {
		p.t.Fatalf("geometry=%+v", geometry)
	}
	p.calls++
	return nil
}

func TestConversionBuildsGeometryBeforePublishing(t *testing.T) {
	t.Setenv("DOC_PROCESSOR_MODE", "auto")
	dir := t.TempDir()
	pdf := filepath.Join(dir, "doc.pdf")
	data := []byte("PDF fixture")
	sum := sha256.Sum256(data)
	if err := os.WriteFile(pdf, data, 0600); err != nil {
		t.Fatal(err)
	}
	parser := `{"pages":[{"page_number":1,"items":[{"type":"table","bbox":[100,100,900,200],"table_body":"<table><tr><td>Type</td><td>Limit</td></tr><tr><td>Machine</td><td>Energy 30</td></tr></table>"}]}]}`
	if err := os.WriteFile(filepath.Join(dir, "doc_mineru.json"), []byte(parser), 0600); err != nil {
		t.Fatal(err)
	}
	physical := documentgeometry.PhysicalDocument{Version: 1, Extractor: "pymupdf-tables-v1", PDFHash: hex.EncodeToString(sum[:]), Tables: []documentgeometry.PhysicalTable{
		{Page: 1, Coords: []float64{100, 100, 900, 200}, Rows: []documentgeometry.PhysicalRow{
			{Coords: []float64{100, 100, 900, 120}, Cells: []documentgeometry.PhysicalCell{{Text: "Type", Coords: []float64{100, 100, 200, 120}}, {Text: "Limit", Coords: []float64{200, 100, 900, 120}}}},
			{Coords: []float64{100, 120, 900, 200}, Cells: []documentgeometry.PhysicalCell{{Text: "Machine", Coords: []float64{100, 120, 200, 200}}, {Text: "Energy 30", Coords: []float64{200, 120, 900, 200}}}},
		}}}}
	raw, _ := json.Marshal(physical)
	if err := os.WriteFile(documentgeometry.PhysicalPath(pdf), raw, 0600); err != nil {
		t.Fatal(err)
	}
	store := &fakeStore{rec: InputRecord{ID: 416, Type: "pdf", FileName: pdf, UserID: "test-tenant", StatusRaw: `[{"operation":"parsed","proc_status":"success"}]`}}
	service := NewService(store, slog.Default())
	publisher := &geometryPublisher{t: t}
	service.Publisher = publisher
	if err := service.HandleRequest(context.Background(), ConvertRequest{RecordID: 416}); err != nil {
		t.Fatal(err)
	}
	if publisher.calls != 1 {
		t.Fatalf("publishes=%d", publisher.calls)
	}
	// A rerun must ignore the new geometry companion as a parser output.
	if err := service.HandleRequest(context.Background(), ConvertRequest{RecordID: 416}); err != nil {
		t.Fatal(err)
	}
	if publisher.calls != 2 {
		t.Fatalf("rerun publishes=%d", publisher.calls)
	}
}
