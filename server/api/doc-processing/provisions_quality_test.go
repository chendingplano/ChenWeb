package docprocessing

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

type provisionLogArtifactCount int

func (count provisionLogArtifactCount) Match(value driver.Value) bool {
	text, ok := value.(string)
	if !ok {
		return false
	}
	var payload map[string]any
	if json.Unmarshal([]byte(text), &payload) != nil {
		return false
	}
	rows, _ := payload["provisions"].([]any)
	return len(rows) == int(count)
}

type provisionUsageExtractor struct{ fakeJSONExtractor }

func (e *provisionUsageExtractor) LastJSONUsage() *llmclients.Usage {
	return &llmclients.Usage{PromptCacheHitTokens: e.structuredCalledCount * 10, PromptCacheMissTokens: e.structuredCalledCount * 100}
}

func TestProvisionsLogsDraftAndReviewSeparately(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for idx, prompt := range []string{"prompt-extract-provisions-v7.md", "custom-audit-v2.md"} {
		args := make([]driver.Value, 18)
		for i := range args {
			args[i] = sqlmock.AnyArg()
		}
		args[3] = prompt
		args[10] = provisionLogArtifactCount(idx + 1)
		args[15], args[16] = int64((idx+1)*10), int64((idx+1)*100)
		mock.ExpectExec("INSERT INTO kb.doc_proc_logs").WithArgs(args...).WillReturnResult(sqlmock.NewResult(int64(idx+1), 1))
	}
	var draft, review map[string]any
	_ = json.Unmarshal([]byte(`{"v":2,"l":"en","p":[{"n":"checks","t":"m","x":"A shall be clean. B shall be dry.","s":[10],"lt":"s","q":0.9,"f":1}]}`), &draft)
	_ = json.Unmarshal([]byte(`{"v":2,"l":"en","p":[{"n":"clean","t":"m","x":"A shall be clean.","s":[10],"lt":"s","q":0.9,"f":1},{"n":"dry","t":"m","x":"B shall be dry.","s":[10],"lt":"s","q":0.9,"f":1}]}`), &review)
	extractor := &provisionUsageExtractor{fakeJSONExtractor: fakeJSONExtractor{outs: []map[string]any{draft, review}}}
	p := NewProvisionsProcessor(&fakeDocMetadataStore{}, &fakeProvisionsStore{}, extractor, nil)
	p.PromptErr, p.ModelErr = nil, nil
	p.PromptRef, p.PromptText = "prompt-extract-provisions-v7.md", "instructions"
	p.ReviewPromptErr, p.ReviewPromptRef, p.ReviewPromptText = nil, "custom-audit-v2.md", "review instructions"
	p.ProcLogger = DocProcLogger{DB: db}
	ctx := context.Background()
	chunks := []Chunk{{Lines: []MarkedLine{{Mark: "n", Line: Line{LineNo: 10, Content: "A shall be clean. B shall be dry."}}}}}
	if err := p.InitChunkBatch(ctx, 416, chunks, ""); err != nil {
		t.Fatal(err)
	}
	if err := p.ProcessChunk(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	if extractor.contractNames[1] != "chenweb_provision_extraction_v2" {
		t.Fatalf("custom audit used legacy contract: %v", extractor.contractNames)
	}
}

func TestProvisionsReviewFailureDoesNotSaveDraft(t *testing.T) {
	for _, fail := range []bool{true, false} {
		extractor := &fakeJSONExtractor{err: errors.New("review failed")}
		p := NewProvisionsProcessor(nil, nil, extractor, nil)
		p.PromptRef = "prompt-extract-provisions-v7.md"
		p.ReviewPromptErr = nil
		p.ReviewPromptText = "review"
		if !fail {
			p.ReviewPromptText = ""
		}
		payload := map[string]any{"provisions": []any{map[string]any{"provision": "A shall be clean. B shall be dry."}}}
		if got, err := p.reviewProvisionPayload(context.Background(), "source", "instructions", payload, "model", 0, 1); err == nil || got != nil {
			t.Fatalf("failed review returned draft: got=%#v err=%v", got, err)
		}
	}
}

func TestProvisionsReviewUnitsPreserveEvidenceAndWordBoundaries(t *testing.T) {
	input := `{"doc_context":"requirements","lines":[{"flag":"o","line_number":9,"content":"overlap"},{"flag":"n","line_number":10,"line_type":"list-item","content":"A应为1.25，B应清洁。"}]}`
	for _, source := range []string{input, "Block index: 1\nInput block lines (JSON array):\n" + `[{"flag":"n","line_number":10,"line_type":"list-item","content":"A应为1.25，B应清洁。"}]`} {
		units := provisionReviewUnits(source)
		if len(units) != 1 || units[0]["line"] != 10 || strings.Join(units[0]["clauses"].([]string), "") != "A应为1.25，B应清洁。" {
			t.Fatalf("review hints changed source or evidence: %#v", units)
		}
	}
}

func TestProvisionsReviewUsesSourceAndReplacesDraft(t *testing.T) {
	decode := func(raw string) map[string]any {
		var value map[string]any
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	draft := decode(`{"v":2,"l":"en","p":[{"n":"checks","t":"m","x":"A shall be clean. B shall be dry.","s":[10],"lt":"s","q":0.9,"f":1}]}`)
	reviewed := decode(`{"v":2,"l":"en","p":[{"n":"clean","t":"m","x":"A shall be clean.","s":[10],"lt":"s","q":0.9,"f":1},{"n":"dry","t":"m","x":"B shall be dry.","s":[10],"lt":"s","q":0.9,"f":1}]}`)
	extractor := &fakeJSONExtractor{outs: []map[string]any{draft, reviewed}}
	p := NewProvisionsProcessor(nil, nil, extractor, nil)
	p.PromptRef, p.PromptText = "prompt-extract-provisions-v7.md", "instructions"
	source := `[{"flag":"n","line_number":10,"content":"A shall be clean. B shall be dry."}]`
	draftPayload, model, err := p.extractProvisionPayloadWithFallback(context.Background(), source, p.PromptText)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.reviewProvisionPayload(context.Background(), source, p.PromptText, draftPayload, model, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got["provisions"].([]any)) != 2 || extractor.structuredCalledCount != 2 {
		t.Fatalf("draft was not replaced by review: %#v", got)
	}
	if !strings.Contains(extractor.inputTexts[1], "source") || !strings.Contains(extractor.inputTexts[1], "draft") || !strings.Contains(extractor.inputTexts[1], "line_number") {
		t.Fatalf("review missing source or draft: %s", extractor.inputTexts[1])
	}
	if extractor.promptNames[1] != "prompt-audit-provision-extraction-v1.md" || toFloat(got["review_calls"]) != 1 {
		t.Fatalf("review provenance missing: prompts=%v payload=%#v", extractor.promptNames, got)
	}
}

func TestProvisionsChunkOwnershipAndDuplicates(t *testing.T) {
	row := func(text string, spans ...string) any {
		return map[string]any{"provision": text, "source_line_spans": spans}
	}
	chunk := Chunk{Lines: []MarkedLine{
		{Mark: "o", Line: Line{LineNo: 50}},
		{Mark: "r", Line: Line{LineNo: 51}},
	}}
	items := []any{
		row("Overlap only.", "50"),
		row("The station shall be clean.", "51"),
		row("The station shall be clean", "51"),
		row("The equipment shall be inspected.", "51"),
		row("A requirement spanning context and owned text.", "50-51"),
	}
	p := &ProvisionsProcessor{}
	got := p.normalizeChunkProvisions(items, chunk)
	got = p.deduplicateProvisions(got)
	var texts []string
	for _, r := range got {
		texts = append(texts, asString(r["provision"]))
	}
	want := []string{"The station shall be clean.", "The equipment shall be inspected.", "A requirement spanning context and owned text."}
	if !reflect.DeepEqual(texts, want) {
		t.Fatalf("provisions=%v, want %v", texts, want)
	}
	// Identical wording at separate source locations is not an overlap duplicate.
	distinct := normalizeProvisionList([]any{row("Inspect daily.", "51"), row("Inspect daily.", "52")}, nil, nil)
	if got := p.deduplicateProvisions(distinct); len(got) != 2 {
		t.Fatalf("distinct source locations collapsed: %#v", got)
	}
	distinct = normalizeProvisionList([]any{row("Use A B.", "51"), row("Use AB.", "51")}, nil, nil)
	if got := p.deduplicateProvisions(distinct); len(got) != 2 {
		t.Fatalf("distinct word boundaries collapsed: %#v", got)
	}
	block := Block{Lines: []BlockLine{{Flag: "o", LineNumber: 50}, {Flag: "n", LineNumber: 51}}}
	if got := p.deduplicateProvisions(p.normalizeBlockProvisions(items, block)); !reflect.DeepEqual(got, p.deduplicateProvisions(p.normalizeChunkProvisions(items, chunk))) {
		t.Fatalf("block and chunk ownership differ: %#v", got)
	}
}

func TestProvisionsBatchFailedAndEmptyResults(t *testing.T) {
	for _, scenario := range []string{"all failed", "successful empty", "first failed"} {
		t.Run(scenario, func(t *testing.T) {
			tmp := t.TempDir()
			t.Setenv("ARTIFACT_DIR", tmp)
			t.Setenv("ARTIFACT_WEB_DIR", tmp)
			store := &fakeProvisionsStore{}
			input := &fakeDocMetadataStore{rec: DocMetadataInputRecord{ID: 416, ResultFilename: "test.txt", StatusRaw: "[]"}}
			extractor := &fakeJSONExtractor{out: map[string]any{"provisions": []any{}}}
			p := NewProvisionsProcessor(input, store, extractor, nil)
			p.PromptErr, p.ModelErr = nil, nil
			p.PromptRef = "test"
			chunks := []Chunk{
				{Lines: []MarkedLine{{Mark: "n", Line: Line{LineNo: 10}}}},
				{Lines: []MarkedLine{{Mark: "n", Line: Line{LineNo: 20}}}},
			}
			ctx := context.Background()
			if err := p.InitChunkBatch(ctx, 416, chunks, ""); err != nil {
				t.Fatal(err)
			}
			if scenario != "all failed" {
				if scenario == "first failed" {
					extractor.out["provisions"] = []any{map[string]any{"provision": "Second.", "source_line_spans": []int{20}}}
				}
				if err := p.ProcessChunk(ctx, 1); err != nil {
					t.Fatal(err)
				}
			}
			if err := p.FinalizeChunkBatch(ctx); err != nil {
				t.Fatal(err)
			}
			if scenario == "all failed" {
				if store.saveCalled != 0 {
					t.Fatal("all failed calls finalized as successful empty extraction")
				}
			} else if store.saveCalled != 1 {
				t.Fatal("successful result was not finalized")
			} else if scenario == "first failed" && len(store.lastSave.Provisions) != 1 {
				t.Fatalf("successful chunk lost after earlier failure: %#v", store.lastSave)
			}
		})
	}
}

func TestProvisionsBatchFinalizesInChunkOrder(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("ARTIFACT_DIR", tmp)
	t.Setenv("ARTIFACT_WEB_DIR", tmp)
	store := &fakeProvisionsStore{}
	inputs := &fakeDocMetadataStore{rec: DocMetadataInputRecord{ID: 416, ResultFilename: "test.txt", StatusRaw: "[]"}}
	extractor := &fakeJSONExtractor{}
	p := NewProvisionsProcessor(inputs, store, extractor, nil)
	p.PromptErr, p.ModelErr = nil, nil
	p.PromptRef = "test"
	chunks := []Chunk{
		{SeqNo: 1, Lines: []MarkedLine{{Mark: "r", Line: Line{LineNo: 10, Content: "First."}}}},
		{SeqNo: 2, Lines: []MarkedLine{{Mark: "r", Line: Line{LineNo: 20, Content: "Second."}}}},
	}
	ctx := context.Background()
	if err := p.InitChunkBatch(ctx, 416, chunks, ""); err != nil {
		t.Fatal(err)
	}
	// Deliberately finish the second chunk first. No sleep or scheduler timing dependency.
	for _, idx := range []int{1, 0} {
		extractor.out = map[string]any{"language": "en", "provisions": []any{map[string]any{
			"provision":         chunks[idx].Lines[0].Line.Content,
			"source_line_spans": []int{chunks[idx].Lines[0].Line.LineNo},
		}}}
		if err := p.ProcessChunk(ctx, idx); err != nil {
			t.Fatal(err)
		}
	}
	if err := p.FinalizeChunkBatch(ctx); err != nil {
		t.Fatal(err)
	}
	rows := store.lastSave.Provisions
	if len(rows) != 2 || rows[0]["provision"] != "First." || rows[1]["provision"] != "Second." {
		t.Fatalf("batch result order=%#v", rows)
	}
}
