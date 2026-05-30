package docprocessing

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	llmclients "github.com/chendingplano/shared/go/api/llm"
)

type fakeSceneObjectsStore struct {
	exists       bool
	existsErr    error
	deleteErr    error
	upsertErr    error
	deleteCalled int
	upsertCalled int
	lastUpsert   UpsertSceneObjectRequest
}

func (f *fakeSceneObjectsStore) SceneObjectsExist(_ context.Context, _ int64) (bool, error) {
	if f.existsErr != nil {
		return false, f.existsErr
	}
	return f.exists, nil
}

func (f *fakeSceneObjectsStore) DeleteSceneObjectsByInputRecordID(_ context.Context, _ int64) (int64, error) {
	f.deleteCalled++
	if f.deleteErr != nil {
		return 0, f.deleteErr
	}
	return 0, nil
}

func (f *fakeSceneObjectsStore) UpsertSceneObject(_ context.Context, req UpsertSceneObjectRequest) error {
	f.upsertCalled++
	f.lastUpsert = req
	if f.upsertErr != nil {
		return f.upsertErr
	}
	return nil
}

func TestSceneBlocksProcessor_RetriesFallbackOnEmptyJSON(t *testing.T) {
	tmp := t.TempDir()
	lineFile := filepath.Join(tmp, "scene-lines.txt")
	lineBody := strings.Join([]string{
		"1\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tScene introduction",
		"2\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tOperator opens the access panel.",
	}, "\n")
	if err := os.WriteFile(lineFile, []byte(lineBody), 0o644); err != nil {
		t.Fatalf("write line file: %v", err)
	}

	inputStore := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              7001,
		ParserName:      "opendata",
		ResultFilename:  filepath.Join(tmp, "ocr_rslt_7001.json"),
		StagingFilename: filepath.Join(tmp, "source.pdf"),
		StatusRaw:       "[]",
	}}
	sceneStore := &fakeSceneObjectsStore{}
	extractor := &fakeJSONExtractor{
		errs: []error{
			fmt.Errorf("(MID_26050174) failed resolveScopedString, error:(MID_26052922) failed resolveScopedString, error:(MID_26050142) decode llm response: unexpected end of JSON input, json:{[]}"),
			nil,
			nil,
		},
		outs: []map[string]any{
			nil,
			{
				"candidates": []any{
					map[string]any{
						"scene_key":         "scene_1",
						"scene_type_hint":   "operation",
						"title":             "Open access panel",
						"summary_hint":      "Operator opens the access panel.",
						"evidence_quote":    "Operator opens the access panel.",
						"line_spans":        []any{"1-2"},
						"confidence":        0.8,
						"confidence_reason": "Direct statement",
					},
				},
			},
			{
				"scene_blocks": []any{
					map[string]any{
						"scene_id":   "scene-1",
						"scene_type": "operation",
						"title":      "Open access panel",
						"summary":    "Operator opens the access panel.",
					},
				},
			},
		},
	}

	p := NewSceneBlocksProcessor(inputStore, sceneStore, extractor, nil)
	p.MentionPromptText = "extract scene candidates"
	p.MentionPromptRef = "prompt-extract-scene-candidates-v1.md"
	p.MentionPromptErr = nil
	p.MentionModelErr = nil
	p.MentionModelName = "primary-model"
	p.RelationPromptText = "extract scene blocks"
	p.RelationPromptRef = "prompt-enrich-scene-blocks-v1.md"
	p.RelationPromptErr = nil
	p.RelationModelErr = nil
	p.RelationModelName = "primary-model"
	p.PromptText = "extract scene blocks"
	p.PromptRef = "prompt-enrich-scene-blocks-v1.md"
	p.PromptErr = nil
	p.ModelErr = nil
	p.ModelName = "primary-model"
	p.FallbackModelName = "fallback-model"
	p.ChunkSize = 200
	p.OverlapPercent = 0
	p.ArtifactDir = tmp

	payload := fmt.Sprintf(`{"record_id":"7001","force":true,"line_file_filename":%q}`, lineFile)
	if err := p.HandleEvent(context.Background(), []byte(payload)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}

	if extractor.structuredCalledCount != 3 {
		t.Fatalf("structuredCalledCount=%d, want 3", extractor.structuredCalledCount)
	}
	if extractor.calledCount != 0 {
		t.Fatalf("calledCount=%d, want 0", extractor.calledCount)
	}
	if !strings.Contains(extractor.inputText, `"content":"Scene introduction"`) {
		t.Fatalf("scene block input missing first source line: %q", extractor.inputText)
	}
	if !strings.Contains(extractor.inputText, `"content":"Operator opens the access panel."`) {
		t.Fatalf("scene block input missing second source line: %q", extractor.inputText)
	}
	if len(extractor.modelNames) != 3 {
		t.Fatalf("modelNames=%v", extractor.modelNames)
	}
	if extractor.modelNames[0] != "primary-model" {
		t.Fatalf("first model=%q, want primary-model", extractor.modelNames[0])
	}
	if extractor.modelNames[1] != "fallback-model" {
		t.Fatalf("second model=%q, want fallback-model", extractor.modelNames[1])
	}
	if extractor.modelNames[2] != "primary-model" {
		t.Fatalf("third model=%q, want primary-model", extractor.modelNames[2])
	}
	if sceneStore.upsertCalled != 1 {
		t.Fatalf("upsertCalled=%d, want 1", sceneStore.upsertCalled)
	}
	if got := strings.TrimSpace(sceneStore.lastUpsert.ModelName); got != "primary-model" {
		t.Fatalf("saved model=%q, want primary-model", got)
	}
	if got := strings.TrimSpace(asString(sceneStore.lastUpsert.SceneBlock["title"])); got != "Open access panel" {
		t.Fatalf("saved title=%q", got)
	}
	if inputStore.updateReq.ErrorMsg != nil {
		t.Fatalf("unexpected error msg: %v", *inputStore.updateReq.ErrorMsg)
	}

	var statusArr []map[string]any
	if err := json.Unmarshal([]byte(inputStore.updateReq.StatusRaw), &statusArr); err != nil {
		t.Fatalf("status json: %v", err)
	}
	last := statusArr[len(statusArr)-1]
	if got := strings.TrimSpace(asString(last["proc_status"])); got != "success" {
		t.Fatalf("proc_status=%q, want success", got)
	}
}

