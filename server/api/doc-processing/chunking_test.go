package docprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeLogger struct {
	infos  []fakeLogEntry
	warns  []fakeLogEntry
	errors []fakeLogEntry
}

type fakeLogEntry struct {
	message string
	args    []any
}

func (f *fakeLogger) Debug(string, ...any) {}
func (f *fakeLogger) Line(string, ...any)  {}
func (f *fakeLogger) Trace(string)         {}
func (f *fakeLogger) Close()               {}

func TestChunkLineRawByteSizeDelegatesToProductionAccounting(t *testing.T) {
	tests := []Line{
		{LineNo: 1, PageNo: 2, LineType: "paragraph", Content: "ascii"},
		{LineNo: 2, PageNo: 3, LineType: "paragraph", Content: "tab\tvalue"},
		{LineNo: 3, PageNo: 4, LineType: "paragraph", Content: "中文α"},
		{Raw: "raw\tbytes", LineNo: 4, PageNo: 5, LineType: "paragraph", Content: "canonical"},
	}
	for _, line := range tests {
		if got, want := ChunkLineRawByteSize(line), lineRawByteSize(line); got != want {
			t.Fatalf("line %d: got %d, want production size %d", line.LineNo, got, want)
		}
	}
}

func (f *fakeLogger) Warn(message string, args ...any) {
	f.warns = append(f.warns, fakeLogEntry{message: message, args: append([]any(nil), args...)})
}

func (f *fakeLogger) Error(message string, args ...any) {
	f.errors = append(f.errors, fakeLogEntry{message: message, args: append([]any(nil), args...)})
}

func (f *fakeLogger) Info(message string, args ...any) {
	f.infos = append(f.infos, fakeLogEntry{message: message, args: append([]any(nil), args...)})
}

func findInfoLog(entries []fakeLogEntry, message string) (fakeLogEntry, bool) {
	for _, entry := range entries {
		if entry.message == message {
			return entry, true
		}
	}
	return fakeLogEntry{}, false
}

func logValue(args []any, key string) (any, bool) {
	for i := 0; i+1 < len(args); i += 2 {
		if k, ok := args[i].(string); ok && k == key {
			return args[i+1], true
		}
	}
	return nil, false
}

type fakeStore struct {
	rec           InputRecord
	getErr        error
	insertedRun   ChunkRunRecord
	insertCalls   int
	updatedStatus string
	updateHistory []string
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
	f.updateHistory = append(f.updateHistory, statusJSON)
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

func TestBuildChunks_NumericListNotSplit(t *testing.T) {
	input := strings.Join([]string{
		"1	1	paragraph	TestFont	12	[0,0,1,1]	Intro",
		"2	1	list-item	TestFont	12	[0,0,1,1]	1. item A",
		"3	1	list-item	TestFont	12	[0,0,1,1]	2. item B",
		"4	1	paragraph	TestFont	12	[0,0,1,1]	Tail",
	}, "\n")
	lines, err := ParseInputLines([]byte(input))
	if err != nil {
		t.Fatalf("ParseInputLines: %v", err)
	}

	chunks, err := BuildChunks(lines, ChunkOptions{ChunkSize: 40, OverlapPercent: 0})
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
		t.Fatalf("expected numeric list lines 2 and 3 to stay in same chunk, got line2=%d line3=%d", chunkByLine[2], chunkByLine[3])
	}
}

func TestBuildChunks_MixedListVariantsNotSplit(t *testing.T) {
	input := strings.Join([]string{
		"111	6	list-item	HiddenHorzOCR	9	[112.81,351.18,453.85,362.88]	c)呼吸...:",
		"112	6	m-sym-list-item	HiddenHorzOCR	10	[112.81,336,287.77,348]	d) 中枢神...",
		"114	6	heading-4	Times-Roman	11	[92.36,306.242,211.93,318.612]	4. 1. 1. 3 耳、鼻、咽眼科",
		"115	6	list-item	HiddenHorzOCR	10	[113.28,289.93,553.44,301.93]	a)昕觉:纯音...;",
		"116	6	m-sym-list-item	HiddenHorzOCR	9	[114,275.46,330.97,286.56]	b) 嗅觉:嗅觉正常，能觉察燃烧物和异常气味。",
		"117	6	heading-4	Times-Roman	11	[92.6,260.393,159.61,272.762]	4. 1. 1. 4 眼科",
	}, "\n")
	lines, err := ParseInputLines([]byte(input))
	if err != nil {
		t.Fatalf("ParseInputLines: %v", err)
	}

	chunks, err := BuildChunks(lines, ChunkOptions{ChunkSize: 120, OverlapPercent: 0})
	if err != nil {
		t.Fatalf("BuildChunks: %v", err)
	}
	chunkByLine := map[int]int{}
	for _, c := range chunks {
		for _, ml := range c.Lines {
			chunkByLine[ml.Line.LineNo] = c.SeqNo
		}
	}
	if chunkByLine[115] == 0 || chunkByLine[116] == 0 || chunkByLine[115] != chunkByLine[116] {
		t.Fatalf("expected mixed list-variant lines 115 and 116 to stay in same chunk, got line115=%d line116=%d", chunkByLine[115], chunkByLine[116])
	}
}

func TestParseBlockBufferLines_SkipsTOCLines(t *testing.T) {
	buf := &BlockBuffer{
		Blocks: []Block{
			{
				Index: 1,
				Lines: []BlockLine{
					{Flag: "n", LineNumber: 8, PageNumber: 1, LineType: "paragraph", Content: "title"},
					{Flag: "n", LineNumber: 12, PageNumber: 2, LineType: "toc", Content: "目次"},
					{Flag: "n", LineNumber: 13, PageNumber: 2, LineType: "TOC", Content: "前言"},
					{Flag: "o", LineNumber: 14, PageNumber: 2, LineType: "toc", Content: "ignored overlap"},
					{Flag: "n", LineNumber: 23, PageNumber: 3, LineType: "paragraph", Content: "body"},
				},
			},
		},
	}

	lines := ParseBlockBufferLines(buf)
	if len(lines) != 2 {
		t.Fatalf("lines=%d, want 2", len(lines))
	}
	if lines[0].LineNo != 8 || lines[1].LineNo != 23 {
		t.Fatalf("line numbers=%v, want [8 23]", []int{lines[0].LineNo, lines[1].LineNo})
	}
	for _, line := range lines {
		if strings.EqualFold(strings.TrimSpace(line.LineType), "toc") {
			t.Fatalf("unexpected TOC line in output: %+v", line)
		}
	}
}

