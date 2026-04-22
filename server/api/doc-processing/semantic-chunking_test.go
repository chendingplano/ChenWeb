package docprocessing

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	llmclients "github.com/chendingplano/shared/go/api/llm"
)

func TestBuildSemanticPageBlocks_WithOnePageOverlap(t *testing.T) {
	lines, err := ParseInputLines([]byte(strings.Join([]string{
		"1\t1\theading\tTestFont\t12\t[0,0,1,1]\tP1",
		"2\t2\theading\tTestFont\t12\t[0,0,1,1]\tP2",
		"3\t3\theading\tTestFont\t12\t[0,0,1,1]\tP3",
		"4\t4\theading\tTestFont\t12\t[0,0,1,1]\tP4",
		"5\t5\theading\tTestFont\t12\t[0,0,1,1]\tP5",
	}, "\n")))
	if err != nil {
		t.Fatalf("ParseInputLines: %v", err)
	}

	blocks := BuildSemanticPageBlocks(lines, 2)
	if len(blocks) != 3 {
		t.Fatalf("len(blocks)=%d, want 3", len(blocks))
	}
	if got := blocks[0].Pages; len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("block1 pages=%v, want [1 2]", got)
	}
	if got := blocks[1].Pages; len(got) != 3 || got[0] != 2 || got[1] != 3 || got[2] != 4 {
		t.Fatalf("block2 pages=%v, want [2 3 4]", got)
	}
	if got := blocks[2].Pages; len(got) != 2 || got[0] != 4 || got[1] != 5 {
		t.Fatalf("block3 pages=%v, want [4 5]", got)
	}
}

func TestSemanticChunkingService_HandleInput_WritesTopicsAndStatus(t *testing.T) {
	tmp := t.TempDir()
	input := strings.Join([]string{
		"1\t1\theading\tTestFont\t12\t[0,0,1,1]\tCover",
		"2\t2\tparagraph\tTestFont\t12\t[0,0,1,1]\tSection A",
		"3\t3\ttable\tTestFont\t12\t[0,0,1,1]\t|a|b|",
		"4\t4\tformula\tTestFont\t12\t[0,0,1,1]\ta=b+c",
	}, "\n")

	ex := &fakeSemanticExtractor{
		outs: []map[string]any{
			{
				"topics": []any{
					map[string]any{
						"topic_type": "cover",
						"lines":      []any{"1"},
						"keywords":   []any{"cover"},
						"topic":      "Cover page",
					},
					map[string]any{
						"topic_type": "policy",
						"lines":      []any{"2"},
						"keywords":   []any{"section", "policy"},
						"topic":      "Section A",
					},
				},
			},
			{
				"topics": []any{
					map[string]any{
						"topic_type": "table",
						"lines":      []any{"3"},
						"keywords":   []any{"table"},
						"topic":      "Data table",
					},
					map[string]any{
						"topic_type": "formula",
						"lines":      []any{"4"},
						"keywords":   []any{"formula"},
						"topic":      "Equation",
					},
				},
			},
		},
	}
	st := &fakeStore{rec: InputRecord{ID: 7523, StatusRaw: "[]"}}
	svc := NewSemanticChunkingService(st, ex, nil)
	svc.ChunkDir = tmp
	svc.FileBlockSize = 2
	svc.ModelErr = nil
	svc.ModelName = "topic-model"

	if err := svc.HandleInput(context.Background(), 7523, "sample.txt", []byte(input)); err != nil {
		t.Fatalf("HandleInput: %v", err)
	}
	if ex.calls != 2 {
		t.Fatalf("extractor calls=%d, want 2", ex.calls)
	}
	if st.insertCalls != 1 {
		t.Fatalf("InsertChunkRun calls=%d, want 1", st.insertCalls)
	}
	if st.insertedRun.ChunkingMethod != "topic-chunking" {
		t.Fatalf("chunking_method=%q, want topic-chunking", st.insertedRun.ChunkingMethod)
	}

	topicsPath := filepath.Join(tmp, "7", "7523", "topics.txt")
	bs, err := os.ReadFile(topicsPath)
	if err != nil {
		t.Fatalf("read topics file: %v", err)
	}
	content := string(bs)
	if !strings.Contains(content, "1\tcover\t[1]\t[cover]\tCover page") {
		t.Fatalf("topics content missing expected line: %q", content)
	}
	if !strings.Contains(content, "4\tformula\t[4]\t[formula]\tEquation") {
		t.Fatalf("topics content missing expected formula line: %q", content)
	}

	var status []map[string]any
	if err := json.Unmarshal([]byte(st.updatedStatus), &status); err != nil {
		t.Fatalf("status json: %v", err)
	}
	if len(status) == 0 {
		t.Fatalf("expected status entry")
	}
	last := status[len(status)-1]
	if strings.TrimSpace(asString(last["operation"])) != "topic_chunk" {
		t.Fatalf("operation=%v, want topic_chunk", last["operation"])
	}
	if strings.TrimSpace(asString(last["proc_status"])) != "success" {
		t.Fatalf("proc_status=%v, want success", last["proc_status"])
	}
}

func TestSemanticChunkingService_HandleInput_LLMErrorPersistsFailure(t *testing.T) {
	input := "1\t1\theading\tTestFont\t12\t[0,0,1,1]\tCover"
	st := &fakeStore{rec: InputRecord{ID: 9001, StatusRaw: "[]"}}
	ex := &fakeSemanticExtractor{err: context.DeadlineExceeded}
	svc := NewSemanticChunkingService(st, ex, nil)
	svc.ChunkDir = t.TempDir()
	svc.FileBlockSize = 2
	svc.ModelErr = nil

	err := svc.HandleInput(context.Background(), 9001, "sample.txt", []byte(input))
	if err == nil {
		t.Fatalf("expected error")
	}
	if st.insertCalls != 0 {
		t.Fatalf("InsertChunkRun calls=%d, want 0", st.insertCalls)
	}
	if st.updateCalls != 1 {
		t.Fatalf("UpdateInputStatus calls=%d, want 1", st.updateCalls)
	}
	if st.updatedError == nil || !strings.Contains(*st.updatedError, "extract topics") {
		t.Fatalf("expected persisted extract error, got %v", st.updatedError)
	}

	var status []map[string]any
	if err := json.Unmarshal([]byte(st.updatedStatus), &status); err != nil {
		t.Fatalf("status json: %v", err)
	}
	last := status[len(status)-1]
	if strings.TrimSpace(asString(last["proc_status"])) != "failed" {
		t.Fatalf("proc_status=%v, want failed", last["proc_status"])
	}
	if strings.TrimSpace(asString(last["error"])) == "" {
		t.Fatalf("error should be present on failure")
	}
}

type fakeSemanticExtractor struct {
	outs  []map[string]any
	err   error
	calls int
}

func (f *fakeSemanticExtractor) ExtractJSON(_ context.Context, _ llmclients.JSONExtractionInput) (map[string]any, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if len(f.outs) == 0 {
		return map[string]any{"topics": []any{}}, nil
	}
	if f.calls > len(f.outs) {
		return f.outs[len(f.outs)-1], nil
	}
	return f.outs[f.calls-1], nil
}
