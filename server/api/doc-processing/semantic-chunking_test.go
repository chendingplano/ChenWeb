package docprocessing

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	llmclients "github.com/chendingplano/shared/go/api/llm"
)

func TestNewSemanticChunkingService_UsesExtractTopicModelEnv(t *testing.T) {
	t.Setenv("SEMANTIC_CHUNKING_MODEL_NAME", "")
	t.Setenv("EXTRACT_TOPIC_MODEL_NAME", "topic-extractor")

	logger := &fakeLogger{}
	svc := NewSemanticChunkingService(&fakeStore{}, &fakeSemanticExtractor{}, logger)
	if svc == nil {
		t.Fatalf("service is nil")
	}
	if svc.ModelErr != nil {
		t.Fatalf("ModelErr=%v, want nil", svc.ModelErr)
	}
	if svc.ModelRef != "topic-extractor" {
		t.Fatalf("ModelRef=%q, want topic-extractor", svc.ModelRef)
	}
	if len(logger.errors) != 0 {
		t.Fatalf("startup logged unexpected errors: %#v", logger.errors)
	}
}

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
	treeRoot := t.TempDir()
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
						"category_path": []any{
							"document_overview",
						},
					},
					map[string]any{
						"topic_type": "policy",
						"lines":      []any{"2"},
						"keywords":   []any{"section", "policy"},
						"topic":      "Section A",
						"category_path": []any{
							"document_overview",
							"section_a",
						},
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
						"category_path": []any{
							"data_tables",
						},
					},
					map[string]any{
						"topic_type": "formula",
						"lines":      []any{"4"},
						"keywords":   []any{"formula"},
						"topic":      "Equation",
						"category_path": []any{
							"document_overview",
							"equations",
						},
					},
				},
			},
		},
	}
	st := &fakeStore{rec: InputRecord{
		ID:              7523,
		StatusRaw:       "[]",
		ParserName:      "opendata",
		StagingFilename: "std_20039.pdf",
	}}
	svc := NewSemanticChunkingService(st, ex, nil)
	svc.ChunkDir = tmp
	svc.ArtifactWebDir = treeRoot
	svc.FileBlockSize = 2
	svc.ModelErr = nil
	svc.ModelName = "topic-model"

	if err := svc.HandleInput(context.Background(), 7523, "sample.txt", []byte(input)); err != nil {
		t.Fatalf("HandleInput: %v", err)
	}
	if ex.structuredCalls != 2 {
		t.Fatalf("structuredCalls=%d, want 2", ex.structuredCalls)
	}
	if ex.calls != 0 {
		t.Fatalf("legacy ExtractJSON calls=%d, want 0", ex.calls)
	}
	if st.insertCalls != 1 {
		t.Fatalf("InsertChunkRun calls=%d, want 1", st.insertCalls)
	}
	if st.insertedRun.ChunkingMethod != "topic-chunking" {
		t.Fatalf("chunking_method=%q, want topic-chunking", st.insertedRun.ChunkingMethod)
	}

	topicPath := filepath.Join(tmp, "7", "7523", "std_20039_opendata.topics")
	bs, err := os.ReadFile(topicPath)
	if err != nil {
		t.Fatalf("read .topics artifact: %v", err)
	}
	content := string(bs)
	if !strings.Contains(content, `topic_desc: "Cover page"`) {
		t.Fatalf(".topics content missing expected topic: %q", content)
	}
	treeLeaf := filepath.Join(treeRoot, "document_overview", "equations", "topics.txt")
	treeBS, err := os.ReadFile(treeLeaf)
	if err != nil {
		t.Fatalf("read tree leaf: %v", err)
	}
	if !strings.Contains(string(treeBS), `topic_desc: "Equation"`) {
		t.Fatalf("tree content missing expected topic: %q", string(treeBS))
	}
	if _, err := os.Stat(filepath.Join(tmp, "7", "7523", "document_overview")); !os.IsNotExist(err) {
		t.Fatalf("unexpected category directory under artifact dir, err=%v", err)
	}

	var status []map[string]any
	if err := json.Unmarshal([]byte(st.updatedStatus), &status); err != nil {
		t.Fatalf("status json: %v", err)
	}
	if len(status) == 0 {
		t.Fatalf("expected status entry")
	}
	last := status[len(status)-1]
	if strings.TrimSpace(asString(last["operation"])) != "generate_topics" {
		t.Fatalf("operation=%v, want generate_topics", last["operation"])
	}
	if strings.TrimSpace(asString(last["proc_status"])) != "success" {
		t.Fatalf("proc_status=%v, want success", last["proc_status"])
	}
	if got := strings.TrimSpace(asString(last["output_filename"])); got != topicPath {
		t.Fatalf("output_filename=%q, want %q", got, topicPath)
	}
}