func TestSceneBlocksProcessor_UsesMultiPassAndMergesDuplicateCandidates(t *testing.T) {
	tmp := t.TempDir()
	lineFile := filepath.Join(tmp, "scene-lines.txt")
	lineBody := strings.Join([]string{
		"1\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tOperator opens the access panel.",
		"2\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tThe access panel exposes the wiring bay.",
		"3\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tOperator inspects the wiring bay.",
	}, "\n")
	if err := os.WriteFile(lineFile, []byte(lineBody), 0o644); err != nil {
		t.Fatalf("write line file: %v", err)
	}

	inputStore := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              7002,
		ParserName:      "opendata",
		ResultFilename:  filepath.Join(tmp, "ocr_rslt_7002.json"),
		StagingFilename: filepath.Join(tmp, "source.pdf"),
		StatusRaw:       "[]",
	}}
	sceneStore := &fakeSceneObjectsStore{}
	extractor := &fakeJSONExtractor{
		outs: []map[string]any{
			{
				"candidates": []any{
					map[string]any{
						"scene_key":         "access_panel_opening",
						"scene_type_hint":   "operation",
						"title":             "Open access panel",
						"summary_hint":      "Operator opens the access panel.",
						"evidence_quote":    "Operator opens the access panel.",
						"line_spans":        []any{"1-2"},
						"confidence":        0.82,
						"confidence_reason": "Directly stated workflow step",
					},
				},
			},
			{
				"candidates": []any{
					map[string]any{
						"scene_key":         "access_panel_opening",
						"scene_type_hint":   "operation",
						"title":             "Opening the access panel",
						"summary_hint":      "Access panel is opened for inspection.",
						"evidence_quote":    "Operator opens the access panel.",
						"line_spans":        []any{"2-3"},
						"confidence":        0.71,
						"confidence_reason": "Repeated in overlap",
					},
				},
			},
			{
				"candidates": []any{},
			},
			{
				"scene_blocks": []any{
					map[string]any{
						"scene_id":    "access_panel_opening",
						"scene_type":  "operation",
						"title":       "Open access panel",
						"summary":     "Operator opens the access panel to inspect the wiring bay.",
						"line_spans":  []any{"1-3"},
						"confidence":  0.93,
						"source_refs": []any{},
					},
				},
			},
		},
	}

	p := NewSceneBlocksProcessor(inputStore, sceneStore, extractor, nil)
	p.MentionPromptText = "extract scene candidates"
	p.MentionPromptRef = "prompt-extract-scene-candidates-v1.md"
	p.MentionPromptErr = nil
	p.MentionModelErr = nil
	p.MentionModelName = "mention-model"
	p.RelationPromptText = "enrich scene blocks"
	p.RelationPromptRef = "prompt-enrich-scene-blocks-v1.md"
	p.RelationPromptErr = nil
	p.RelationModelErr = nil
	p.RelationModelName = "relation-model"
	p.PromptText = p.RelationPromptText
	p.PromptRef = p.RelationPromptRef
	p.ModelName = p.RelationModelName
	p.ChunkSize = 2
	p.OverlapPercent = 50
	p.ArtifactDir = tmp

	payload := fmt.Sprintf(`{"record_id":"7002","force":true,"line_file_filename":%q}`, lineFile)
	if err := p.HandleEvent(context.Background(), []byte(payload)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}

	if extractor.structuredCalledCount != 4 {
		t.Fatalf("structuredCalledCount=%d, want 4", extractor.structuredCalledCount)
	}
	if extractor.calledCount != 0 {
		t.Fatalf("calledCount=%d, want 0", extractor.calledCount)
	}
	if got, want := extractor.modelNames, []string{"mention-model", "mention-model", "mention-model", "relation-model"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("modelNames=%v, want %v", got, want)
	}
	if sceneStore.upsertCalled != 1 {
		t.Fatalf("upsertCalled=%d, want 1", sceneStore.upsertCalled)
	}
	if got := strings.TrimSpace(asString(sceneStore.lastUpsert.ObjectID)); got != "7002_1" {
		t.Fatalf("objectID=%q, want 7002_1", got)
	}
	if got := strings.TrimSpace(asString(sceneStore.lastUpsert.ModelName)); got != "relation-model" {
		t.Fatalf("saved model=%q, want relation-model", got)
	}
	if got := strings.TrimSpace(asString(sceneStore.lastUpsert.SceneBlock["scene_id"])); got != "access_panel_opening" {
		t.Fatalf("scene_id=%q", got)
	}
}

