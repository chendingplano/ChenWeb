package docprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeStore struct {
	rec           InputRecord
	getErr        error
	insertedRun   ChunkRunRecord
	insertCalls   int
	updatedStatus string
	updatedError  *string
	updateCalls   int
}

func (f *fakeStore) GetInputRecord(_ context.Context, id int64) (InputRecord, error) {
	if f.getErr != nil {
		return InputRecord{}, f.getErr
	}
	if id != f.rec.ID {
		return InputRecord{}, errors.New("not found")
	}
	return f.rec, nil
}

func (f *fakeStore) InsertChunkRun(_ context.Context, rec ChunkRunRecord) error {
	f.insertedRun = rec
	f.insertCalls++
	return nil
}

func (f *fakeStore) UpdateInputStatus(_ context.Context, id int64, statusJSON string, errorMsg *string) error {
	if id != f.rec.ID {
		return errors.New("wrong id")
	}
	f.updateCalls++
	f.updatedStatus = statusJSON
	f.updatedError = errorMsg
	return nil
}

func TestBuildChunks_RespectsTableBlock(t *testing.T) {
	input := strings.Join([]string{
		"1	1	paragraph	TestFont	12	[0,0,1,1]	Intro",
		"2	1	table	TestFont	12	[0,0,1,1]	|A|B|",
		"3	1	table	TestFont	12	[0,0,1,1]	|1|2|",
		"4	1	paragraph	TestFont	12	[0,0,1,1]	Tail",
	}, "\n")
	lines, err := ParseInputLines([]byte(input))
	if err != nil {
		t.Fatalf("ParseInputLines: %v", err)
	}

	chunks, err := BuildChunks(lines, ChunkOptions{ChunkSize: 45, OverlapPercent: 0})
	if err != nil {
		t.Fatalf("BuildChunks: %v", err)
	}
	chunkByLine := map[int]int{}
	for _, c := range chunks {
		for _, ml := range c.Lines {
			chunkByLine[ml.Line.LineNo] = c.SeqNo
		}
	}
	if chunkByLine[2] == 0 || chunkByLine[3] == 0 || chunkByLine[2] != chunkByLine[3] {
		t.Fatalf("expected table lines 2 and 3 to stay in same chunk, got line2=%d line3=%d", chunkByLine[2], chunkByLine[3])
	}
}

func TestBuildChunks_NonNumericListNotSplit(t *testing.T) {
	input := strings.Join([]string{
		"1	1	paragraph	TestFont	12	[0,0,1,1]	Intro",
		"2	1	list-item	TestFont	12	[0,0,1,1]	- item A",
		"3	1	list-item	TestFont	12	[0,0,1,1]	- item B",
		"4	1	paragraph	TestFont	12	[0,0,1,1]	Tail",
	}, "\n")
	lines, err := ParseInputLines([]byte(input))
	if err != nil {
		t.Fatalf("ParseInputLines: %v", err)
	}

	chunks, err := BuildChunks(lines, ChunkOptions{ChunkSize: 45, OverlapPercent: 0})
	if err != nil {
		t.Fatalf("BuildChunks: %v", err)
	}
	chunkByLine := map[int]int{}
	for _, c := range chunks {
		for _, ml := range c.Lines {
			chunkByLine[ml.Line.LineNo] = c.SeqNo
		}
	}
	if chunkByLine[2] == 0 || chunkByLine[3] == 0 || chunkByLine[2] != chunkByLine[3] {
		t.Fatalf("expected non-numeric list lines 2 and 3 to stay in same chunk, got line2=%d line3=%d", chunkByLine[2], chunkByLine[3])
	}
}

func TestBuildChunks_LargeNumericListCanSplit(t *testing.T) {
	raw := []string{"1	1	paragraph	TestFont	12	[0,0,1,1]	Intro"}
	for i := 1; i <= 8; i++ {
		raw = append(raw, "2	1	list-item	TestFont	12	[0,0,1,1]	"+string(rune('0'+i))+". item")
	}
	input := strings.Join(raw, "\n")
	lines, err := ParseInputLines([]byte(input))
	if err != nil {
		t.Fatalf("ParseInputLines: %v", err)
	}

	chunks, err := BuildChunks(lines, ChunkOptions{ChunkSize: 40, OverlapPercent: 0})
	if err != nil {
		t.Fatalf("BuildChunks: %v", err)
	}
	if len(chunks) < 4 {
		t.Fatalf("expected large numeric list to be splittable, chunks=%d", len(chunks))
	}
}