func TestNormalizeAndValidateTopicCategoryPath(t *testing.T) {
	path, reason := normalizeAndValidateTopicCategoryPath([]string{"Safety Rules", "Seismic Design"}, "policy")
	if reason != "" {
		t.Fatalf("unexpected reason: %q", reason)
	}
	want := []string{"safety_rules", "seismic_design"}
	if !reflect.DeepEqual(path, want) {
		t.Fatalf("path=%v, want=%v", path, want)
	}

	path, reason = normalizeAndValidateTopicCategoryPath([]string{"general"}, "policy")
	if reason == "" {
		t.Fatalf("expected non-descriptive category reason")
	}
	if !reflect.DeepEqual(path, []string{"uncategorized", "policy"}) {
		t.Fatalf("fallback path=%v", path)
	}

	path, reason = normalizeAndValidateTopicCategoryPath([]string{"a", "b", "c", "d", "e", "f", "g"}, "table")
	if reason != "depth-exceeds-limit" {
		t.Fatalf("reason=%q, want depth-exceeds-limit", reason)
	}
	if !reflect.DeepEqual(path, []string{"uncategorized", "table"}) {
		t.Fatalf("fallback path=%v", path)
	}
}

func TestWriteTopicsCategoryTree_Deterministic(t *testing.T) {
	tmp := t.TempDir()
	topics := []TopicItem{
		{
			SeqNo:        1,
			TopicType:    "list",
			Lines:        []string{"11-12"},
			Keywords:     []string{"risk", "scoring"},
			Topic:        "Scoring table for seismic safety",
			CategoryPath: []string{"safety_evaluation", "seismic_design"},
		},
		{
			SeqNo:        2,
			TopicType:    "list",
			Lines:        []string{"9"},
			Keywords:     []string{"checklist"},
			Topic:        "Inspection checklist",
			CategoryPath: []string{"safety_evaluation", "seismic_design"},
		},
		{
			SeqNo:        3,
			TopicType:    "policy",
			Lines:        []string{"3"},
			Keywords:     []string{"scope"},
			Topic:        "Project scope",
			CategoryPath: []string{"project_overview"},
		},
	}

	if err := writeTopicsCategoryTree(nil, tmp, 53, topics); err != nil {
		t.Fatalf("writeTopicsCategoryTree: %v", err)
	}
	if err := writeTopicsCategoryTree(nil, tmp, 53, topics); err != nil {
		t.Fatalf("writeTopicsCategoryTree second run: %v", err)
	}

	root := filepath.Join(tmp, "0", "53")
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		files = append(files, strings.TrimPrefix(path, root+string(filepath.Separator)))
		return nil
	})
	if err != nil {
		t.Fatalf("walk root: %v", err)
	}
	sort.Strings(files)
	wantFiles := []string{
		"project_overview.txt",
		"safety_evaluation/seismic_design.txt",
	}
	if !reflect.DeepEqual(files, wantFiles) {
		t.Fatalf("files=%v, want=%v", files, wantFiles)
	}

	seismicPath := filepath.Join(root, "safety_evaluation", "seismic_design.txt")
	seismicContent, err := os.ReadFile(seismicPath)
	if err != nil {
		t.Fatalf("read seismic leaf: %v", err)
	}
	wantSeismic := strings.Join([]string{
		"53\tlist\t[11-12]\t[risk, scoring]\tScoring table for seismic safety",
		"53\tlist\t[9]\t[checklist]\tInspection checklist",
	}, "\n")
	if string(seismicContent) != wantSeismic {
		t.Fatalf("seismic content=%q, want=%q", string(seismicContent), wantSeismic)
	}

}

