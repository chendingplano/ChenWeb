package docprocessing

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

func extractProvisionTestPayload(t *testing.T, source string) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(source), &payload); err != nil {
		t.Fatal(err)
	}
	p := &ProvisionsProcessor{PromptRef: "test"}
	got, err := p.extractProvisionPayloadFromText(context.Background(), "input", "instructions", "test", &fakeJSONExtractor{out: payload})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestProvisionsCompactResponseDoesNotMutateCapturedPayload(t *testing.T) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(`{"output_format":"compact-v1","language":"zh","provisions":[{"source_line_spans":["42"],"confidence":0.9,"objects":[{"object_name":"垃圾"}]}]}`), &raw); err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(raw)
	p := &ProvisionsProcessor{PromptRef: "test"}
	if _, err := p.extractProvisionPayloadFromText(context.Background(), "input", "instructions", "test", &fakeJSONExtractor{out: raw}); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(raw)
	if string(before) != string(after) {
		t.Fatalf("captured response was mutated: %s", after)
	}
}

type compactProvisionStructuredExtractor struct {
	fakeJSONExtractor
}

func (e *compactProvisionStructuredExtractor) ExtractStructuredJSON(_ context.Context, _ llmclients.JSONExtractionInput, _ llmclients.StructuredOutputContract) (*llmclients.StructuredOutputResult, error) {
	return &llmclients.StructuredOutputResult{Parsed: e.out}, nil
}

func TestProvisionsCompactResponseStructuredAndFallback(t *testing.T) {
	const source = `{"output_format":"compact-v1","language":"zh","provisions":[{"source_line_spans":["42"],"confidence":0.9,"objects":[{"object_name":"垃圾"}]}]}`
	for _, structured := range []bool{false, true} {
		var raw map[string]any
		if err := json.Unmarshal([]byte(source), &raw); err != nil {
			t.Fatal(err)
		}
		var fallback LLMJSONExtractor = &fakeJSONExtractor{out: raw}
		if structured {
			fallback = &compactProvisionStructuredExtractor{fakeJSONExtractor{out: raw}}
		}
		p := &ProvisionsProcessor{PromptRef: "test", Logger: loggerutil.CreateDefaultLogger("20261107-342"), ModelName: "primary", Extractor: &fakeJSONExtractor{err: errors.New("retry")}, FallbackModelName: "fallback", FallbackExtractor: fallback}
		got, model, err := p.extractProvisionPayloadWithFallback(context.Background(), "input", "instructions")
		if err != nil || model != "fallback" {
			t.Fatalf("structured=%v model=%s err=%v", structured, model, err)
		}
		obj := got["provisions"].([]any)[0].(map[string]any)["objects"].([]any)[0].(map[string]any)
		if obj["object_name_zh"] != "垃圾" || obj["confidence"] != 0.9 {
			t.Fatalf("fallback response not expanded: %#v", obj)
		}
		originalObject := raw["provisions"].([]any)[0].(map[string]any)["objects"].([]any)[0].(map[string]any)
		if len(originalObject) != 1 {
			t.Fatal("provider's parsed response was mutated")
		}
	}
}

// Replay the ten archived extraction responses without a database or LLM calls.
// The fixture contains only their parsed content, not provider/account metadata.
func TestProvisionsCompactRun258Replay(t *testing.T) {
	f, err := os.Open("testdata/provisions-run-258.json.gz")
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
		Responses []map[string]any `json:"responses"`
	}
	if err := json.NewDecoder(z).Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	var originalChars, compactChars, count int
	p := &ProvisionsProcessor{PromptRef: "replay"}
	for _, original := range fixture.Responses {
		full, _ := json.Marshal(original)
		var compact map[string]any
		if err := json.Unmarshal(full, &compact); err != nil {
			t.Fatal(err)
		}
		compact["output_format"] = "compact-v1"
		items := compact["provisions"].([]any)
		for _, item := range items {
			provision := item.(map[string]any)
			for _, object := range objectItemsFromValue(provision["objects"]) {
				omitEmptyReplayFields(object)
				for key, inherited := range map[string]any{
					"object_name_zh": object["object_name"], "confidence": provision["confidence"], "source_line_spans": provision["source_line_spans"],
				} {
					if reflect.DeepEqual(object[key], inherited) {
						delete(object, key)
					}
				}
			}
			omitEmptyReplayFields(provision)
		}
		wire, _ := json.Marshal(compact)
		originalChars += utf8.RuneCount(full)
		compactChars += utf8.RuneCount(wire)
		expanded := extractProvisionTestPayload(t, string(wire))
		fullItems := original["provisions"].([]any)
		if !reflect.DeepEqual(expanded["provisions"], fullItems) {
			for i, fullItem := range fullItems {
				got := expanded["provisions"].([]any)[i].(map[string]any)
				for key, value := range fullItem.(map[string]any) {
					if !reflect.DeepEqual(got[key], value) {
						t.Errorf("provision %d field %s: got %#v, want %#v", i, key, got[key], value)
					}
				}
			}
			t.Fatal("expanded compact response differs from original response")
		}
		fullRows := p.buildProvisionOutputRows(416, normalizeProvisionList(fullItems, nil, nil), time.Unix(0, 0), 10, 0, "replay")
		compactRows := p.buildProvisionOutputRows(416, normalizeProvisionList(expanded["provisions"].([]any), nil, nil), time.Unix(0, 0), 10, 0, "replay")
		count += len(fullRows)
		for i, fullRow := range fullRows {
			compactRow := compactRows[i]
			for name, pair := range map[string][2]any{
				"storage":          {buildProvisionDBRecord(fullRow, "zh"), buildProvisionDBRecord(compactRow, "zh")},
				"artifact":         {buildProvisionFileRecord(fullRow), buildProvisionFileRecord(compactRow)},
				"objects":          {normalizeArtifactObjectsForArtifact(416, searchArtifactProvision, "replay", fullRow), normalizeArtifactObjectsForArtifact(416, searchArtifactProvision, "replay", compactRow)},
				"Chinese search":   {buildProvisionSearchDocument(fullRow, false), buildProvisionSearchDocument(compactRow, false)},
				"bilingual search": {buildProvisionSearchDocument(fullRow, true), buildProvisionSearchDocument(compactRow, true)},
			} {
				if !reflect.DeepEqual(pair[0], pair[1]) {
					t.Fatalf("%s changed for provision %d", name, i)
				}
			}
		}
	}
	if len(fixture.Responses) != 10 || count != 64 || compactChars >= originalChars {
		t.Fatalf("responses=%d provisions=%d original=%d compact=%d", len(fixture.Responses), count, originalChars, compactChars)
	}
	t.Logf("%d provisions: %d -> %d characters (%.2f%% smaller); stored rows, objects, artifacts, and search text identical", count, originalChars, compactChars, 100*(1-float64(compactChars)/float64(originalChars)))
}

