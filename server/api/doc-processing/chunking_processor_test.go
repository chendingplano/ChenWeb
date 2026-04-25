package docprocessing

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeChunkingProcessorService struct {
	err error
}

func (f fakeChunkingProcessorService) HandleInput(_ context.Context, _ int64, _ string, _ []byte) error {
	return f.err
}

func TestChunkingProcessor_HandleEvent_ReturnsServiceError(t *testing.T) {
	tmp := t.TempDir()
	lineFile := filepath.Join(tmp, "ocr_rslt_81_opendata.txt")
	if err := os.WriteFile(lineFile, []byte("1\t1\tparagraph\tF\t12\t[1,1,2,2]\tok\n"), 0o644); err != nil {
		t.Fatalf("write line file: %v", err)
	}

	store := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              81,
		ParserName:      "opendata",
		ResultFilename:  filepath.Join(tmp, "ocr_rslt_81.json"),
		StagingFilename: filepath.Join(tmp, "stdGk_3032174.pdf"),
		StatusRaw:       "[]",
	}}
	wantErr := errors.New("semantic chunking exploded")

	p := NewChunkingProcessor(store, fakeChunkingProcessorService{err: wantErr}, nil)

	err := p.HandleEvent(context.Background(), []byte(`{"record_id":"81","line_file_filename":"`+lineFile+`"}`))
	if err == nil {
		t.Fatalf("expected HandleEvent to return service error")
	}
	if !strings.Contains(err.Error(), wantErr.Error()) {
		t.Fatalf("error=%v, want to contain %q", err, wantErr.Error())
	}
}