func TestParseInputLinesIncludingTOC_PreservesTOCLines(t *testing.T) {
	input := strings.Join([]string{
		"1\t1\ttoc\tTestFont\t12\t[0,0,1,1]\tTable of Contents",
		"2\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tAlpha",
	}, "\n")

	lines, err := ParseInputLinesIncludingTOC([]byte(input))
	if err != nil {
		t.Fatalf("ParseInputLinesIncludingTOC: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("lines=%d, want 2", len(lines))
	}
	if lines[0].LineNo != 1 || lines[0].LineType != "toc" {
		t.Fatalf("first line=%+v, want toc line 1", lines[0])
	}

	filtered, err := ParseInputLines([]byte(input))
	if err != nil {
		t.Fatalf("ParseInputLines: %v", err)
	}
	if len(filtered) != 1 {
		t.Fatalf("filtered lines=%d, want 1", len(filtered))
	}
	if filtered[0].LineNo != 2 {
		t.Fatalf("filtered line=%+v, want line 2 only", filtered[0])
	}
}

func TestBuildChunks_SkipsTOCLinesInRegularAndOverlap(t *testing.T) {
	lines := []Line{
		{LineNo: 1, PageNo: 1, LineType: "paragraph", Font: "TestFont", FontSize: "12", Coordinate: "[0,0,1,1]", Content: "Alpha Alpha"},
		{LineNo: 2, PageNo: 1, LineType: "paragraph", Font: "TestFont", FontSize: "12", Coordinate: "[0,0,1,1]", Content: "Beta Beta"},
		{LineNo: 3, PageNo: 1, LineType: "toc", Font: "TestFont", FontSize: "12", Coordinate: "[0,0,1,1]", Content: "Table of Contents"},
		{LineNo: 4, PageNo: 1, LineType: "TOC", Font: "TestFont", FontSize: "12", Coordinate: "[0,0,1,1]", Content: "Chapter 1"},
		{LineNo: 5, PageNo: 2, LineType: "paragraph", Font: "TestFont", FontSize: "12", Coordinate: "[0,0,1,1]", Content: "Gamma Gamma Gamma Gamma"},
		{LineNo: 6, PageNo: 2, LineType: "paragraph", Font: "TestFont", FontSize: "12", Coordinate: "[0,0,1,1]", Content: "Delta Delta Delta Delta"},
	}

	chunks, err := BuildChunks(lines, ChunkOptions{ChunkSize: 35, OverlapPercent: 50})
	if err != nil {
		t.Fatalf("BuildChunks: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("chunks=%d, want at least 2 to exercise overlap", len(chunks))
	}
	for _, chunk := range chunks {
		overlap, regular := chunkLineNumbers(chunk)
		for _, got := range append(overlap, regular...) {
			if got == 3 || got == 4 {
				t.Fatalf("chunk %d includes TOC line %d; overlap=%v regular=%v", chunk.SeqNo, got, overlap, regular)
			}
		}
		for _, ml := range chunk.Lines {
			if strings.EqualFold(strings.TrimSpace(ml.Line.LineType), "toc") {
				t.Fatalf("chunk %d includes TOC marked line: %+v", chunk.SeqNo, ml)
			}
		}
	}
}

func TestValidateChunkSizeSanityRejectsShortNonFinalChunk(t *testing.T) {
	chunks := []Chunk{
		{
			SeqNo: 1,
			Lines: []MarkedLine{
				{Line: Line{LineNo: 890, PageNo: 1, LineType: "paragraph", Font: "TestFont", FontSize: "12", Coordinate: "[0,0,1,1]", Content: strings.Repeat("alpha ", 30)}, Mark: "n"},
			},
		},
		{
			SeqNo: 2,
			Lines: []MarkedLine{
				{Line: Line{LineNo: 898, PageNo: 1, LineType: "paragraph", Font: "TestFont", FontSize: "12", Coordinate: "[0,0,1,1]", Content: "context"}, Mark: "o"},
				{Line: Line{LineNo: 905, PageNo: 1, LineType: "paragraph", Font: "TestFont", FontSize: "12", Coordinate: "[0,0,1,1]", Content: "short"}, Mark: "n"},
			},
		},
		{
			SeqNo: 3,
			Lines: []MarkedLine{
				{Line: Line{LineNo: 906, PageNo: 1, LineType: "paragraph", Font: "TestFont", FontSize: "12", Coordinate: "[0,0,1,1]", Content: strings.Repeat("omega ", 30)}, Mark: "n"},
			},
		},
	}

	err := validateChunkSizeSanity(chunks, 100)
	if err == nil {
		t.Fatal("validateChunkSizeSanity returned nil, want error")
	}
	for _, want := range []string{"non-final chunk 2", "regular_bytes=", "min_regular_bytes=80", "lines=[905]"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error=%q, want substring %q", err.Error(), want)
		}
	}
}

func TestValidateChunkSizeSanityAllowsShortFinalChunk(t *testing.T) {
	chunks := []Chunk{
		{
			SeqNo: 1,
			Lines: []MarkedLine{
				{Line: Line{LineNo: 1, PageNo: 1, LineType: "paragraph", Font: "TestFont", FontSize: "12", Coordinate: "[0,0,1,1]", Content: strings.Repeat("alpha ", 30)}, Mark: "n"},
			},
		},
		{
			SeqNo: 2,
			Lines: []MarkedLine{
				{Line: Line{LineNo: 2, PageNo: 1, LineType: "paragraph", Font: "TestFont", FontSize: "12", Coordinate: "[0,0,1,1]", Content: "short"}, Mark: "n"},
			},
		},
	}

	if err := validateChunkSizeSanity(chunks, 100); err != nil {
		t.Fatalf("validateChunkSizeSanity: %v", err)
	}
}

func TestValidateChunkSizeSanityRejectsLargeMultiLineOverlap(t *testing.T) {
	chunks := []Chunk{
		{
			SeqNo: 1,
			Lines: []MarkedLine{
				{Line: Line{LineNo: 1, PageNo: 1, LineType: "paragraph", Font: "TestFont", FontSize: "12", Coordinate: "[0,0,1,1]", Content: strings.Repeat("overlap ", 10)}, Mark: "o"},
				{Line: Line{LineNo: 2, PageNo: 1, LineType: "paragraph", Font: "TestFont", FontSize: "12", Coordinate: "[0,0,1,1]", Content: strings.Repeat("overlap ", 10)}, Mark: "o"},
				{Line: Line{LineNo: 3, PageNo: 1, LineType: "paragraph", Font: "TestFont", FontSize: "12", Coordinate: "[0,0,1,1]", Content: strings.Repeat("regular ", 30)}, Mark: "n"},
			},
		},
	}

	err := validateChunkSizeSanity(chunks, 100)
	if err == nil {
		t.Fatal("validateChunkSizeSanity returned nil, want error")
	}
	for _, want := range []string{"chunk 1", "overlap_bytes=", "max_overlap_bytes=20", "overlap=[1-2]"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error=%q, want substring %q", err.Error(), want)
		}
	}
}