func TestWriteTopicsCategoryTree_ReprocessReplacesRowsForRecord(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "0", "53")
	if err := os.MkdirAll(filepath.Join(root, "safety_evaluation"), 0o755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	seed := strings.Join([]string{
		"53\tlist\t[1]\t[a]\tOld entry for same record",
		"99\tlist\t[2]\t[b]\tOther record entry",
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, "safety_evaluation", "seismic_design.txt"), []byte(seed), 0o644); err != nil {
		t.Fatalf("seed write: %v", err)
	}

	topics := []TopicItem{
		{
			SeqNo:        1,
			TopicType:    "list",
			Lines:        []string{"10"},
			Keywords:     []string{"new"},
			Topic:        "New entry for same record",
			CategoryPath: []string{"safety_evaluation", "seismic_design"},
		},
	}
	if err := writeTopicsCategoryTree(nil, tmp, 53, topics); err != nil {
		t.Fatalf("writeTopicsCategoryTree: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(root, "safety_evaluation", "seismic_design.txt"))
	if err != nil {
		t.Fatalf("read leaf: %v", err)
	}
	content := string(got)
	if strings.Contains(content, "Old entry for same record") {
		t.Fatalf("expected old row for record 53 to be replaced, got: %q", content)
	}
	if !strings.Contains(content, "99\tlist\t[2]\t[b]\tOther record entry") {
		t.Fatalf("expected other record row to remain, got: %q", content)
	}
	if !strings.Contains(content, "53\tlist\t[10]\t[new]\tNew entry for same record") {
		t.Fatalf("expected new record row to be present, got: %q", content)
	}
}

func TestWriteTopicsCategoryTree_PreservesCanonicalLineFile(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "0", "84")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}

	lineFile := filepath.Join(root, "stdGk_3029352_opendata.txt")
	lineBody := "1\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tOriginal line\n"
	if err := os.WriteFile(lineFile, []byte(lineBody), 0o644); err != nil {
		t.Fatalf("write line file: %v", err)
	}

	topics := []TopicItem{
		{
			SeqNo:        1,
			TopicType:    "policy",
			Lines:        []string{"1"},
			Keywords:     []string{"scope"},
			Topic:        "Project scope",
			CategoryPath: []string{"project_overview"},
		},
	}
	if err := writeTopicsCategoryTree(nil, tmp, 84, topics); err != nil {
		t.Fatalf("writeTopicsCategoryTree: %v", err)
	}

	bs, err := os.ReadFile(lineFile)
	if err != nil {
		t.Fatalf("read preserved line file: %v", err)
	}
	if string(bs) != lineBody {
		t.Fatalf("line file changed=%q, want %q", string(bs), lineBody)
	}
}