func omitEmptyReplayFields(record map[string]any) {
	for key, value := range record {
		// Preserve the existing distinction between absent categories and [].
		if key == "category_paths" || key == "category_path_en" {
			continue
		}
		switch v := value.(type) {
		case string:
			if strings.TrimSpace(v) == "" {
				delete(record, key)
			}
		case []any:
			if len(v) == 0 {
				delete(record, key)
			}
		}
	}
}

func TestProvisionsCompactResponseRestoresObjectMetadata(t *testing.T) {
	got := extractProvisionTestPayload(t, `{"output_format":"compact-v1","language":"zh","provisions":[{
		"name":"垃圾收运","type":"mandatory","provision":"垃圾应每日收运。",
		"source_line_spans":["42-43"],"confidence":0.91,"is_explicit":true,"need_verify":false,
		"objects":[{"object_name":"垃圾","object_type":"material","object_role":"regulated_object"}]
	}]}`)
	p := got["provisions"].([]any)[0].(map[string]any)
	o := p["objects"].([]any)[0].(map[string]any)
	for key, want := range map[string]any{
		"object_name_zh": "垃圾", "confidence": 0.91, "source_line_spans": []any{"42-43"},
		"aliases": []any{}, "acronyms": []any{}, "description": "", "evidence_quote": "", "object_name_en": "",
	} {
		if !reflect.DeepEqual(o[key], want) {
			t.Errorf("object %s = %#v, want %#v", key, o[key], want)
		}
	}
	for key, want := range map[string]any{"context": "", "context_en": "", "keywords": []any{}} {
		if !reflect.DeepEqual(p[key], want) {
			t.Errorf("provision %s = %#v, want %#v", key, p[key], want)
		}
	}
}

func TestProvisionsCompactResponsePreservesExplicitOverrides(t *testing.T) {
	got := extractProvisionTestPayload(t, `{"output_format":"compact-v1","language":"zh","provisions":[{
		"source_line_spans":["42-43"],"confidence":0.91,"need_verify":false,
		"objects":[
			{"object_name":"waste","object_name_zh":"生活垃圾","source_line_spans":["43"],"confidence":0.8,"aliases":["垃圾"]},
			{"object_name":"CJJ 27","object_name_zh":"","source_line_spans":[],"confidence":0}
		]
	}]}`)
	p := got["provisions"].([]any)[0].(map[string]any)
	objects := p["objects"].([]any)
	for i, want := range []map[string]any{
		{"object_name_zh": "生活垃圾", "source_line_spans": []any{"43"}, "confidence": 0.8, "aliases": []any{"垃圾"}},
		{"object_name_zh": "", "source_line_spans": []any{}, "confidence": float64(0)},
	} {
		for key, value := range want {
			if got := objects[i].(map[string]any)[key]; !reflect.DeepEqual(got, value) {
				t.Errorf("object %d %s = %#v, want %#v", i, key, got, value)
			}
		}
	}
	if p["need_verify"] != false {
		t.Fatal("explicit false must survive")
	}
}

func TestProvisionsCompactResponseLanguageAndLegacyCompatibility(t *testing.T) {
	for _, tc := range []struct{ language, format, wantZh string }{
		{"zh", "compact-v1", "垃圾"}, {"zh-CN", "compact-v1", "垃圾"}, {"zh-Hans", "compact-v1", "垃圾"},
		{"Chinese", "compact-v1", "垃圾"}, {"en", "compact-v1", ""}, {"unknown", "compact-v1", ""},
		{"zh", "", ""}, {"zh", "future-format", ""},
	} {
		t.Run(tc.language+"/"+tc.format, func(t *testing.T) {
			source, _ := json.Marshal(map[string]any{"language": tc.language, "output_format": tc.format,
				"provisions": []any{map[string]any{"objects": []any{map[string]any{"object_name": "垃圾"}}}}})
			got := extractProvisionTestPayload(t, string(source))
			p := got["provisions"].([]any)[0].(map[string]any)
			o := p["objects"].([]any)[0].(map[string]any)
			if asString(o["object_name_zh"]) != tc.wantZh {
				t.Errorf("object_name_zh = %#v, want %q", o["object_name_zh"], tc.wantZh)
			}
			if tc.format != "compact-v1" && (len(p) != 1 || len(o) != 1) {
				t.Fatalf("legacy payload was expanded: %#v", got)
			}
		})
	}
}