func TestBuildChunks_ExtendsNonFinalChunkWhenOverlapConsumesTarget(t *testing.T) {
	lines := []Line{
		{LineNo: 1, PageNo: 1, LineType: "paragraph", Font: "TestFont", FontSize: "12", Coordinate: "[0,0,1,1]", Content: "intro"},
		{LineNo: 2, PageNo: 1, LineType: "paragraph", Font: "TestFont", FontSize: "12", Coordinate: "[0,0,1,1]", Content: strings.Repeat("overlap ", 30)},
		{LineNo: 3, PageNo: 1, LineType: "paragraph", Font: "TestFont", FontSize: "12", Coordinate: "[0,0,1,1]", Content: "short"},
		{LineNo: 4, PageNo: 1, LineType: "paragraph", Font: "TestFont", FontSize: "12", Coordinate: "[0,0,1,1]", Content: strings.Repeat("regular ", 30)},
		{LineNo: 5, PageNo: 1, LineType: "paragraph", Font: "TestFont", FontSize: "12", Coordinate: "[0,0,1,1]", Content: strings.Repeat("tail ", 30)},
	}

	chunks, err := BuildChunks(lines, ChunkOptions{ChunkSize: 100, OverlapPercent: 50})
	if err != nil {
		t.Fatalf("BuildChunks: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("chunks=%d, want at least 2", len(chunks))
	}

	overlap, regular := chunkLineNumbers(chunks[1])
	if got := formatLineNumberRanges(overlap); got != "[2]" {
		t.Fatalf("chunk 2 overlap=%s, want [2]", got)
	}
	if got := formatLineNumberRanges(regular); got != "[3-4]" {
		t.Fatalf("chunk 2 regular=%s, want [3-4]", got)
	}
}

func TestBuildChunks_TrimsOverlapToTwentyPercentOrOneLine(t *testing.T) {
	lines := make([]Line, 0, 12)
	for lineNo := 1; lineNo <= 12; lineNo++ {
		lines = append(lines, Line{
			LineNo:     lineNo,
			PageNo:     1,
			LineType:   "paragraph",
			Font:       "TestFont",
			FontSize:   "12",
			Coordinate: "[0,0,1,1]",
			Content:    "abcdefghij",
		})
	}

	chunks, err := BuildChunks(lines, ChunkOptions{ChunkSize: 100, OverlapPercent: 90})
	if err != nil {
		t.Fatalf("BuildChunks: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("chunks=%d, want at least 2", len(chunks))
	}

	overlap, _ := chunkLineNumbers(chunks[1])
	if len(overlap) != 1 {
		t.Fatalf("chunk 2 overlap=%v, want one overlap line", overlap)
	}
}

func TestService_HandleInput_WritesChunksAndStatus(t *testing.T) {
	t.Setenv("EMBEDDING_MODEL_NAME", "test-summary-embed-model")
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
	logger := &fakeLogger{}
	svc := NewFixedSizeChunkingService(st, ex, nil)
	svc.Logger = logger
	svc.ChunkDir = tmp
	svc.ArtifactWebDir = t.TempDir()
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
	var leafSummaryInputs []string
	svc.GenerateSummary = func(_ context.Context, _ int64, level int, seqNo int, lines []MarkedLine, children []SummaryItem) (summaryGenerateResult, error) {
		if level == 0 {
			leafSummaryInputs = append(leafSummaryInputs, buildMarkedChunkInputText(lines))
			return summaryGenerateResult{Summary: "chunk summary " + asString(seqNo), CategoryPaths: []string{"legacy_summary_tree"}}, nil
		}
		return summaryGenerateResult{Summary: "parent summary " + strings.Join(collectSummaryIDs(children), ","), CategoryPaths: []string{"legacy_summary_tree"}}, nil
	}

	if err := svc.HandleInput(context.Background(), 7523, "sample.txt", []byte(input)); err != nil {
		t.Fatalf("HandleInput: %v", err)
	}
	if ex.calls != 0 {
		t.Fatalf("extractor calls=%d, want 0", ex.calls)
	}
	if len(ex.inputs) != 0 {
		t.Fatalf("extractor inputs=%d, want 0", len(ex.inputs))
	}
	if len(leafSummaryInputs) != 0 {
		t.Fatalf("leafSummaryInputs=%d, want 0", len(leafSummaryInputs))
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
	if _, err := os.Stat(chunkPath); err != nil {
		t.Fatalf("missing chunk artifact: %v", err)
	}
	logEntry, ok := findInfoLog(logger.infos, "chunk file generated")
	if !ok {
		t.Fatalf("expected \"chunk file generated\" log entry, got %#v", logger.infos)
	}
	if got, ok := logValue(logEntry.args, "chunk_file"); !ok || got != chunkPath {
		t.Fatalf("chunk_file=%v, ok=%v, want %q", got, ok, chunkPath)
	}
	if got, ok := logValue(logEntry.args, "num_chunks"); !ok || got != len(ex.outs) {
		t.Fatalf("num_chunks=%v, ok=%v, want %d", got, ok, len(ex.outs))
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

	var status []map[string]any
	if err := json.Unmarshal([]byte(st.updatedStatus), &status); err != nil {
		t.Fatalf("status json: %v", err)
	}
	if len(status) < 1 {
		t.Fatalf("expected chunk status entry, got %d", len(status))
	}
	var chunkStatus map[string]any
	var summaryStatus map[string]any
	for _, entry := range status {
		switch entry["operation"] {
		case "chunked":
			chunkStatus = entry
		case "generate_summaries":
			summaryStatus = entry
		}
	}
	if chunkStatus == nil {
		t.Fatalf("missing chunked status entry: %#v", status)
	}
	if summaryStatus != nil {
		t.Fatalf("unexpected generate_summaries status during chunk-only phase: %#v", summaryStatus)
	}
	if chunkStatus["record_id"] != "7523" {
		t.Fatalf("chunk record_id=%v, want 7523", chunkStatus["record_id"])
	}
	if chunkStatus["file_type"] != "pdf" {
		t.Fatalf("chunk file_type=%v, want pdf", chunkStatus["file_type"])
	}
	if chunkStatus["proc_status"] != "success" {
		t.Fatalf("chunk proc_status=%v, want success", chunkStatus["proc_status"])
	}
	if got := int(toFloat(chunkStatus["num_labeled_lines"])); got != 3 {
		t.Fatalf("num_labeled_lines=%d, want 3", got)
	}
	if _, err := os.Stat(filepath.Join(tmp, "7", "7523", "std_20039_opendata.topics")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("topic artifact should not exist after chunk-only phase, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(treeRoot, "document_overview", "closing_notes", "topics.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("topic tree should not exist after chunk-only phase, err=%v", err)
	}
}

func TestFixedSizeChunkingService_SummaryEmbeddingUsesEmbeddingModelConfig(t *testing.T) {
	tmp := t.TempDir()

	goodServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2,0.3]}]}`))
	}))
	defer goodServer.Close()

	modelsPath := filepath.Join(tmp, ".models.toml")
	modelsBody := `
[good-embedding]
host = "cloud"
model_name = "text-embedding-v4"
api_key = "sk-test"
base_url = "` + goodServer.URL + `"
timeout_sec = 5
`
	if err := os.WriteFile(modelsPath, []byte(modelsBody), 0o644); err != nil {
		t.Fatalf("write models file: %v", err)
	}

	t.Setenv("MODELS_FILE", modelsPath)
	t.Setenv("EMBEDDING_MODEL_NAME", "good-embedding")
	t.Setenv("EMBEDDING_DIMENSIONS", "3")

	svc := NewFixedSizeChunkingService(&fakeStore{}, &fakeSemanticExtractor{}, nil)
	svc.ChunkDir = tmp

	err := svc.embedAndWriteSummaries(context.Background(), 416, []SummaryItem{{
		SummaryID: "416_sum_0_0001",
		Level:     0,
		SeqNo:     1,
		Summary:   "summary text",
	}})
	if err != nil {
		t.Fatalf("embedAndWriteSummaries: %v", err)
	}

	embedPath := filepath.Join(tmp, "0", "416", "embeddings", summaryEmbedFileName(0, 1))
	if _, err := os.Stat(embedPath); err != nil {
		t.Fatalf("missing summary embedding file: %v", err)
	}
}

func TestService_HandleGenerateTopicsInput_LoadsChunkArtifactWithTOCLines(t *testing.T) {
	t.Setenv("EMBEDDING_MODEL_NAME", "test-summary-embed-model")
	tmp := t.TempDir()
	treeRoot := t.TempDir()
	input := strings.Join([]string{
		"1\t1\ttoc\tTestFont\t12\t[0,0,1,1]\tTable of Contents",
		"2\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tAlpha",
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
						"keywords":      []any{"alpha"},
						"topic":         "Alpha section",
						"category_path": []any{"document_overview"},
					},
				},
			},
		},
	}
	svc := NewFixedSizeChunkingService(st, ex, nil)
	svc.ChunkDir = tmp
	svc.ArtifactWebDir = treeRoot
	svc.ModelErr = nil
	svc.PromptErr = nil
	svc.SummaryModelErr = nil
	svc.SummaryPromptErr = nil
	svc.ModelName = "topic-model"
	svc.PromptText = "extract chunk topics"

	targetDir := filepath.Join(tmp, "7", "7523")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	chunkArtifact := "overlap: []\nlines: [1-2]\n"
	if err := os.WriteFile(filepath.Join(targetDir, "std_20039_opendata.chunks"), []byte(chunkArtifact), 0o644); err != nil {
		t.Fatalf("write chunk artifact: %v", err)
	}

	if err := svc.HandleGenerateTopicsInput(context.Background(), 7523, "sample.txt", []byte(input)); err != nil {
		t.Fatalf("HandleGenerateTopicsInput: %v", err)
	}
	if ex.calls+ex.structuredCalls != 1 {
		t.Fatalf("extractor calls=%d structured_calls=%d, want total 1", ex.calls, ex.structuredCalls)
	}
	if len(ex.inputs) != 1 {
		t.Fatalf("extractor inputs=%d, want 1", len(ex.inputs))
	}
	if !strings.Contains(ex.inputs[0], "Table of Contents") {
		t.Fatalf("extractor input missing TOC line from artifact: %q", ex.inputs[0])
	}
	if _, err := os.Stat(filepath.Join(targetDir, "std_20039_opendata.topics")); err != nil {
		t.Fatalf("missing topics artifact: %v", err)
	}
}

func TestLoadChunksFromArtifactFile_IgnoresStaleTrailingLineNumbers(t *testing.T) {
	tmp := t.TempDir()
	targetDir := filepath.Join(tmp, "7", "7523")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	chunkArtifact := "overlap: []\nlines: [303-304]\n"
	if err := os.WriteFile(filepath.Join(targetDir, "std_20039_opendata.chunks"), []byte(chunkArtifact), 0o644); err != nil {
		t.Fatalf("write chunk artifact: %v", err)
	}

	lines := []Line{
		{LineNo: 303, PageNo: 12, LineType: "paragraph", Content: "Last line"},
	}
	chunks, err := loadChunksFromArtifactFile(tmp, 7523, "std_20039_opendata.chunks", lines)
	if err != nil {
		t.Fatalf("loadChunksFromArtifactFile: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("chunks=%d, want 1", len(chunks))
	}
	if len(chunks[0].Lines) != 1 {
		t.Fatalf("chunk lines=%d, want 1", len(chunks[0].Lines))
	}
	if got := chunks[0].Lines[0].Line.LineNo; got != 303 {
		t.Fatalf("line no=%d, want 303", got)
	}
}

// TestLoadChunksFromArtifactFile_SkipsTOCLinesFromStaleArtifact ensures that
// line numbers present in an old artifact but absent from the filtered input
// (e.g. TOC lines that were recorded before BuildChunks started filtering them)
// are silently skipped rather than causing an error.  This is the regression
// scenario for record 162: artifact has "lines: [1-25]" where line 25 is toc,
// but the block-based topic-gen path passes TOC-filtered lines to
// loadChunksFromArtifactFile, so line 25 is not in lineByNo.
func TestLoadChunksFromArtifactFile_SkipsTOCLinesFromStaleArtifact(t *testing.T) {
	tmp := t.TempDir()
	targetDir := filepath.Join(tmp, "0", "162")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// Artifact written by old code: lines 1-25, where line 25 is a toc line.
	chunkArtifact := "overlap: []\nlines: [1-25]\n\noverlap: [21-25]\nlines: [26-30]\n"
	if err := os.WriteFile(filepath.Join(targetDir, "std_33830_opendata.chunks"), []byte(chunkArtifact), 0o644); err != nil {
		t.Fatalf("write chunk artifact: %v", err)
	}

	// lineByNo built by ParseBlockBufferLines — TOC lines are filtered out.
	lines := make([]Line, 0, 30)
	for i := 1; i <= 30; i++ {
		lt := "paragraph"
		if i == 25 {
			lt = "toc" // this line is absent from the filtered set
			continue
		}
		lines = append(lines, Line{LineNo: i, PageNo: 1, LineType: lt, Content: "text"})
	}

	chunks, err := loadChunksFromArtifactFile(tmp, 162, "std_33830_opendata.chunks", lines)
	if err != nil {
		t.Fatalf("loadChunksFromArtifactFile returned error: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("chunks=%d, want 2", len(chunks))
	}
	// Chunk 1: lines 1-24 (25 skipped as absent/TOC)
	for _, ml := range chunks[0].Lines {
		if ml.Line.LineNo == 25 {
			t.Fatalf("chunk 1 contains TOC line 25 — should have been skipped")
		}
	}
	// Chunk 2 overlap: lines 21-24 (25 skipped); regular: lines 26-30
	for _, ml := range chunks[1].Lines {
		if ml.Line.LineNo == 25 {
			t.Fatalf("chunk 2 contains TOC line 25 in overlap — should have been skipped")
		}
	}
}

func TestLoadChunksFromArtifactFile_ErrorsOnEmptyChunkFromStaleArtifact(t *testing.T) {
	tmp := t.TempDir()
	targetDir := filepath.Join(tmp, "0", "162")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	chunkArtifact := "overlap: []\nlines: [25]\n\noverlap: []\nlines: [26-27]\n"
	if err := os.WriteFile(filepath.Join(targetDir, "std_33830_opendata.chunks"), []byte(chunkArtifact), 0o644); err != nil {
		t.Fatalf("write chunk artifact: %v", err)
	}

	lines := []Line{
		{LineNo: 26, PageNo: 1, LineType: "paragraph", Content: "Alpha"},
		{LineNo: 27, PageNo: 1, LineType: "paragraph", Content: "Beta"},
	}

	chunks, err := loadChunksFromArtifactFile(tmp, 162, "std_33830_opendata.chunks", lines)
	if err == nil {
		t.Fatalf("loadChunksFromArtifactFile returned nil error, chunks=%v", chunks)
	}
	if !strings.Contains(err.Error(), "references only missing/filtered lines") {
		t.Fatalf("error=%v, want missing/filtered lines error", err)
	}
	for _, want := range []string{"seq 1", "overlap=[]", "regular=[25]", "missing_regular=[25]"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error=%v, want substring %q", err, want)
		}
	}
}

func TestService_HandleGenerateTopicsInput_ReadsChunksAndWritesTopics(t *testing.T) {
	t.Setenv("EMBEDDING_MODEL_NAME", "test-summary-embed-model")
	tmp := t.TempDir()
	treeRoot := t.TempDir()
	input := strings.Join([]string{
		"1\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tAlpha",
		"2\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tBeta",
		"3\t2\tparagraph\tTestFont\t12\t[0,0,1,1]\tGamma",
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
	svc.ArtifactWebDir = treeRoot
	svc.ChunkSize = 25
	svc.OverlapPercent = 50
	svc.ModelErr = nil
	svc.PromptErr = nil
	svc.ModelName = "topic-model"
	svc.PromptText = "extract chunk topics"
	svc.SummaryModelErr = nil
	svc.SummaryPromptErr = nil

	if err := svc.HandleInput(context.Background(), 7523, "sample.txt", []byte(input)); err != nil {
		t.Fatalf("HandleInput: %v", err)
	}
	st.rec.StatusRaw = st.updatedStatus
	st.updatedStatus = ""
	st.updatedError = nil
	st.updateCalls = 0

	if err := svc.HandleGenerateTopicsInput(context.Background(), 7523, "sample.txt", []byte(input)); err != nil {
		t.Fatalf("HandleGenerateTopicsInput: %v", err)
	}
	if ex.structuredCalls != 2 {
		t.Fatalf("structuredCalls=%d, want 2", ex.structuredCalls)
	}
	if ex.calls != 0 {
		t.Fatalf("legacy calls=%d, want 0", ex.calls)
	}
	if len(ex.inputs) != 2 {
		t.Fatalf("extractor inputs=%d, want 2", len(ex.inputs))
	}
	if !strings.Contains(ex.inputs[0], `"flag":"n"`) || !strings.Contains(ex.inputs[0], `"line_number":1`) || !strings.Contains(ex.inputs[0], `"content":"Alpha"`) {
		t.Fatalf("first topic chunk missing flagged line format: %q", ex.inputs[0])
	}
	if !strings.Contains(ex.inputs[1], `"flag":"o"`) || !strings.Contains(ex.inputs[1], `"line_number":2`) || !strings.Contains(ex.inputs[1], `"content":"Beta"`) {
		t.Fatalf("second topic chunk missing overlap flag format: %q", ex.inputs[1])
	}

	topicPath := filepath.Join(tmp, "7", "7523", "std_20039_opendata.topics")
	if _, err := os.Stat(topicPath); err != nil {
		t.Fatalf("missing topic artifact: %v", err)
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
	treeLeaf := filepath.Join(treeRoot, "document_overview", "closing_notes", "topics.txt")
	treeContent, err := os.ReadFile(treeLeaf)
	if err != nil {
		t.Fatalf("read topic tree leaf: %v", err)
	}
	wantTopicLines := []string{"record_id: 7523", `topic_type: "policy"`, "Ending section"}
	for _, want := range wantTopicLines {
		if !strings.Contains(string(treeContent), want) {
			t.Fatalf("expected tree leaf to contain %q, got: %q", want, string(treeContent))
		}
	}

	var status []map[string]any
	if err := json.Unmarshal([]byte(st.updatedStatus), &status); err != nil {
		t.Fatalf("status json: %v", err)
	}
	var topicStatus map[string]any
	for _, entry := range status {
		if entry["operation"] == "generate_topics" {
			topicStatus = entry
			break
		}
	}
	if topicStatus == nil {
		t.Fatalf("missing generate_topics status entry: %#v", status)
	}
	if topicStatus["proc_status"] != "success" {
		t.Fatalf("topic proc_status=%v, want success", topicStatus["proc_status"])
	}
}

func TestFixedSizeChunkingService_GenerateSummaryUsesStructuredContractWhenAvailable(t *testing.T) {
	t.Setenv("EMBEDDING_MODEL_NAME", "test-summary-embed-model")
	ex := &fakeSemanticExtractor{
		outs: []map[string]any{
			{
				"summary":       "Alarm handling summary",
				"summary_en":    "Alarm handling summary",
				"keywords":      []any{"alarm", "handling"},
				"category_path": []any{"operations", "alarm_handling"},
			},
		},
	}
	svc := NewFixedSizeChunkingService(&fakeStore{}, ex, nil)
	svc.SummaryPromptText = "summary prompt"
	svc.SummaryModelName = "summary-model"

	result, err := svc.generateSummary(context.Background(), 101, 0, 1, []MarkedLine{
		{
			Line: Line{
				LineNo:   1,
				PageNo:   1,
				LineType: "paragraph",
				Content:  "Operator acknowledges the alarm and logs the incident.",
			},
			Mark: "n",
		},
	}, nil)
	if err != nil {
		t.Fatalf("generateSummary: %v", err)
	}
	if ex.structuredCalls != 1 {
		t.Fatalf("structuredCalls=%d, want 1", ex.structuredCalls)
	}
	if ex.calls != 0 {
		t.Fatalf("legacy calls=%d, want 0", ex.calls)
	}
	if len(ex.contractNames) != 1 || ex.contractNames[0] != "chenweb_summary_extraction" {
		t.Fatalf("contractNames=%v, want [chenweb_summary_extraction]", ex.contractNames)
	}
	if result.Summary != "Alarm handling summary" {
		t.Fatalf("Summary=%q, want Alarm handling summary", result.Summary)
	}
	if len(result.CategoryPaths) != 2 || result.CategoryPaths[0] != "operations" {
		t.Fatalf("CategoryPaths=%v", result.CategoryPaths)
	}
}

func TestFixedSizeChunkingService_GenerateSummaryMissingCategoryDoesNotWarn(t *testing.T) {
	t.Setenv("EMBEDDING_MODEL_NAME", "test-summary-embed-model")
	ex := &fakeSemanticExtractor{
		outs: []map[string]any{
			{
				"summary":    "Alarm handling summary",
				"summary_en": "Alarm handling summary",
				"keywords":   []any{"alarm", "handling"},
			},
		},
	}
	logger := &fakeLogger{}
	svc := NewFixedSizeChunkingService(&fakeStore{}, ex, nil)
	svc.Logger = logger
	svc.SummaryPromptText = "summary prompt"
	svc.SummaryModelName = "summary-model"

	result, err := svc.generateSummary(context.Background(), 101, 0, 1, []MarkedLine{
		{
			Line: Line{
				LineNo:   1,
				PageNo:   1,
				LineType: "paragraph",
				Content:  "Operator acknowledges the alarm and logs the incident.",
			},
			Mark: "n",
		},
	}, nil)
	if err != nil {
		t.Fatalf("generateSummary: %v", err)
	}
	if len(result.CategoryPaths) != 0 {
		t.Fatalf("CategoryPaths=%v, want empty", result.CategoryPaths)
	}
	if len(logger.warns) != 0 {
		t.Fatalf("warns=%v, want none", logger.warns)
	}
}

func TestFixedSizeChunkingService_GenerateSummaryLogsReasonWhenSummaryMissing(t *testing.T) {
	t.Setenv("EMBEDDING_MODEL_NAME", "test-summary-embed-model")
	ex := &fakeSemanticExtractor{
		outs: []map[string]any{
			{
				"keywords":      []any{"alarm", "handling"},
				"category_path": []any{"operations", "alarm_handling"},
			},
		},
	}
	logger := &fakeLogger{}
	svc := NewFixedSizeChunkingService(&fakeStore{}, ex, nil)
	svc.Logger = logger
	svc.SummaryPromptText = "summary prompt"
	svc.SummaryModelName = "summary-model"

	result, err := svc.generateSummary(context.Background(), 101, 0, 1, []MarkedLine{
		{
			Line: Line{
				LineNo:   1,
				PageNo:   1,
				LineType: "paragraph",
				Content:  "Operator acknowledges the alarm and logs the incident.",
			},
			Mark: "n",
		},
	}, nil)
	if err != nil {
		t.Fatalf("generateSummary: %v", err)
	}
	if strings.TrimSpace(result.Summary) == "" {
		t.Fatal("expected fallback summary to be populated")
	}
	entry, ok := findInfoLog(logger.errors, "failed retrieving summary")
	if !ok {
		t.Fatalf("expected failed retrieving summary log, got errors=%v", logger.errors)
	}
	reason, ok := logValue(entry.args, "reason")
	if !ok {
		t.Fatalf("expected reason in log args=%v", entry.args)
	}
	if got := strings.TrimSpace(reason.(string)); got == "" {
		t.Fatalf("expected non-empty reason, got %q", got)
	}
	parsed, ok := logValue(entry.args, "parsed_response")
	if !ok {
		t.Fatalf("expected parsed_response in log args=%v", entry.args)
	}
	parsedMap, ok := parsed.(map[string]any)
	if !ok {
		t.Fatalf("parsed_response type=%T, want map[string]any", parsed)
	}
	if _, ok := parsedMap["keywords"]; !ok {
		t.Fatalf("parsed_response=%v, want keywords payload", parsedMap)
	}
}

func TestFixedSizeChunkingService_GenerateSummaryErrorsOnEmptyInput(t *testing.T) {
	t.Setenv("EMBEDDING_MODEL_NAME", "test-summary-embed-model")
	ex := &fakeSemanticExtractor{
		outs: []map[string]any{
			{
				"summary": "should not be used",
			},
		},
	}
	svc := NewFixedSizeChunkingService(&fakeStore{}, ex, nil)
	svc.SummaryPromptText = "summary prompt"
	svc.SummaryModelName = "summary-model"

	_, err := svc.generateSummary(context.Background(), 101, 0, 1, nil, nil)
	if err == nil {
		t.Fatal("generateSummary returned nil error")
	}
	if !strings.Contains(err.Error(), "empty summary input") {
		t.Fatalf("error=%v, want empty summary input error", err)
	}
	if ex.calls != 0 || ex.structuredCalls != 0 {
		t.Fatalf("extractor should not be called, calls=%d structuredCalls=%d", ex.calls, ex.structuredCalls)
	}
}

func TestFixedSizeChunkingService_GenerateSummaryTranslatesMissingEnglishCategoryPaths(t *testing.T) {
	t.Setenv("EMBEDDING_MODEL_NAME", "test-summary-embed-model")
	ex := &fakeJSONExtractor{
		outs: []map[string]any{
			{
				"summary":  "该文本介绍了气体防护站设计规范。",
				"keywords": []any{"气体防护站", "设计规范"},
				"category_paths": []any{
					map[string]any{
						"path_keywords":   []any{"气体防护站", "设计规范", "石油天然气"},
						"path_confidence": 0.82,
						"category_path": []any{
							map[string]any{"name": "工业安全", "keywords": []any{"安全规范", "防护标准"}, "confidence": 0.9},
							map[string]any{"name": "气体防护", "keywords": []any{"有毒气体", "应急救援"}, "confidence": 0.85},
							map[string]any{"name": "设计规范", "keywords": []any{"气防站", "装备配置", "定员"}, "confidence": 0.8},
						},
					},
				},
			},
			{
				"category_paths_en": []any{
					map[string]any{
						"path_keywords":   []any{"gas protection station", "design specification", "oil and gas"},
						"path_confidence": 0.82,
						"category_path": []any{
							map[string]any{"name": "Industrial Safety", "keywords": []any{"safety standards", "protection standards"}, "confidence": 0.9},
							map[string]any{"name": "Gas Protection", "keywords": []any{"toxic gas", "emergency rescue"}, "confidence": 0.85},
							map[string]any{"name": "Design Specification", "keywords": []any{"gas defense station", "equipment configuration", "staffing"}, "confidence": 0.8},
						},
					},
				},
			},
		},
	}
	svc := NewFixedSizeChunkingService(&fakeStore{}, ex, nil)
	svc.SummaryPromptText = "summary prompt"
	svc.SummaryModelName = "summary-model"
	svc.TranslationEnabled = true
	svc.TranslationModelName = "translation-model"

	result, err := svc.generateSummary(context.Background(), 124, 1, 1, []MarkedLine{
		{
			Line: Line{
				LineNo:   1,
				PageNo:   1,
				LineType: "paragraph",
				Content:  "该文本介绍了中国石油天然气行业标准SY/T 6772-2009《气体防护站设计规范》的主要内容。",
			},
			Mark: "n",
		},
	}, nil)
	if err != nil {
		t.Fatalf("generateSummary: %v", err)
	}
	if ex.structuredCalledCount != 2 {
		t.Fatalf("structuredCalledCount=%d, want 2", ex.structuredCalledCount)
	}
	if len(ex.contractNames) != 2 || ex.contractNames[1] != "chenweb_summary_category_translation" {
		t.Fatalf("contractNames=%v", ex.contractNames)
	}
	if len(result.CategoryPathItemsEn) != 1 {
		t.Fatalf("CategoryPathItemsEn=%v, want translated path", result.CategoryPathItemsEn)
	}
	if got := result.CategoryPathItemsEn[0].Nodes[0].Name; got != "Industrial Safety" {
		t.Fatalf("translated first node=%q", got)
	}
	if got := result.CategoryPathItems[0].Nodes[0].Name; got != "工业安全" {
		t.Fatalf("original first node=%q", got)
	}
	if len(ex.modelNames) != 2 || ex.modelNames[1] != "translation-model" {
		t.Fatalf("modelNames=%v", ex.modelNames)
	}
}

func TestFixedSizeChunkingService_FixSummarySourceLanguage_TranslatesKeywordsToSourceLanguage(t *testing.T) {
	t.Setenv("EMBEDDING_MODEL_NAME", "test-summary-embed-model")
	ex := &fakeJSONExtractor{
		outs: []map[string]any{
			{"summary": "这是英文摘要的中文翻译。"},
			{"keywords": []any{"安全", "健康"}},
		},
	}
	svc := NewFixedSizeChunkingService(&fakeStore{}, ex, nil)
	svc.ChunkDir = t.TempDir()
	svc.TranslationEnabled = true
	svc.TranslationModelName = "translation-model"

	summaries := []SummaryItem{
		{
			SummaryID:  "124_0_0001",
			RecordID:   124,
			Level:      0,
			SeqNo:      1,
			Lines:      []string{"1-2"},
			Summary:    "This summary is in English.",
			SummaryEn:  "This summary is in English.",
			Keywords:   []string{"safety", "health"},
			KeywordsEn: []string{"safety", "health"},
		},
	}
	if _, err := writeSummaryFile(svc.ChunkDir, summaries[0].RecordID, summaries[0]); err != nil {
		t.Fatalf("writeSummaryFile: %v", err)
	}

	got, err := svc.fixSummarySourceLanguage(context.Background(), "zh", summaries)
	if err != nil {
		t.Fatalf("fixSummarySourceLanguage: %v", err)
	}
	if got[0].Summary != "这是英文摘要的中文翻译。" {
		t.Fatalf("Summary=%q", got[0].Summary)
	}
	if strings.Join(got[0].Keywords, ",") != "安全,健康" {
		t.Fatalf("Keywords=%v", got[0].Keywords)
	}
	if len(ex.contractNames) != 2 {
		t.Fatalf("contractNames=%v", ex.contractNames)
	}
	if ex.contractNames[0] != "chenweb_summary_text_translation" {
		t.Fatalf("first contract=%v", ex.contractNames)
	}
	if ex.contractNames[1] != "chenweb_summary_keywords_translation" {
		t.Fatalf("second contract=%v", ex.contractNames)
	}
}

func TestFixedSizeChunkingService_FixSummarySourceLanguage_BackfillsMissingKeywordsEnBeforeTranslating(t *testing.T) {
	t.Setenv("EMBEDDING_MODEL_NAME", "test-summary-embed-model")
	ex := &fakeJSONExtractor{
		outs: []map[string]any{
			{"keywords": []any{"生活垃圾分类", "可回收物", "有害垃圾", "其他垃圾", "垃圾分类要求", "易腐垃圾处置"}},
		},
	}
	svc := NewFixedSizeChunkingService(&fakeStore{}, ex, nil)
	svc.ChunkDir = t.TempDir()
	svc.TranslationEnabled = true
	svc.TranslationModelName = "translation-model"

	keywords := []string{
		"household waste classification",
		"recyclable waste",
		"hazardous waste",
		"other waste",
		"waste sorting requirements",
		"perishable waste disposal",
	}
	summaries := []SummaryItem{
		{
			SummaryID: "416_sum_0_0004",
			RecordID:  416,
			Level:     0,
			SeqNo:     4,
			Lines:     []string{"61-86"},
			Summary:   "输入内容定义了家庭垃圾类别。",
			SummaryEn: "The input defines household waste categories.",
			Keywords:  keywords,
			Language:  "zh",
		},
	}
	if _, err := writeSummaryFile(svc.ChunkDir, summaries[0].RecordID, summaries[0]); err != nil {
		t.Fatalf("writeSummaryFile: %v", err)
	}

	got, err := svc.fixSummarySourceLanguage(context.Background(), "zh", summaries)
	if err != nil {
		t.Fatalf("fixSummarySourceLanguage: %v", err)
	}
	if strings.Join(got[0].KeywordsEn, "|") != strings.Join(keywords, "|") {
		t.Fatalf("KeywordsEn=%v, want copied English keywords %v", got[0].KeywordsEn, keywords)
	}
	wantKeywords := []string{"生活垃圾分类", "可回收物", "有害垃圾", "其他垃圾", "垃圾分类要求", "易腐垃圾处置"}
	if strings.Join(got[0].Keywords, "|") != strings.Join(wantKeywords, "|") {
		t.Fatalf("Keywords=%v, want %v", got[0].Keywords, wantKeywords)
	}
	if len(ex.contractNames) != 1 || ex.contractNames[0] != "chenweb_summary_keywords_translation" {
		t.Fatalf("contractNames=%v", ex.contractNames)
	}
	artifactDir, err := buildRecordArtifactDir(svc.ChunkDir, summaries[0].RecordID)
	if err != nil {
		t.Fatalf("buildRecordArtifactDir: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(artifactDir, "summary_0_0004.txt"))
	if err != nil {
		t.Fatalf("read summary file: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, `keywords_en: ["household waste classification", "recyclable waste", "hazardous waste", "other waste", "waste sorting requirements", "perishable waste disposal"]`) {
		t.Fatalf("summary file missing backfilled keywords_en: %q", text)
	}
	if !strings.Contains(text, `keywords: ["生活垃圾分类", "可回收物", "有害垃圾", "其他垃圾", "垃圾分类要求", "易腐垃圾处置"]`) {
		t.Fatalf("summary file missing translated keywords: %q", text)
	}
}

func TestFixedSizeChunkingService_FixSummarySourceLanguage_TranslatesEnglishSummaryWithoutSummaryEn(t *testing.T) {
	t.Setenv("EMBEDDING_MODEL_NAME", "test-summary-embed-model")
	ex := &fakeJSONExtractor{
		outs: []map[string]any{
			{"summary": "这是英文摘要的中文翻译。"},
		},
	}
	svc := NewFixedSizeChunkingService(&fakeStore{}, ex, nil)
	svc.ChunkDir = t.TempDir()
	svc.TranslationEnabled = true
	svc.TranslationModelName = "translation-model"

	summaries := []SummaryItem{
		{
			SummaryID: "177_0_0014",
			RecordID:  177,
			Level:     0,
			SeqNo:     14,
			Lines:     []string{"245-273"},
			Summary:   "The document outlines the assessment system for elderly ability.",
			Language:  "zh",
		},
	}
	if _, err := writeSummaryFile(svc.ChunkDir, summaries[0].RecordID, summaries[0]); err != nil {
		t.Fatalf("writeSummaryFile: %v", err)
	}

	got, err := svc.fixSummarySourceLanguage(context.Background(), "zh", summaries)
	if err != nil {
		t.Fatalf("fixSummarySourceLanguage: %v", err)
	}
	if got[0].Summary != "这是英文摘要的中文翻译。" {
		t.Fatalf("Summary=%q", got[0].Summary)
	}
	if got[0].SummaryEn != "The document outlines the assessment system for elderly ability." {
		t.Fatalf("SummaryEn=%q", got[0].SummaryEn)
	}

	artifactDir, err := buildRecordArtifactDir(svc.ChunkDir, summaries[0].RecordID)
	if err != nil {
		t.Fatalf("buildRecordArtifactDir: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(artifactDir, "summary_0_0014.txt"))
	if err != nil {
		t.Fatalf("read summary file: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, "summary_en_begin\nThe document outlines the assessment system for elderly ability.\nsummary_en_end") {
		t.Fatalf("summary file missing english summary block: %q", text)
	}
}

func TestFixedSizeChunkingService_FixSummarySourceLanguage_BackfillsMissingEnglishSummaryBlock(t *testing.T) {
	t.Setenv("EMBEDDING_MODEL_NAME", "test-summary-embed-model")
	ex := &fakeJSONExtractor{
		outs: []map[string]any{
			{"summary": "A social participation total of 0-2 indicates intact ability."},
		},
	}
	svc := NewFixedSizeChunkingService(&fakeStore{}, ex, nil)
	svc.ChunkDir = t.TempDir()
	svc.TranslationEnabled = true
	svc.TranslationModelName = "translation-model"

	summaries := []SummaryItem{
		{
			SummaryID: "177_0_0006",
			RecordID:  177,
			Level:     0,
			SeqNo:     6,
			Lines:     []string{"100-115"},
			Summary:   "社会参与总分0～2分为能力完好，3～7分为轻度受损，8～13分为中度受损，≥14分为重度受损。",
			Language:  "zh",
		},
	}
	if _, err := writeSummaryFile(svc.ChunkDir, summaries[0].RecordID, summaries[0]); err != nil {
		t.Fatalf("writeSummaryFile: %v", err)
	}

	got, err := svc.fixSummarySourceLanguage(context.Background(), "zh", summaries)
	if err != nil {
		t.Fatalf("fixSummarySourceLanguage: %v", err)
	}
	if got[0].Summary != "社会参与总分0～2分为能力完好，3～7分为轻度受损，8～13分为中度受损，≥14分为重度受损。" {
		t.Fatalf("Summary=%q", got[0].Summary)
	}
	if got[0].SummaryEn != "A social participation total of 0-2 indicates intact ability." {
		t.Fatalf("SummaryEn=%q", got[0].SummaryEn)
	}

	artifactDir, err := buildRecordArtifactDir(svc.ChunkDir, summaries[0].RecordID)
	if err != nil {
		t.Fatalf("buildRecordArtifactDir: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(artifactDir, "summary_0_0006.txt"))
	if err != nil {
		t.Fatalf("read summary file: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, "summary_en_begin\nA social participation total of 0-2 indicates intact ability.\nsummary_en_end") {
		t.Fatalf("summary file missing english summary block: %q", text)
	}
}

func TestService_HandleInput_MissingInputFilename(t *testing.T) {
	t.Setenv("EMBEDDING_MODEL_NAME", "test-summary-embed-model")
	st := &fakeStore{rec: InputRecord{ID: 1001, StatusRaw: "[]"}}
	svc := NewFixedSizeChunkingService(st, &fakeSemanticExtractor{}, nil)
	svc.ChunkDir = t.TempDir()
	svc.ArtifactWebDir = t.TempDir()
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
	t.Setenv("EMBEDDING_MODEL_NAME", "test-summary-embed-model")
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
	t.Setenv("EMBEDDING_MODEL_NAME", "test-summary-embed-model")
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

func TestService_HandleGenerateSummariesInput_WritesSummariesTree(t *testing.T) {
	t.Setenv("EMBEDDING_MODEL_NAME", "test-summary-embed-model")
	tmp := t.TempDir()
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
	svc.ArtifactWebDir = summaryTreeRoot
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
	svc.GenerateSummary = func(_ context.Context, _ int64, level int, seqNo int, lines []MarkedLine, children []SummaryItem) (summaryGenerateResult, error) {
		if level == 0 {
			return summaryGenerateResult{
				Summary:       "leaf summary " + asString(seqNo) + " lines=" + formatLineNumberRanges(chunkLineNosFromMarkedLines(lines)),
				CategoryPaths: []string{"Safety Overview", "Closing Notes"},
			}, nil
		}
		ids := make([]string, 0, len(children))
		for _, child := range children {
			ids = append(ids, child.SummaryID)
		}
		return summaryGenerateResult{
			Summary:       "parent summary " + asString(seqNo) + " children=" + strings.Join(ids, ","),
			CategoryPaths: []string{"Safety Overview", "Closing Notes"},
		}, nil
	}

	if err := svc.HandleInput(context.Background(), 8123, "sample.txt", []byte(input)); err != nil {
		t.Fatalf("HandleInput: %v", err)
	}
	st.rec.StatusRaw = st.updatedStatus
	st.updatedStatus = ""
	st.updatedError = nil
	st.updateCalls = 0

	if err := svc.HandleGenerateSummariesInput(context.Background(), 8123, "sample.txt", []byte(input)); err != nil {
		t.Fatalf("HandleGenerateSummariesInput: %v", err)
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

	var status []map[string]any
	if err := json.Unmarshal([]byte(st.updatedStatus), &status); err != nil {
		t.Fatalf("status json: %v", err)
	}
	var summaryStatus map[string]any
	for _, entry := range status {
		if entry["operation"] == "generate_summaries" {
			summaryStatus = entry
			break
		}
	}
	if summaryStatus == nil {
		t.Fatalf("missing generate_summaries status entry: %#v", status)
	}
	if summaryStatus["proc_status"] != "success" {
		t.Fatalf("summary proc_status=%v, want success", summaryStatus["proc_status"])
	}
	if summaryStatus["progress"] != "100% (3/3)" {
		t.Fatalf("summary progress=%v, want 100%% (3/3)", summaryStatus["progress"])
	}
	if st.updateCalls < 3 {
		t.Fatalf("updateCalls=%d, want at least 3 summary progress updates", st.updateCalls)
	}
	foundIntermediate := false
	for _, raw := range st.updateHistory {
		var entries []map[string]any
		if err := json.Unmarshal([]byte(raw), &entries); err != nil {
			t.Fatalf("status history json: %v", err)
		}
		for _, entry := range entries {
			if entry["operation"] == "generate_summaries" && entry["proc_status"] == "running" && entry["progress"] == "33% (1/3)" {
				foundIntermediate = true
				break
			}
		}
	}
	if !foundIntermediate {
		t.Fatalf("missing intermediate summary progress update in history: %#v", st.updateHistory)
	}
}

func TestService_HandleGenerateSummariesInput_BuildsChunksWhenChunkingWasNotRun(t *testing.T) {
	t.Setenv("EMBEDDING_MODEL_NAME", "test-summary-embed-model")
	tmp := t.TempDir()
	summaryTreeRoot := t.TempDir()
	input := strings.Join([]string{
		"1\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tAlpha",
		"2\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tBeta",
	}, "\n")

	st := &fakeStore{rec: InputRecord{
		ID:              8451,
		StatusRaw:       "[]",
		ParserName:      "opendata",
		StagingFilename: "sample.pdf",
	}}
	svc := NewFixedSizeChunkingService(st, nil, nil)
	svc.ChunkDir = tmp
	svc.ArtifactWebDir = summaryTreeRoot
	svc.ChunkSize = 50
	svc.OverlapPercent = 0
	svc.SummaryModelName = "summary-model"
	svc.SummaryPromptText = "summarize chunk"
	svc.SummaryGroupSize = 4
	svc.GenerateSummary = func(_ context.Context, _ int64, level int, seqNo int, lines []MarkedLine, _ []SummaryItem) (summaryGenerateResult, error) {
		if level != 0 {
			t.Fatalf("unexpected non-leaf summary level=%d seq=%d", level, seqNo)
		}
		return summaryGenerateResult{
			Summary:       "leaf summary",
			CategoryPaths: []string{"Safety Overview"},
		}, nil
	}

	if err := svc.HandleGenerateSummariesInput(context.Background(), 8451, "sample.txt", []byte(input)); err != nil {
		t.Fatalf("HandleGenerateSummariesInput without chunking: %v", err)
	}

	recordDir := filepath.Join(tmp, "8", "8451")
	if _, err := os.Stat(filepath.Join(recordDir, "sample_opendata.chunks")); err != nil {
		t.Fatalf("missing generated chunk artifact: %v", err)
	}
	if _, err := os.Stat(filepath.Join(recordDir, "summary_0_0001.txt")); err != nil {
		t.Fatalf("missing summary artifact: %v", err)
	}

	treeLeaf := filepath.Join(summaryTreeRoot, "safety_overview", "summaries.txt")
	treeBody, err := os.ReadFile(treeLeaf)
	if err != nil {
		t.Fatalf("read summary tree leaf: %v", err)
	}
	if strings.TrimSpace(string(treeBody)) != buildSummaryID(8451, 0, 1) {
		t.Fatalf("unexpected summary tree content: %q", strings.TrimSpace(string(treeBody)))
	}
}

func TestService_HandleGenerateSummariesInput_LeafLinesIncludeSingleOverlap(t *testing.T) {
	t.Setenv("EMBEDDING_MODEL_NAME", "test-summary-embed-model")
	tmp := t.TempDir()
	recordDir := filepath.Join(tmp, "0", "173")
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(recordDir, "std_1351666_opendata.chunks"),
		[]byte("overlap: [784]\nlines: [785-804]\n"),
		0o644,
	); err != nil {
		t.Fatalf("write chunk artifact: %v", err)
	}

	inputLines := make([]string, 0, 21)
	for lineNo := 784; lineNo <= 804; lineNo++ {
		inputLines = append(inputLines, asString(lineNo)+"\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tLine "+asString(lineNo))
	}
	input := strings.Join(inputLines, "\n")
	st := &fakeStore{rec: InputRecord{
		ID:              173,
		ParserName:      "opendata",
		StagingFilename: "std_1351666.pdf",
	}}
	svc := NewFixedSizeChunkingService(st, nil, nil)
	svc.ChunkDir = tmp
	svc.ArtifactWebDir = t.TempDir()
	svc.SummaryModelErr = nil
	svc.SummaryPromptErr = nil
	svc.SummaryModelName = "summary-model"
	svc.SummaryPromptText = "summarize chunk"
	svc.SummaryGroupSize = 10
	svc.GenerateSummary = func(_ context.Context, _ int64, _ int, _ int, lines []MarkedLine, _ []SummaryItem) (summaryGenerateResult, error) {
		if got := formatLineNumberRanges(chunkLineNosFromMarkedLines(lines)); got != "[784-804]" {
			t.Fatalf("summary input lines=%s, want [784-804]", got)
		}
		return summaryGenerateResult{Summary: "summary"}, nil
	}

	if err := svc.HandleGenerateSummariesInput(context.Background(), 173, "std_1351666.txt", []byte(input)); err != nil {
		t.Fatalf("HandleGenerateSummariesInput: %v", err)
	}

	body, err := os.ReadFile(filepath.Join(recordDir, "summary_0_0001.txt"))
	if err != nil {
		t.Fatalf("read summary artifact: %v", err)
	}
	if !strings.Contains(string(body), "lines: [784-804]") {
		t.Fatalf("summary artifact=%q, want lines [784-804]", string(body))
	}
}

func TestService_HandleGenerateSummariesInput_SummaryGenerationFailure(t *testing.T) {
	t.Setenv("EMBEDDING_MODEL_NAME", "test-summary-embed-model")
	st := &fakeStore{rec: InputRecord{
		ID:              9001,
		StatusRaw:       "[]",
		ParserName:      "opendata",
		StagingFilename: "sample.pdf",
	}}
	svc := NewFixedSizeChunkingService(st, &fakeSemanticExtractor{}, nil)
	svc.ChunkDir = t.TempDir()
	svc.ArtifactWebDir = t.TempDir()
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
	svc.GenerateSummary = func(_ context.Context, _ int64, _ int, _ int, _ []MarkedLine, _ []SummaryItem) (summaryGenerateResult, error) {
		return summaryGenerateResult{}, errors.New("summary generator boom")
	}

	if err := svc.HandleInput(context.Background(), 9001, "sample.txt", []byte("1\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tx")); err != nil {
		t.Fatalf("HandleInput: %v", err)
	}
	st.rec.StatusRaw = st.updatedStatus
	st.updatedStatus = ""
	st.updatedError = nil

	err := svc.HandleGenerateSummariesInput(context.Background(), 9001, "sample.txt", []byte("1\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tx"))
	if err == nil {
		t.Fatalf("expected summary generation error")
	}
	if !strings.Contains(err.Error(), "summary generator boom") {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.updatedError == nil || !strings.Contains(*st.updatedError, "summary generator boom") {
		t.Fatalf("expected persisted summary generation error, got %v", st.updatedError)
	}
	var status []map[string]any
	if err := json.Unmarshal([]byte(st.updatedStatus), &status); err != nil {
		t.Fatalf("status json: %v", err)
	}
	var summaryStatus map[string]any
	for _, entry := range status {
		if entry["operation"] == "generate_summaries" {
			summaryStatus = entry
			break
		}
	}
	if summaryStatus == nil {
		t.Fatalf("missing generate_summaries failure status: %#v", status)
	}
	if summaryStatus["proc_status"] != "failed" {
		t.Fatalf("summary proc_status=%v, want failed", summaryStatus["proc_status"])
	}
	if summaryStatus["progress"] != "0% (0/1)" {
		t.Fatalf("summary progress=%v, want 0%% (0/1) on first-summary failure", summaryStatus["progress"])
	}
	if !strings.Contains(asString(summaryStatus["error"]), "summary generator boom") {
		t.Fatalf("summary error=%v, want summary generator boom", summaryStatus["error"])
	}
}

func TestTotalPlannedSummaries(t *testing.T) {
	tests := []struct {
		leafCount int
		groupSize int
		want      int
	}{
		{leafCount: 0, groupSize: 5, want: 0},
		{leafCount: 1, groupSize: 5, want: 1},
		{leafCount: 2, groupSize: 5, want: 3},
		{leafCount: 5, groupSize: 5, want: 6},
		{leafCount: 12, groupSize: 5, want: 16},
	}
	for _, tt := range tests {
		if got := totalPlannedSummaries(tt.leafCount, tt.groupSize); got != tt.want {
			t.Fatalf("totalPlannedSummaries(%d, %d)=%d, want %d", tt.leafCount, tt.groupSize, got, tt.want)
		}
	}
}

func TestFormatSummaryProgress(t *testing.T) {
	if got := formatSummaryProgress(2, 3); got != "66% (2/3)" {
		t.Fatalf("formatSummaryProgress(2, 3)=%q, want 66%% (2/3)", got)
	}
	if got := formatSummaryProgress(3, 3); got != "100% (3/3)" {
		t.Fatalf("formatSummaryProgress(3, 3)=%q, want 100%% (3/3)", got)
	}
}

func TestService_HandleGenerateSummariesInput_TranslatesEnglishFallbackKeywords(t *testing.T) {
	t.Setenv("EMBEDDING_MODEL_NAME", "test-summary-embed-model")
	ex := &fakeJSONExtractor{
		outs: []map[string]any{
			{"summary": "这是中文摘要。"},
			{"keywords": []any{"健康"}},
		},
	}
	st := &fakeStore{rec: InputRecord{
		ID:              9012,
		StatusRaw:       "[]",
		ParserName:      "opendata",
		StagingFilename: "sample.pdf",
		SourceLanguage:  "zh",
	}}
	svc := NewFixedSizeChunkingService(st, ex, nil)
	svc.ChunkDir = t.TempDir()
	svc.ArtifactWebDir = t.TempDir()
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
	svc.TranslationEnabled = true
	svc.TranslationModelName = "translation-model"
	svc.GenerateSummary = func(_ context.Context, _ int64, _ int, seqNo int, _ []MarkedLine, _ []SummaryItem) (summaryGenerateResult, error) {
		return summaryGenerateResult{
			Summary:       "This summary is in English.",
			SummaryEn:     "This summary is in English.",
			Keywords:      []string{"health"},
			KeywordsEn:    []string{"health"},
			CategoryPaths: []string{"Safety Overview"},
		}, nil
	}

	if err := svc.HandleInput(context.Background(), 9012, "sample.txt", []byte("1\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tx")); err != nil {
		t.Fatalf("HandleInput: %v", err)
	}
	st.rec.StatusRaw = st.updatedStatus
	st.updatedStatus = ""
	st.updatedError = nil

	if err := svc.HandleGenerateSummariesInput(context.Background(), 9012, "sample.txt", []byte("1\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tx")); err != nil {
		t.Fatalf("HandleGenerateSummariesInput: %v", err)
	}
	if st.updatedError != nil {
		t.Fatalf("expected no persisted error, got %v", *st.updatedError)
	}
	artifactDir, err := buildRecordArtifactDir(svc.ChunkDir, 9012)
	if err != nil {
		t.Fatalf("buildRecordArtifactDir: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(artifactDir, "summary_0_0001.txt"))
	if err != nil {
		t.Fatalf("read summary file: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, `keywords: ["健康"]`) {
		t.Fatalf("summary file missing translated keywords: %q", text)
	}
}

func TestAppendChunkedStatus_SanitizesInvalidUTF8Error(t *testing.T) {
	rawErr := errors.New(string([]byte{'b', 'a', 'd', ':', ' ', 0xe4, 0x2e, 0x6d}))
	statusRaw, err := appendChunkedStatus("[]", chunkStatusParams{
		RecordID:        42,
		FileType:        "pdf",
		InputFilename:   "sample.txt",
		NumPages:        1,
		NumLines:        2,
		NumLabeledLines: 2,
		NumChunks:       1,
		Start:           time.Unix(0, 0),
		DurationMs:      12,
		ProcErr:         rawErr,
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
	var status []map[string]any
	if err := json.Unmarshal([]byte(statusRaw), &status); err != nil {
		t.Fatalf("unmarshal status JSON: %v", err)
	}
	last := status[len(status)-1]
	if last["record_id"] != "42" {
		t.Fatalf("record_id=%v, want 42", last["record_id"])
	}
	if last["file_type"] != "pdf" {
		t.Fatalf("file_type=%v, want pdf", last["file_type"])
	}
	if got := int(toFloat(last["num_labeled_lines"])); got != 2 {
		t.Fatalf("num_labeled_lines=%d, want 2", got)
	}
}

func chunkLineNosFromMarkedLines(lines []MarkedLine) []int {
	out := make([]int, 0, len(lines))
	for _, line := range lines {
		out = append(out, line.Line.LineNo)
	}
	return out
}
