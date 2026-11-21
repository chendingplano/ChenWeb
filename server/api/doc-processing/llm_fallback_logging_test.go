package docprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

func TestReferenceExtractionSendsCorrectionInstructions(t *testing.T) {
	const incorrect = `{"references":[{"cited_raw":"Example standard","source_line_spans":["12"]},{"cited_raw":" ","source_line_spans":[]}]}`
	const corrected = `{"references":[{"cited_raw":"Example standard","source_line_spans":["12"]},{"cited_raw":"Example guide","source_line_spans":["13"]}]}`
	for _, tc := range []struct {
		name      string
		answer    string
		wantCalls int
		wantRows  int
	}{
		{"corrected", corrected, 2, 2},
		{"still invalid", `{"references":[{"cited_raw":" ","source_line_spans":[]}]}`, 1 + defaultStructuredContractRetries, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				requests = append(requests, string(body))
				answer := tc.answer
				if len(requests) == 1 {
					answer = incorrect
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"choices": []any{map[string]any{"message": map[string]string{"content": answer}}},
				})
			}))
			defer srv.Close()

			client := &llmclients.OpenAIJSONClient{BaseURL: srv.URL, APIKey: "test-key", HTTPClient: srv.Client()}
			in := llmclients.JSONExtractionInput{
				PromptText: "Extract references.", InputText: "12: Example standard\n13: Example guide", ModelName: "test-model",
			}
			payload, err := extractReferenceChunk(context.Background(), client, in)
			if err != nil {
				t.Fatal(err)
			}
			if len(requests) != tc.wantCalls {
				t.Fatalf("got %d requests, want %d", len(requests), tc.wantCalls)
			}
			for _, want := range []string{"test-model", "13: Example guide", "Reference 2", "cited_raw", "source_line_spans"} {
				if !strings.Contains(requests[1], want) {
					t.Errorf("retry request is missing %q", want)
				}
			}
			if got := len(parseRefMentions(payload)); got != tc.wantRows {
				t.Errorf("got %d usable references, want %d", got, tc.wantRows)
			}
		})
	}
}

func TestSummaryGenerationSendsCorrectionInstructions(t *testing.T) {
	const source = "11: Inspect the valve."
	const empty = `{"summary":" ","keywords":["valve"]}`
	const summary = "The document explains how to inspect a valve."
	for _, tc := range []struct {
		name        string
		answer      string
		wantCalls   int
		wantSummary string
	}{
		{"corrected", `{"summary":"` + summary + `"}`, 2, summary},
		{"still empty", empty, 1 + defaultStructuredContractRetries, source},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var correction string
			var calls int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				answer := empty
				if calls > 1 {
					body, _ := io.ReadAll(r.Body)
					correction = string(body)
					answer = tc.answer
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"choices": []any{map[string]any{"message": map[string]string{"content": answer}}},
				})
			}))
			defer srv.Close()
			service := FixedSizeChunkingService{
				Extractor: &llmclients.OpenAIJSONClient{BaseURL: srv.URL, APIKey: "test-key", HTTPClient: srv.Client()},
				Logger:    noopLogger{}, SummaryPromptText: "Summarize the document.", SummaryModelName: "test-model",
			}
			result, err := service.generateSummaryWithInputText(context.Background(), 1, 1, 1, source)
			if err != nil {
				t.Fatal(err)
			}
			if calls != tc.wantCalls || !strings.Contains(correction, "Set summary to a non-empty summary of the supplied document") {
				t.Fatalf("got %d requests, want %d with a correction on the second", calls, tc.wantCalls)
			}
			if result.Summary != tc.wantSummary {
				t.Fatalf("summary = %q, want %q", result.Summary, tc.wantSummary)
			}
			if tc.answer == empty && (len(result.Keywords) != 1 || result.Keywords[0] != "valve") {
				t.Fatalf("lost model keywords after exhausting retries: %v", result.Keywords)
			}
		})
	}
}

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
