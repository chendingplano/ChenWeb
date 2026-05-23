package docprocessing

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

type fakePhaseService struct {
	chunkCalls   int
	topicCalls   int
	summaryCalls int
}

func (f *fakePhaseService) HandleInput(_ context.Context, _ int64, _ string, _ []byte) error {
	f.chunkCalls++
	return nil
}

func (f *fakePhaseService) HandleGenerateTopicsInput(_ context.Context, _ int64, _ string, _ []byte) error {
	f.topicCalls++
	return nil
}

func (f *fakePhaseService) HandleGenerateSummariesInput(_ context.Context, _ int64, _ string, _ []byte) error {
	f.summaryCalls++
	return nil
}

func TestGenerateTopicsProcessor_PrefersTopicPhaseHandler(t *testing.T) {
	tmp := t.TempDir()
	lineFile := filepath.Join(tmp, "ocr_rslt_81_opendata.txt")
	if err := os.WriteFile(lineFile, []byte("1\t1\tparagraph\tF\t12\t[1,1,2,2]\tok\n"), 0o644); err != nil {
		t.Fatalf("write line file: %v", err)
	}

	service := &fakePhaseService{}
	processor := NewGenerateTopicsProcessor(&fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              81,
		ParserName:      "opendata",
		ResultFilename:  filepath.Join(tmp, "ocr_rslt_81.json"),
		StagingFilename: filepath.Join(tmp, "stdGk_3032174.pdf"),
		StatusRaw:       "[]",
	}}, service, nil)

	if err := processor.HandleEvent(context.Background(), []byte(`{"record_id":"81","line_file_filename":"`+lineFile+`"}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if service.topicCalls != 1 {
		t.Fatalf("topicCalls=%d, want 1", service.topicCalls)
	}
	if service.chunkCalls != 0 {
		t.Fatalf("chunkCalls=%d, want 0", service.chunkCalls)
	}
}

func TestGenerateSummariesProcessor_PrefersSummaryPhaseHandler(t *testing.T) {
	tmp := t.TempDir()
	lineFile := filepath.Join(tmp, "ocr_rslt_81_opendata.txt")
	if err := os.WriteFile(lineFile, []byte("1\t1\tparagraph\tF\t12\t[1,1,2,2]\tok\n"), 0o644); err != nil {
		t.Fatalf("write line file: %v", err)
	}

	service := &fakePhaseService{}
	processor := NewGenerateSummariesProcessor(&fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              81,
		ParserName:      "opendata",
		ResultFilename:  filepath.Join(tmp, "ocr_rslt_81.json"),
		StagingFilename: filepath.Join(tmp, "stdGk_3032174.pdf"),
		StatusRaw:       "[]",
	}}, service, nil)

	if err := processor.HandleEvent(context.Background(), []byte(`{"record_id":"81","line_file_filename":"`+lineFile+`"}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if service.summaryCalls != 1 {
		t.Fatalf("summaryCalls=%d, want 1", service.summaryCalls)
	}
	if service.chunkCalls != 0 {
		t.Fatalf("chunkCalls=%d, want 0", service.chunkCalls)
	}
}
