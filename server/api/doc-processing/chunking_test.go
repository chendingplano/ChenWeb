package docprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	svc.SummaryTreeDir = t.TempDir()
	svc.ChunkSize = 25
	svc.OverlapPercent = 50
	svc.ModelErr = nil
	svc.PromptErr = nil
	svc.ModelName = "topic-model"
	svc.PromptText = "extract chunk topics"
	svc.SummaryModelErr = nil
	svc.SummaryPromptErr = nil
	svc.SummaryModelName = "summary-model"
	svc.SummaryPromptText = "summary prompt"
	svc.GenerateSummary = func(_ context.Context, _ int64, level int, seqNo int, _ []Line, children []SummaryItem) (string, []string, error) {
		if level == 0 {
			return "chunk summary " + asString(seqNo), nil, nil
		}
		return "parent summary " + strings.Join(collectSummaryIDs(children), ","), nil, nil
	}
	svc.GenerateSummaryTreeCategories = func(_ context.Context, _ SummaryItem) ([]string, []CategoryPathNode, error) {
		return []string{"legacy_summary_tree"}, nil, nil
	}

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
	if _, err := os.Stat(chunkPath); err != nil {
		t.Fatalf("missing chunk artifact: %v", err)
	}
	if _, err := os.Stat(topicPath); err != nil {
		t.Fatalf("missing topic artifact: %v", err)
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
	topicRows, err := readTopicsFile(topicPath)
	if err != nil {
		t.Fatalf("read topic artifact: %v", err)
	}
	if len(topicRows) == 0 {
		t.Fatalf("expected at least one topic record in topic artifact")
	}
	first := topicRows[0]
	if first.TopicType != "policy" {
		t.Fatalf("first topic_type=%q, want policy", first.TopicType)
	}
	if first.Topic != "Intro scope" {
		t.Fatalf("first topic=%q, want Intro scope", first.Topic)
	}
	// New tree format: topics.txt inside a sub-directory for each category level.
	treeLeaf := filepath.Join(treeRoot, "document_overview", "closing_notes", "topics.txt")
	treeContent, err := os.ReadFile(treeLeaf)
	if err != nil {
		t.Fatalf("read topic tree leaf: %v", err)
	}
	wantTopicLines := []string{"record_id: 7523,", `topic_type: "policy"`, "Ending section"}
	for _, want := range wantTopicLines {
		if !strings.Contains(string(treeContent), want) {
			t.Fatalf("expected tree leaf to contain %q, got: %q", want, string(treeContent))
		}
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
	svc.SummaryTreeDir = t.TempDir()
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
	svc.SummaryTreeDir = t.TempDir()
	svc.SummaryModelErr = nil
	svc.SummaryPromptErr = nil
	svc.SummaryModelName = "summary-model"
	svc.SummaryPromptText = "summary prompt"

	err := svc.HandleInput(context.Background(), 2003, "sample.txt", []byte("1\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tx"))
	if err == nil {
		t.Fatalf("expected error when TOPIC_TREE_ROOT_DIR is empty")
	}
	if !strings.Contains(err.Error(), "missing TOPIC_TREE_ROOT_DIR") {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.insertCalls != 0 {
		t.Fatalf("InsertChunkRun calls=%d, want 0", st.insertCalls)
	}
	if st.updateCalls != 1 {
		t.Fatalf("UpdateInputStatus calls=%d, want 1", st.updateCalls)
	}
	if st.updatedError == nil || !strings.Contains(*st.updatedError, "missing TOPIC_TREE_ROOT_DIR") {
		t.Fatalf("expected persisted error for missing TOPIC_TREE_ROOT_DIR, got %v", st.updatedError)
	}
}

func TestService_HandleInput_WritesSummariesTree(t *testing.T) {
	tmp := t.TempDir()
	topicTreeRoot := t.TempDir()
	summaryTreeRoot := t.TempDir()
	input := strings.Join([]string{
		"1\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tAlpha",
		"2\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tBeta",
		"3\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tGamma",
		"4\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tDelta",
	}, "\n")

	st := &fakeStore{rec: InputRecord{
		ID:              8123,
		StatusRaw:       "[]",
		ParserName:      "opendata",
		StagingFilename: "sample.pdf",
	}}
	ex := &fakeSemanticExtractor{
		outs: []map[string]any{
			{"topics": []any{map[string]any{
				"topic_type":    "policy",
				"lines":         []any{"1-2"},
				"keywords":      []any{"alpha"},
				"topic":         "Alpha topic",
				"category_path": []any{"document_overview"},
			}}},
			{"topics": []any{map[string]any{
				"topic_type":    "policy",
				"lines":         []any{"3-4"},
				"keywords":      []any{"gamma"},
				"topic":         "Gamma topic",
				"category_path": []any{"document_overview", "details"},
			}}},
		},
	}
	svc := NewFixedSizeChunkingService(st, ex, nil)
	svc.ChunkDir = tmp
	svc.TreeRootDir = topicTreeRoot
	svc.SummaryTreeDir = summaryTreeRoot
	svc.ChunkSize = 22
	svc.OverlapPercent = 0
	svc.ModelErr = nil
	svc.PromptErr = nil
	svc.ModelName = "topic-model"
	svc.PromptText = "extract chunk topics"
	svc.SummaryModelErr = nil
	svc.SummaryPromptErr = nil
	svc.SummaryModelName = "summary-model"
	svc.SummaryPromptText = "summarize chunk"
	svc.SummaryGroupSize = 2
	svc.GenerateSummary = func(_ context.Context, _ int64, level int, seqNo int, lines []Line, children []SummaryItem) (string, []string, error) {
		if level == 0 {
			return "leaf summary " + asString(seqNo) + " lines=" + formatLineNumberRanges(chunkLineNosFromLines(lines)), nil, nil
		}
		ids := make([]string, 0, len(children))
		for _, child := range children {
			ids = append(ids, child.SummaryID)
		}
		return "parent summary " + asString(seqNo) + " children=" + strings.Join(ids, ","), nil, nil
	}
	svc.GenerateSummaryTreeCategories = func(_ context.Context, _ SummaryItem) ([]string, []CategoryPathNode, error) {
		return []string{"Safety Overview", "Closing Notes"}, nil, nil
	}

	if err := svc.HandleInput(context.Background(), 8123, "sample.txt", []byte(input)); err != nil {
		t.Fatalf("HandleInput: %v", err)
	}

	recordDir := filepath.Join(tmp, "8", "8123")
	for _, rel := range []string{
		"summary_0_0001.txt",
		"summary_0_0002.txt",
		"summary_1_0001.txt",
	} {
		if _, err := os.Stat(filepath.Join(recordDir, rel)); err != nil {
			t.Fatalf("missing summary artifact %s: %v", rel, err)
		}
	}

	treeLeaf := filepath.Join(summaryTreeRoot, "safety_overview", "closing_notes", "summaries.txt")
	treeBody, err := os.ReadFile(treeLeaf)
	if err != nil {
		t.Fatalf("read summary tree leaf: %v", err)
	}
	wantTreeContent := "8123_0_0001\n8123_0_0002\n8123_1_0001"
	if strings.TrimSpace(string(treeBody)) != wantTreeContent {
		t.Fatalf("unexpected summary tree content: %q, want %q", strings.TrimSpace(string(treeBody)), wantTreeContent)
	}
}

func TestService_HandleInput_SummaryGenerationFailure(t *testing.T) {
	st := &fakeStore{rec: InputRecord{
		ID:              9001,
		StatusRaw:       "[]",
		ParserName:      "opendata",
		StagingFilename: "sample.pdf",
	}}
	svc := NewFixedSizeChunkingService(st, &fakeSemanticExtractor{}, nil)
	svc.ChunkDir = t.TempDir()
	svc.TreeRootDir = t.TempDir()
	svc.SummaryTreeDir = t.TempDir()
	svc.ChunkSize = 10
	svc.OverlapPercent = 0
	svc.ModelErr = nil
	svc.PromptErr = nil
	svc.ModelName = "topic-model"
	svc.PromptText = "prompt"
	svc.SummaryModelErr = nil
	svc.SummaryPromptErr = nil
	svc.SummaryModelName = "summary-model"
	svc.SummaryPromptText = "summary prompt"
	svc.GenerateSummary = func(_ context.Context, _ int64, _ int, _ int, _ []Line, _ []SummaryItem) (string, []string, error) {
		return "", nil, errors.New("summary generator boom")
	}

	err := svc.HandleInput(context.Background(), 9001, "sample.txt", []byte("1\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tx"))
	if err == nil {
		t.Fatalf("expected summary generation error")
	}
	if !strings.Contains(err.Error(), "summary generator boom") {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.updatedError == nil || !strings.Contains(*st.updatedError, "summary generator boom") {
		t.Fatalf("expected persisted summary generation error, got %v", st.updatedError)
	}
}

func TestAppendChunkedStatus_SanitizesInvalidUTF8Error(t *testing.T) {
	rawErr := errors.New(string([]byte{'b', 'a', 'd', ':', ' ', 0xe4, 0x2e, 0x6d}))
	statusRaw, err := appendChunkedStatus("[]", chunkStatusParams{
		InputFilename: "sample.txt",
		NumPages:      1,
		NumLines:      2,
		NumChunks:     1,
		Start:         time.Unix(0, 0),
		DurationMs:    12,
		ProcErr:       rawErr,
	})
	if err != nil {
		t.Fatalf("appendChunkedStatus: %v", err)
	}
	if !json.Valid([]byte(statusRaw)) {
		t.Fatalf("status JSON is invalid: %q", statusRaw)
	}
	if strings.Contains(statusRaw, string([]byte{0xe4, 0x2e, 0x6d})) {
		t.Fatalf("status JSON still contains invalid utf8 sequence: %q", statusRaw)
	}
	if !strings.Contains(statusRaw, "bad: ") {
		t.Fatalf("expected sanitized error message in status JSON: %q", statusRaw)
	}
}

func chunkLineNosFromLines(lines []Line) []int {
	out := make([]int, 0, len(lines))
	for _, line := range lines {
		out = append(out, line.LineNo)
	}
	return out
}