func TestSemanticChunkingService_HandleInput_LLMErrorPersistsFailure(t *testing.T) {
	input := "1\t1\theading\tTestFont\t12\t[0,0,1,1]\tCover"
	st := &fakeStore{rec: InputRecord{ID: 9001, StatusRaw: "[]"}}
	ex := &fakeSemanticExtractor{err: context.DeadlineExceeded}
	svc := NewSemanticChunkingService(st, ex, nil)
	svc.ChunkDir = t.TempDir()
	svc.ArtifactWebDir = t.TempDir()
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

func TestSemanticChunkingService_HandleInput_MissingArtifactWebDir(t *testing.T) {
	st := &fakeStore{rec: InputRecord{
		ID:              9002,
		StatusRaw:       "[]",
		ParserName:      "opendata",
		StagingFilename: "sample.pdf",
	}}
	svc := NewSemanticChunkingService(st, &fakeSemanticExtractor{}, nil)
	svc.ChunkDir = t.TempDir()
	svc.ArtifactWebDir = ""
	svc.FileBlockSize = 2
	svc.ModelErr = nil
	svc.PromptErr = nil
	svc.ModelName = "topic-model"
	svc.PromptText = "prompt"

	err := svc.HandleInput(context.Background(), 9002, "sample.txt", []byte("1\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tx"))
	if err == nil {
		t.Fatalf("expected error when ARTIFACT_WEB_DIR is empty")
	}
	if !strings.Contains(err.Error(), "missing ARTIFACT_WEB_DIR") {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.insertCalls != 0 {
		t.Fatalf("InsertChunkRun calls=%d, want 0", st.insertCalls)
	}
	if st.updateCalls != 1 {
		t.Fatalf("UpdateInputStatus calls=%d, want 1", st.updateCalls)
	}
	if st.updatedError == nil || !strings.Contains(*st.updatedError, "missing ARTIFACT_WEB_DIR") {
		t.Fatalf("expected persisted missing artifact web dir error, got %v", st.updatedError)
	}
}

func TestNewSemanticChunkingService_LoadsPromptFromEnv(t *testing.T) {
	tmp := t.TempDir()
	promptPath := filepath.Join(tmp, "topic_prompt.txt")
	if err := os.WriteFile(promptPath, []byte("custom topic prompt"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	t.Setenv("TOPIC_CHUNK_PROMPT", promptPath)

	svc := NewSemanticChunkingService(&fakeStore{}, &fakeSemanticExtractor{}, nil)
	if got := strings.TrimSpace(svc.PromptText); got != "custom topic prompt" {
		t.Fatalf("PromptText=%q, want custom topic prompt", got)
	}
	if svc.PromptErr != nil {
		t.Fatalf("PromptErr=%v, want nil", svc.PromptErr)
	}
	if svc.PromptPath != promptPath {
		t.Fatalf("PromptPath=%q, want %q", svc.PromptPath, promptPath)
	}
}

func TestNormalizeCategorySegment_CJK(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"消防员职业健康", "消防员职业健康"},
		{"Safety Overview", "safety_overview"},
		{"本标准规定了消防员", "本标准规定了消防员"},
		{"mixed 消防 content", "mixed_消防_content"},
		{"", ""},
		{"123", "123"},
	}
	for _, tc := range cases {
		got := normalizeCategorySegment(tc.input)
		if got != tc.want {
			t.Errorf("normalizeCategorySegment(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

type fakeSemanticExtractor struct {
	outs            []map[string]any
	err             error
	calls           int
	inputs          []string
	structuredCalls int
	contractNames   []string
}

func (f *fakeSemanticExtractor) ExtractJSON(_ context.Context, in llmclients.JSONExtractionInput) (map[string]any, error) {
	f.calls++
	f.inputs = append(f.inputs, in.InputText)
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

func (f *fakeSemanticExtractor) ExtractStructuredJSON(_ context.Context, in llmclients.JSONExtractionInput, contract llmclients.StructuredOutputContract) (*llmclients.StructuredOutputResult, error) {
	f.structuredCalls++
	f.contractNames = append(f.contractNames, contract.Name)
	f.inputs = append(f.inputs, in.InputText)
	if f.err != nil {
		return nil, f.err
	}
	if len(f.outs) == 0 {
		return &llmclients.StructuredOutputResult{Parsed: map[string]any{"topics": []any{}}}, nil
	}
	idx := f.structuredCalls - 1
	if idx >= len(f.outs) {
		idx = len(f.outs) - 1
	}
	return &llmclients.StructuredOutputResult{Parsed: f.outs[idx]}, nil
}

type fakeStructuredSemanticExtractor struct {
	fakeSemanticExtractor
}

func (f *fakeStructuredSemanticExtractor) ExtractStructuredJSON(_ context.Context, in llmclients.JSONExtractionInput, contract llmclients.StructuredOutputContract) (*llmclients.StructuredOutputResult, error) {
	f.structuredCalls++
	f.contractNames = append(f.contractNames, contract.Name)
	f.inputs = append(f.inputs, in.InputText)
	if f.err != nil {
		return nil, f.err
	}
	if len(f.outs) == 0 {
		return &llmclients.StructuredOutputResult{Parsed: map[string]any{"topics": []any{}}}, nil
	}
	return &llmclients.StructuredOutputResult{Parsed: f.outs[0]}, nil
}

func TestExtractTopicsFromLinesWithLLM_UsesStructuredContractWhenAvailable(t *testing.T) {
	extractor := &fakeStructuredSemanticExtractor{
		fakeSemanticExtractor: fakeSemanticExtractor{
			outs: []map[string]any{
				{
					"topics": []any{
						map[string]any{
							"topic_type": "requirement",
							"lines":      []any{"1-2"},
							"topic_desc": "alarm requirements",
						},
					},
				},
			},
		},
	}

	topics, err := extractTopicsFromLinesWithLLM(
		context.Background(),
		113,
		extractor,
		&fakeLogger{},
		"deepseek-v4-flash",
		"prompt",
		"prompt-extract-topics",
		[]Line{
			{LineNo: 1, PageNo: 1, LineType: "paragraph", Content: "The device shall log all alarms."},
		},
		1,
		"block_no",
		5,
	)
	if err != nil {
		t.Fatalf("extractTopicsFromLinesWithLLM: %v", err)
	}
	if extractor.structuredCalls != 1 {
		t.Fatalf("structuredCalls=%d, want 1", extractor.structuredCalls)
	}
	if extractor.calls != 0 {
		t.Fatalf("legacy ExtractJSON calls=%d, want 0", extractor.calls)
	}
	if len(extractor.contractNames) != 1 || extractor.contractNames[0] != "chenweb_topic_extraction" {
		t.Fatalf("contractNames=%v, want [chenweb_topic_extraction]", extractor.contractNames)
	}
	if len(topics) != 1 || topics[0].Topic != "alarm requirements" {
		t.Fatalf("topics=%#v", topics)
	}
}