func TestService_HandleInput_WritesChunksAndStatus(t *testing.T) {
	tmp := t.TempDir()
	treeRoot := t.TempDir()
	input := strings.Join([]string{
		"1	1	paragraph	TestFont	12	[0,0,1,1]	Intro",
		"2	1	paragraph	TestFont	12	[0,0,1,1]	More",
		"3	2	paragraph	TestFont	12	[0,0,1,1]	End",
	}, "\n")

	st := &fakeStore{rec: InputRecord{
		ID:              7523,
		StatusRaw:       "[]",
		ParserName:      "opendata",
		StagingFilename: "std_20039.pdf",
	}}
	ex := &fakeSemanticExtractor{
		outs: []map[string]any{
			{
				"topics": []any{
					map[string]any{
						"topic_type":    "policy",
						"lines":         []any{"1-2"},
						"keywords":      []any{"intro", "scope"},
						"topic":         "Intro scope",
						"category_path": []any{"document_overview"},
					},
				},
			},
			{
				"topics": []any{
					map[string]any{
						"topic_type":    "policy",
						"lines":         []any{"3"},
						"keywords":      []any{"end"},
						"topic":         "Ending section",
						"category_path": []any{"document_overview", "closing_notes"},
					},
				},
			},
		},
	}
	svc := NewFixedSizeChunkingService(st, ex, nil)
	svc.ChunkDir = tmp
	svc.TreeRootDir = treeRoot
	svc.ChunkSize = 25
	svc.OverlapPercent = 50
	svc.ModelErr = nil
	svc.PromptErr = nil
	svc.ModelName = "topic-model"
	svc.PromptText = "extract chunk topics"

	if err := svc.HandleInput(context.Background(), 7523, "sample.txt", []byte(input)); err != nil {
		t.Fatalf("HandleInput: %v", err)
	}
	if ex.calls != 2 {
		t.Fatalf("extractor calls=%d, want 2", ex.calls)
	}

	if st.insertCalls != 1 {
		t.Fatalf("InsertChunkRun calls=%d, want 1", st.insertCalls)
	}
	if st.updateCalls != 1 {
		t.Fatalf("UpdateInputStatus calls=%d, want 1", st.updateCalls)
	}
	if st.insertedRun.ChunkingMethod != "fix-size" {
		t.Fatalf("chunking_method=%q, want fix-size", st.insertedRun.ChunkingMethod)
	}

	chunkPath := filepath.Join(tmp, "7", "7523", "std_20039_opendata.chunks")
	topicPath := filepath.Join(tmp, "7", "7523", "std_20039_opendata.topics")
	legacyTopicsPath := filepath.Join(tmp, "7", "7523", "topics.txt")
	if _, err := os.Stat(chunkPath); err != nil {
		t.Fatalf("missing chunk artifact: %v", err)
	}
	if _, err := os.Stat(topicPath); err != nil {
		t.Fatalf("missing topic artifact: %v", err)
	}
	if _, err := os.Stat(legacyTopicsPath); err != nil {
		t.Fatalf("missing legacy topics.txt: %v", err)
	}

	b2, err := os.ReadFile(chunkPath)
	if err != nil {
		t.Fatalf("read chunk artifact: %v", err)
	}
	content := strings.TrimSpace(string(b2))
	wantSnippets := []string{
		"overlap: []",
		"lines: [1-2]",
		"overlap: [2]",
		"lines: [3]",
	}
	for _, want := range wantSnippets {
		if !strings.Contains(content, want) {
			t.Fatalf("expected chunk artifact to contain %q, got %q", want, content)
		}
	}
	topicContent, err := os.ReadFile(topicPath)
	if err != nil {
		t.Fatalf("read topic artifact: %v", err)
	}
	if !strings.Contains(string(topicContent), "1\tpolicy\t[1-2]\t[intro, scope]\tIntro scope") {
		t.Fatalf("unexpected topic artifact content: %q", string(topicContent))
	}
	treeLeaf := filepath.Join(treeRoot, "document_overview", "closing_notes.txt")
	treeContent, err := os.ReadFile(treeLeaf)
	if err != nil {
		t.Fatalf("read topic tree leaf: %v", err)
	}
	if !strings.Contains(string(treeContent), "7523\tpolicy\t[3]\t[end]\tEnding section") {
		t.Fatalf("unexpected tree leaf content: %q", string(treeContent))
	}

	var status []map[string]any
	if err := json.Unmarshal([]byte(st.updatedStatus), &status); err != nil {
		t.Fatalf("status json: %v", err)
	}
	if len(status) == 0 {
		t.Fatalf("expected status entry")
	}
	last := status[len(status)-1]
	if last["operation"] != "chunked" {
		t.Fatalf("operation=%v, want chunked", last["operation"])
	}
	if last["proc_status"] != "success" {
		t.Fatalf("proc_status=%v, want success", last["proc_status"])
	}
}

