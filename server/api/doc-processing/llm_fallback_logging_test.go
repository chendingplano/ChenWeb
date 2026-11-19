package docprocessing

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

func TestReferencesRetryBeforeFallback(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		retryErr, fallbackErr error
		wantCalls             []string
		wantAlerts            int
	}{
		{"retry succeeds", nil, nil, []string{"primary", "primary"}, 1},
		{"fallback succeeds", context.DeadlineExceeded, nil, []string{"primary", "primary", "fallback"}, 0},
		{"fallback fails", context.DeadlineExceeded, errors.New("fallback unavailable"), []string{"primary", "primary", "fallback"}, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			ext := &referenceFallbackTestExtractor{errs: []error{context.DeadlineExceeded, tc.retryErr, tc.fallbackErr}}
			p := &ReferencesProcessor{
				DB: db, Extractor: ext, FallbackExtractor: ext, Logger: noopLogger{},
				ModelName: "primary", FallbackModelName: "fallback", batchRecordID: 441, batchChunks: make([]Chunk, 29),
			}
			if tc.retryErr != nil && tc.fallbackErr == nil {
				mock.ExpectExec("INSERT INTO kb.failed_llm_calls").
					WithArgs(int64(441), int64(3941), "extract_references", "", "primary", "fallback", context.DeadlineExceeded.Error(), "", 28).
					WillReturnResult(sqlmock.NewResult(0, 1))
			}
			err = p.ProcessChunk(withLLMRunID(context.Background(), 3941), 28)
			if (err != nil) != (tc.fallbackErr != nil) {
				t.Fatalf("ProcessChunk error = %v", err)
			}
			if len(ext.models) != len(tc.wantCalls) {
				t.Fatalf("models = %v, want %v", ext.models, tc.wantCalls)
			}
			for i, model := range tc.wantCalls {
				if model != ext.models[i] {
					t.Fatalf("models = %v, want %v", ext.models, tc.wantCalls)
				}
			}
			if ext.alerts != tc.wantAlerts {
				t.Fatalf("attempt alerts = %d, want %d", ext.alerts, tc.wantAlerts)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type referenceFallbackTestExtractor struct {
	errs   []error
	models []string
	alerts int
}

func (e *referenceFallbackTestExtractor) ExtractJSON(ctx context.Context, in llmclients.JSONExtractionInput) (map[string]any, error) {
	err := e.errs[len(e.models)]
	e.models = append(e.models, in.ModelName)
	if err != nil {
		llmclients.ReportCallAlert(ctx, func() { e.alerts++ })
		return nil, err
	}
	return map[string]any{"references": []any{}}, nil
}