func TestMergeSceneCandidates_DropsOverlapOnlyCandidates(t *testing.T) {
	overlapOnly := []sceneCandidateMention{
		{
			SceneKey:          "access_panel_opening",
			Title:             "Open access panel",
			SummaryHint:       "Operator opens the access panel.",
			EvidenceQuote:     "Operator opens the access panel.",
			LineSpans:         []string{"10"},
			Confidence:        0.8,
			ChunkIndex:        1,
			ChunkLines:        []MarkedLine{{Line: Line{LineNo: 10, PageNo: 1, LineType: "paragraph", Content: "Operator opens the access panel."}, Mark: "o"}},
			HasNormalEvidence: false,
		},
	}
	if got := mergeSceneCandidateMentions(overlapOnly); len(got) != 0 {
		t.Fatalf("overlap-only candidates=%d, want 0", len(got))
	}

	withNormal := append([]sceneCandidateMention(nil), overlapOnly...)
	withNormal = append(withNormal, sceneCandidateMention{
		SceneKey:          "access_panel_opening",
		Title:             "Opening the access panel",
		SummaryHint:       "Panel opening step",
		EvidenceQuote:     "Operator opens the access panel.",
		LineSpans:         []string{"10"},
		Confidence:        0.7,
		ChunkIndex:        2,
		ChunkLines:        []MarkedLine{{Line: Line{LineNo: 10, PageNo: 1, LineType: "paragraph", Content: "Operator opens the access panel."}, Mark: "r"}},
		HasNormalEvidence: true,
	})
	merged := mergeSceneCandidateMentions(withNormal)
	if len(merged) != 1 {
		t.Fatalf("merged candidates=%d, want 1", len(merged))
	}
	if got := merged[0].SceneKey; got != "access_panel_opening" {
		t.Fatalf("scene key=%q", got)
	}
}