func TestService_HandleInput_MissingInputFilename(t *testing.T) {
	st := &fakeStore{rec: InputRecord{ID: 1001, StatusRaw: "[]"}}
	svc := NewFixedSizeChunkingService(st, &fakeSemanticExtractor{}, nil)
	svc.ChunkDir = t.TempDir()
	svc.TreeRootDir = t.TempDir()
	svc.ChunkSize = 2
	svc.OverlapPercent = 0

	err := svc.HandleInput(context.Background(), 1001, "", []byte("1	1	paragraph	TestFont	12	[0,0,1,1]	x"))
	if err == nil {
		t.Fatalf("expected error when input_filename is empty")
	}
	if !strings.Contains(err.Error(), "missing input filename") {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.insertCalls != 0 {
		t.Fatalf("InsertChunkRun calls=%d, want 0", st.insertCalls)
	}
	if st.updateCalls != 1 {
		t.Fatalf("UpdateInputStatus calls=%d, want 1", st.updateCalls)
	}
	if st.updatedError == nil || !strings.Contains(*st.updatedError, "missing input filename") {
		t.Fatalf("expected persisted error for missing filename, got %v", st.updatedError)
	}
}

func TestNewService_UsesRequiredAndDefaultChunkEnv(t *testing.T) {
	t.Setenv("CHUNK_SIZE", "")
	t.Setenv("CHUNK_OVERLAP_PERCENT", "")
	t.Setenv("ARTIFACT_DIR", "")

	svc := NewFixedSizeChunkingService(&fakeStore{}, &fakeSemanticExtractor{}, nil)
	if svc.ChunkSize != 300 {
		t.Fatalf("ChunkSize=%d, want 300", svc.ChunkSize)
	}
	if svc.OverlapPercent != 20 {
		t.Fatalf("OverlapPercent=%d, want 20", svc.OverlapPercent)
	}
	if svc.ChunkDir != "" {
		t.Fatalf("ChunkDir=%q, want empty when ARTIFACT_DIR is unset", svc.ChunkDir)
	}
}

func TestService_HandleInput_MissingChunkDir(t *testing.T) {
	st := &fakeStore{rec: InputRecord{ID: 2002, StatusRaw: "[]"}}
	svc := NewFixedSizeChunkingService(st, &fakeSemanticExtractor{}, nil)
	svc.ChunkDir = ""
	svc.ChunkSize = 2
	svc.OverlapPercent = 0

	err := svc.HandleInput(context.Background(), 2002, "sample.txt", []byte("1	1	paragraph	TestFont	12	[0,0,1,1]	x"))
	if err == nil {
		t.Fatalf("expected error when ARTIFACT_DIR is empty")
	}
	if !strings.Contains(err.Error(), "missing ARTIFACT_DIR") {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.insertCalls != 0 {
		t.Fatalf("InsertChunkRun calls=%d, want 0", st.insertCalls)
	}
	if st.updateCalls != 1 {
		t.Fatalf("UpdateInputStatus calls=%d, want 1", st.updateCalls)
	}
	if st.updatedError == nil || !strings.Contains(*st.updatedError, "missing ARTIFACT_DIR") {
		t.Fatalf("expected persisted error for missing ARTIFACT_DIR, got %v", st.updatedError)
	}
}

func TestService_HandleInput_MissingTreeRootDir(t *testing.T) {
	st := &fakeStore{rec: InputRecord{
		ID:              2003,
		StatusRaw:       "[]",
		ParserName:      "opendata",
		StagingFilename: "sample.pdf",
	}}
	svc := NewFixedSizeChunkingService(st, &fakeSemanticExtractor{}, nil)
	svc.ChunkDir = t.TempDir()
	svc.TreeRootDir = ""
	svc.ChunkSize = 2
	svc.OverlapPercent = 0
	svc.ModelErr = nil
	svc.PromptErr = nil
	svc.ModelName = "topic-model"
	svc.PromptText = "prompt"

	err := svc.HandleInput(context.Background(), 2003, "sample.txt", []byte("1\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tx"))
	if err == nil {
		t.Fatalf("expected error when CHUNK_TREE_ROOT_DIR is empty")
	}
	if !strings.Contains(err.Error(), "missing CHUNK_TREE_ROOT_DIR") {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.insertCalls != 0 {
		t.Fatalf("InsertChunkRun calls=%d, want 0", st.insertCalls)
	}
	if st.updateCalls != 1 {
		t.Fatalf("UpdateInputStatus calls=%d, want 1", st.updateCalls)
	}
	if st.updatedError == nil || !strings.Contains(*st.updatedError, "missing CHUNK_TREE_ROOT_DIR") {
		t.Fatalf("expected persisted error for missing CHUNK_TREE_ROOT_DIR, got %v", st.updatedError)
	}
}
