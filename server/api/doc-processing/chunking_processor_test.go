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

func TestChunkingProcessor_HandleEvent_BlockInputFallbackWritesChunks(t *testing.T) {
	t.Setenv("EMBEDDING_MODEL_NAME", "test-summary-embed-model")
	tmp := t.TempDir()
	summaryRoot := t.TempDir()

	store := &fakeStore{rec: InputRecord{
		ID:              81,
		ParserName:      "opendata",
		StagingFilename: filepath.Join(tmp, "stdGk_3032174.pdf"),
		StatusRaw:       "[]",
	}}
	extractor := &fakeSemanticExtractor{
		outs: []map[string]any{
			{
				"topics": []any{
					map[string]any{
						"topic_type":    "policy",
						"lines":         []any{"1"},
						"keywords":      []any{"intro"},
						"topic":         "Intro",
						"category_path": []any{"document_overview"},
					},
				},
			},
		},
	}
	service := NewFixedSizeChunkingService(store, extractor, nil)
	service.ChunkDir = tmp
	service.ArtifactWebDir = summaryRoot
	service.ChunkSize = 100
	service.OverlapPercent = 0
	service.ModelErr = nil
	service.PromptErr = nil
	service.SummaryModelErr = nil
	service.SummaryPromptErr = nil
	service.GenerateSummary = func(_ context.Context, _ int64, level int, seqNo int, _ []MarkedLine, _ []SummaryItem) (summaryGenerateResult, error) {
		return summaryGenerateResult{Summary: "summary " + asString(level) + "-" + asString(seqNo), CategoryPaths: []string{"legacy_summary_tree"}}, nil
	}

	processor := NewChunkingProcessor(&fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              81,
		ParserName:      "opendata",
		ResultFilename:  filepath.Join(tmp, "ocr_rslt_81.json"),
		StagingFilename: filepath.Join(tmp, "stdGk_3032174.pdf"),
		StatusRaw:       "[]",
	}}, service, nil)

	ctx, holder := withBlockBufferHolder(context.Background())
	holder.buffer = &BlockBuffer{
		Blocks: []Block{
			{
				Index: 1,
				Lines: []BlockLine{
					{Flag: "n", LineNumber: 1, PageNumber: 1, LineType: "paragraph", Content: "Intro"},
				},
			},
		},
	}

	err := processor.HandleEvent(ctx, []byte(`{"record_id":"81","operation":"chunking"}`))
	if err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}

	chunkPath := filepath.Join(tmp, "0", "81", "stdGk_3032174_opendata.chunks")
	if _, err := os.Stat(chunkPath); err != nil {
		t.Fatalf("missing chunk artifact: %v", err)
	}
}
