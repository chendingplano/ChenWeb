package docprocessing

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

type fakeSummaryLogExtractor struct {
	result map[string]any
	err    error
}

func (f *fakeSummaryLogExtractor) ExtractJSON(_ context.Context, _ llmclients.JSONExtractionInput) (map[string]any, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func TestFixedSizeChunkingServiceGenerateSummary_WritesGenerateSummaryLog(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	mock.ExpectExec("INSERT INTO kb\\.doc_proc_logs").
		WithArgs(
			"generate summary",
			"generate_summary",
			"{summary-model}",
			"summary-prompt",
			int64(42),
			nil,
			EntryTypeGenerateSummary,
			nil,
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			nil,
			nil,
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	logger := &fakeLogger{}
	svc := &FixedSizeChunkingService{
		Extractor:         &fakeSummaryLogExtractor{result: map[string]any{"summary": "Hello world", "keywords": []any{"hello"}}},
		Logger:            logger,
		SummaryModelName:  "summary-model",
		SummaryPromptRef:  "summary-prompt",
		SummaryPromptText: "prompt body",
		ProcLogger:        DocProcLogger{DB: db},
	}

	_, err = svc.generateSummary(context.Background(), 42, 0, 3, []MarkedLine{
		{Line: Line{LineNo: 1, Content: "A"}, Mark: "r"},
		{Line: Line{LineNo: 2, Content: "B"}, Mark: "r"},
	}, nil)
	if err != nil {
		t.Fatalf("generateSummary: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}