func TestSceneBlocksProcessor_ExtractSceneBlocksWithModelAppliesModelConfig(t *testing.T) {
	client := &llmclients.OpenAIJSONClient{
		ModelName: "primary-model",
		APIKey:    "primary-key",
		BaseURL:   "https://primary.example.com",
	}
	p := &SceneBlocksProcessor{
		Extractor:  client,
		PromptText: "extract scene blocks",
	}

	_, _ = p.extractSceneBlocksWithModel(context.Background(), "hello", "fallback-model", structureModelConfig{
		ModelName:  "fallback-model",
		APIKey:     "fallback-key",
		BaseURL:    "https://fallback.example.com",
		TimeoutSec: 17,
	})

	if client.ModelName != "fallback-model" {
		t.Fatalf("ModelName=%q, want fallback-model", client.ModelName)
	}
	if client.APIKey != "fallback-key" {
		t.Fatalf("APIKey=%q, want fallback-key", client.APIKey)
	}
	if client.BaseURL != "https://fallback.example.com" {
		t.Fatalf("BaseURL=%q, want https://fallback.example.com", client.BaseURL)
	}
	if client.HTTPClient == nil {
		t.Fatalf("HTTPClient should be initialized")
	}
	if got := int(client.HTTPClient.Timeout.Seconds()); got != 17 {
		t.Fatalf("Timeout=%d, want 17", got)
	}
}

func TestSceneBlocksProcessor_ExtractScenePayloadUsesStructuredContractWhenAvailable(t *testing.T) {
	extractor := &fakeJSONExtractor{
		out: map[string]any{
			"scene_blocks": []any{
				map[string]any{
					"scene_id":   "access_panel_opening",
					"scene_type": "operation",
					"title":      "Open access panel",
					"summary":    "Operator opens the access panel.",
				},
			},
		},
	}

	p := &SceneBlocksProcessor{
		Extractor: extractor,
	}

	payload, err := p.extractScenePayload(context.Background(), "input lines", "prompt", "scene-model", structureModelConfig{})
	if err != nil {
		t.Fatalf("extractScenePayload: %v", err)
	}
	if extractor.structuredCalledCount != 1 {
		t.Fatalf("structuredCalledCount=%d, want 1", extractor.structuredCalledCount)
	}
	if extractor.calledCount != 0 {
		t.Fatalf("calledCount=%d, want 0", extractor.calledCount)
	}
	if len(extractor.contractNames) != 1 || extractor.contractNames[0] != "chenweb_scene_extraction" {
		t.Fatalf("contractNames=%v, want [chenweb_scene_extraction]", extractor.contractNames)
	}
	items, ok := payload["scene_blocks"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("scene_blocks=%#v", payload["scene_blocks"])
	}
}

func TestSceneBlocksProcessor_WriteSceneBlocksArtifactAddsCreateTimeToAllBlocks(t *testing.T) {
	tmp := t.TempDir()
	p := &SceneBlocksProcessor{ArtifactDir: tmp}
	rec := DocMetadataInputRecord{
		ParserName:      "opendata",
		StagingFilename: "/tmp/source.pdf",
	}
	sceneBlocks := []map[string]any{
		{"object_id": "7001_1", "title": "Open access panel"},
		{"object_id": "7001_2", "title": "Review system state"},
	}

	if err := p.writeSceneBlocksArtifact(7001, rec, sceneBlocks); err != nil {
		t.Fatalf("writeSceneBlocksArtifact: %v", err)
	}

	outPath := filepath.Join(tmp, "7", "7001", "source_opendata.scene_blocks")
	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}

	var got []map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal artifact: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("artifact block count=%d, want 2", len(got))
	}
	for i, block := range got {
		rawCreateTime := strings.TrimSpace(asString(block["create_time"]))
		if rawCreateTime == "" {
			t.Fatalf("block %d missing create_time: %#v", i, block)
		}
		if _, err := time.Parse("20060102-150405", rawCreateTime); err != nil {
			t.Fatalf("block %d invalid create_time %q: %v", i, rawCreateTime, err)
		}
	}
}
