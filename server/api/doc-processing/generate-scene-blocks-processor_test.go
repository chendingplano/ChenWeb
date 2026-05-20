package docprocessing

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
			fmt.Errorf("(MID_26050174) failed resolveScopedString, error:(MID_26050177) failed resolveScopedString, error:(MID_26050142) decode llm response: unexpected end of JSON input, json:{[]}"),
			nil,
		},
		outs: []map[string]any{
			nil,
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
	p.PromptText = "extract scene blocks"
	p.PromptRef = "prompt-generate-scene-blocks-v2.md"
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

	if extractor.calledCount != 2 {
		t.Fatalf("calledCount=%d, want 2", extractor.calledCount)
	}
	if !strings.Contains(extractor.inputText, "n\t1\t1\tparagraph\tScene introduction") {
		t.Fatalf("scene block input missing flagged line format: %q", extractor.inputText)
	}
	if !strings.Contains(extractor.inputText, "n\t2\t1\tparagraph\tOperator opens the access panel.") {
		t.Fatalf("scene block input missing second flagged line: %q", extractor.inputText)
	}
	if len(extractor.modelNames) != 2 {
		t.Fatalf("modelNames=%v", extractor.modelNames)
	}
	if extractor.modelNames[0] != "primary-model" {
		t.Fatalf("first model=%q, want primary-model", extractor.modelNames[0])
	}
	if extractor.modelNames[1] != "fallback-model" {
		t.Fatalf("second model=%q, want fallback-model", extractor.modelNames[1])
	}
	if sceneStore.upsertCalled != 1 {
		t.Fatalf("upsertCalled=%d, want 1", sceneStore.upsertCalled)
	}
	if got := strings.TrimSpace(sceneStore.lastUpsert.ModelName); got != "fallback-model" {
		t.Fatalf("saved model=%q, want fallback-model", got)
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
