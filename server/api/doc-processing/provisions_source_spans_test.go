package docprocessing

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestProvisionsRejectsUnusableSourceSpans(t *testing.T) {
	for _, format := range []string{"", "compact-v1"} {
		for _, spans := range []string{"", "null", "[]", `[""]`, `["not-a-line"]`, `["0"]`} {
			t.Run(format+"/"+spans, func(t *testing.T) {
				field := ""
				if spans != "" {
					field = `,"source_line_spans":` + spans
				}
				var payload map[string]any
				if err := json.Unmarshal([]byte(`{"output_format":"`+format+`","provisions":[{"name":"bad evidence","provision":"shall inspect"`+field+`}]}`), &payload); err != nil {
					t.Fatal(err)
				}
				p := &ProvisionsProcessor{PromptRef: "test"}
				got, err := p.extractProvisionPayloadFromText(context.Background(), "input", "instructions", "test", &fakeJSONExtractor{out: payload})
				if err == nil || !strings.Contains(err.Error(), "source_line_spans") || got != nil {
					t.Fatalf("unanchored provision accepted: payload=%#v err=%v", got, err)
				}
			})
		}
	}
}

func TestProvisionsMissingSourceSpansTriggersFallback(t *testing.T) {
	bad := map[string]any{"output_format": "compact-v1", "provisions": []any{map[string]any{"provision": "shall inspect"}}}
	good := map[string]any{"output_format": "compact-v1", "provisions": []any{map[string]any{"provision": "shall inspect", "source_line_spans": []any{"42"}}}}
	for _, fallbackBad := range []bool{false, true} {
		fallback := good
		if fallbackBad {
			fallback = bad
		}
		p := &ProvisionsProcessor{
			PromptRef: "test", ModelName: "primary", Extractor: &fakeJSONExtractor{out: bad},
			FallbackModelName: "fallback", FallbackExtractor: &fakeJSONExtractor{out: fallback},
			Logger: loggerutil.CreateDefaultLogger("20261107-649"),
		}
		got, model, err := p.extractProvisionPayloadWithFallback(context.Background(), "input", "instructions")
		if fallbackBad {
			if err == nil || got != nil {
				t.Fatalf("both unanchored responses must fail, not become a successful empty extraction: got=%#v err=%v", got, err)
			}
		} else if err != nil || model != "fallback" || got == nil {
			t.Fatalf("valid fallback was not used: model=%s got=%#v err=%v", model, got, err)
		}
	}
}

func TestProvisionsRun259RejectsUnanchoredResponses(t *testing.T) {
	f, err := os.Open("testdata/provisions-run-259.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	var fixture struct {
		Responses []struct {
			CallID  string         `json:"call_id"`
			Payload map[string]any `json:"payload"`
		} `json:"responses"`
	}
	if err := json.NewDecoder(z).Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	var rejected, affected int
	for _, response := range fixture.Responses {
		p := &ProvisionsProcessor{PromptRef: "prompt-extract-provisions-v4.md"}
		_, err := p.extractProvisionPayloadFromText(context.Background(), "input", "instructions", "test", &fakeJSONExtractor{out: response.Payload})
		wantRejected := response.CallID == "llm-1f52fd3c" || response.CallID == "llm-7274abaf"
		if (err != nil) != wantRejected {
			t.Errorf("call %s: rejection=%v, want %v", response.CallID, err, wantRejected)
		}
		if err != nil {
			rejected++
			affected += len(response.Payload["provisions"].([]any))
		}
	}
	if rejected != 2 || affected != 16 {
		t.Fatalf("rejected=%d affected=%d, want 2 responses and 16 provisions", rejected, affected)
	}
}

func TestProvisionsContractRequiresNonemptySourceSpans(t *testing.T) {
	for _, source := range []string{
		`{"provisions":[{"provision":"shall inspect"}]}`,
		`{"provisions":[{"provision":"shall inspect","source_line_spans":[]}]}`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": source}}}})
		}))
		client := &llmclients.OpenAIJSONClient{BaseURL: srv.URL, APIKey: "test-key", ModelName: "test", HTTPClient: srv.Client()}
		contract := provisionExtractionContract()
		contract.MaxRetries = 0
		_, err := client.ExtractStructuredJSON(context.Background(), llmclients.JSONExtractionInput{PromptText: "test", InputText: "test"}, contract)
		srv.Close()
		if err == nil {
			t.Fatalf("structured contract accepted missing evidence: %s", source)
		}
	}
}

func TestProvisionsStructuredRetriesMissingSourceSpans(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		source := `{"provisions":[{"provision":"shall inspect"}]}`
		if requests.Add(1) > 1 {
			source = `{"provisions":[{"provision":"shall inspect","source_line_spans":["42"]}]}`
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": source}}}})
	}))
	defer srv.Close()
	client := &llmclients.OpenAIJSONClient{BaseURL: srv.URL, APIKey: "test-key", ModelName: "test", HTTPClient: srv.Client()}
	p := &ProvisionsProcessor{PromptRef: "test"}
	got, err := p.extractProvisionPayloadFromText(context.Background(), "input", "instructions", "test", client)
	if err != nil || got == nil || requests.Load() != 2 {
		t.Fatalf("requests=%d got=%#v err=%v, want repair on the second attempt", requests.Load(), got, err)
	}
}

func TestProvisionsProcessorDoesNotSaveUnanchoredResults(t *testing.T) {
	for _, mode := range []string{"blocks", "chunks"} {
		t.Run(mode, func(t *testing.T) {
			inputStore := &fakeDocMetadataStore{rec: DocMetadataInputRecord{ID: 416, ResultFilename: "/tmp/input.json", StagingFilename: "/tmp/input.pdf", ParserName: "mineru", StatusRaw: "[]"}}
			store := &fakeProvisionsStore{}
			bad := map[string]any{"output_format": "compact-v1", "provisions": []any{map[string]any{"provision": "shall inspect"}}}
			p := NewProvisionsProcessor(inputStore, store, &fakeJSONExtractor{out: bad}, nil)
			p.PromptErr, p.ModelErr = nil, nil
			p.PromptText, p.ModelName = "instructions", "test"
			p.FallbackModelName = "fallback"
			p.FallbackModelErr = nil
			p.FallbackExtractor = &fakeJSONExtractor{out: bad}
			p.ExtractProvisionsInput = mode
			ctx, blocks := withBlockBufferHolder(context.Background())
			blocks.buffer = &BlockBuffer{Blocks: []Block{{Index: 1, Lines: []BlockLine{{Flag: "n", LineNumber: 42, PageNumber: 1, LineType: "paragraph", Content: "shall inspect"}}}}}
			ctx, chunks := withChunkBufferHolder(ctx)
			chunks.buffer = &ChunkBuffer{Chunks: []Chunk{{SeqNo: 1, Lines: []MarkedLine{{Mark: "r", Line: Line{LineNo: 42, PageNo: 1, LineType: "paragraph", Content: "shall inspect"}}}}}}
			if err := p.HandleEvent(ctx, []byte(`{"record_id":"416","force":true}`)); err != nil {
				t.Fatal(err)
			}
			if store.saveCalled != 0 {
				t.Fatal("unanchored extraction reached persistence")
			}
			if !strings.Contains(inputStore.updateReq.StatusRaw, `"proc_status":"failed"`) || !strings.Contains(inputStore.updateReq.StatusRaw, "source_line_spans") {
				t.Fatalf("failure status did not report missing evidence: %s", inputStore.updateReq.StatusRaw)
			}
		})
	}
}
