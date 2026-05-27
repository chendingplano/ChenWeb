package docprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestNormalizeEntityRows(t *testing.T) {
	raw := []any{
		map[string]any{
			"entity":         "PostgreSQL",
			"entity_en":      "PostgreSQL",
			"entity_type":    "software_system",
			"entity_type_en": "software system",
			"aliases":        []any{"postgres", " pg "},
			"aliases_en":     []any{},
			"desc":           "relational database",
			"desc_en":        "",
			"keywords":       []any{"db", "rdbms"},
			"keywords_en":    []any{},
			"lines":          []any{"12", "14-16"},
			"confidence":     0.92,
		},
		map[string]any{
			// dropped: missing entity name
			"entity":     "  ",
			"desc":       "should be dropped",
			"confidence": 0.99,
		},
		"not-a-map",
	}
	out := normalizeEntityRows(raw, 3)
	if len(out) != 1 {
		t.Fatalf("expected 1 normalized entity, got %d: %#v", len(out), out)
	}
	got := out[0]
	if got["entity"] != "PostgreSQL" {
		t.Errorf("entity = %q, want PostgreSQL", got["entity"])
	}
	if got["chunk_seq_no"] != 3 {
		t.Errorf("chunk_seq_no = %v, want 3", got["chunk_seq_no"])
	}
	wantAliases := []string{"postgres", "pg"}
	if !reflect.DeepEqual(got["aliases"], wantAliases) {
		t.Errorf("aliases = %#v, want %#v", got["aliases"], wantAliases)
	}
	wantSpans := []string{"12", "14-16"}
	if !reflect.DeepEqual(got["line_spans"], wantSpans) {
		t.Errorf("line_spans = %#v, want %#v", got["line_spans"], wantSpans)
	}
}

func TestNormalizeRelationRows(t *testing.T) {
	raw := []any{
		map[string]any{
			"subject":      "temperature_monitoring_device",
			"subject_en":   "temperature monitoring device",
			"predicate":    "Triggers",
			"predicate_en": "triggers",
			"object":       "excursion_alarm",
			"object_en":    "excursion alarm",
			"desc":         "device triggers alarm",
			"keywords":     []any{"trigger", "alarm"},
			"lines":        []any{"40"},
			"confidence":   0.88,
		},
		map[string]any{
			// dropped: empty object
			"subject":   "x",
			"predicate": "y",
			"object":    "",
		},
	}
	out := normalizeRelationRows(raw, 7)
	if len(out) != 1 {
		t.Fatalf("expected 1 normalized relation, got %d: %#v", len(out), out)
	}
	got := out[0]
	if got["predicate"] != "triggers" {
		t.Errorf("predicate = %q, want triggers (lowercased+snake)", got["predicate"])
	}
	if got["chunk_seq_no"] != 7 {
		t.Errorf("chunk_seq_no = %v, want 7", got["chunk_seq_no"])
	}
}

func TestNormalizePredicate(t *testing.T) {
	cases := map[string]string{
		"":            "",
		"   ":         "",
		"triggers":    "triggers",
		"DEPENDS ON":  "depends_on",
		"  Has  Many": "has_many",
	}
	for in, want := range cases {
		if got := normalizePredicate(in); got != want {
			t.Errorf("normalizePredicate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsLanguageEnglish(t *testing.T) {
	if !isLanguageEnglish("en") {
		t.Error("en should be English")
	}
	if !isLanguageEnglish(" English ") {
		t.Error("English should be English")
	}
	if isLanguageEnglish("zh") {
		t.Error("zh should not be English")
	}
	if isLanguageEnglish("") {
		t.Error("empty should not be English")
	}
}

func TestAppendEntityRelationStatusAppendsNew(t *testing.T) {
	existing := `[{"operation":"chunked","proc_status":"success"}]`
	out, err := appendEntityRelationStatus(existing, entityRelationStatusParams{
		RecordID:      42,
		FileType:      "pdf",
		InputFilename: "f.txt",
		Start:         time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		DurationMs:    100,
		ProcErr:       nil,
	})
	if err != nil {
		t.Fatalf("append err: %v", err)
	}
	var arr []map[string]any
	if err := json.Unmarshal([]byte(out), &arr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(arr) != 2 {
		t.Fatalf("len=%d, want 2: %s", len(arr), out)
	}
	if arr[1]["operation"] != "extract_entity_relation" {
		t.Errorf("appended op = %v, want extract_entity_relation", arr[1]["operation"])
	}
	if arr[1]["proc_status"] != "success" {
		t.Errorf("proc_status = %v, want success", arr[1]["proc_status"])
	}
}

func TestAppendEntityRelationStatusReplacesExisting(t *testing.T) {
	existing := `[{"operation":"extract_entity_relation","proc_status":"failed","error":"old"}]`
	out, err := appendEntityRelationStatus(existing, entityRelationStatusParams{
		RecordID:   42,
		FileType:   "pdf",
		Start:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		DurationMs: 200,
		ProcErr:    nil,
	})
	if err != nil {
		t.Fatalf("append err: %v", err)
	}
	var arr []map[string]any
	if err := json.Unmarshal([]byte(out), &arr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(arr) != 1 {
		t.Fatalf("len=%d, want 1: %s", len(arr), out)
	}
	if arr[0]["proc_status"] != "success" {
		t.Errorf("proc_status = %v, want success (replaced)", arr[0]["proc_status"])
	}
	if _, ok := arr[0]["error"]; ok {
		t.Errorf("stale error field should be gone after replacement: %v", arr[0])
	}
}

func TestWriteEntityRelationArtifactFile(t *testing.T) {
	tmp := t.TempDir()
	rec := DocMetadataInputRecord{
		StagingFilename: "std_20039_opendata.pdf",
		ParserName:      "marker",
	}
	rows := []map[string]any{
		{"entity": "PostgreSQL", "desc": "a database"},
		{"entity": "FDA", "desc": "an agency"},
	}
	if err := writeEntityRelationArtifactFile(tmp, 100, rec, rows, ".entities", "MID_TEST"); err != nil {
		t.Fatalf("write: %v", err)
	}
	path := filepath.Join(tmp, "0", "100", "std_20039_opendata_marker.entities")
	bs, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var got []map[string]any
	if err := json.Unmarshal(bs, &got); err != nil {
		t.Fatalf("unmarshal: %v\nfile:\n%s", err, string(bs))
	}
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2", len(got))
	}
}

func TestWriteEntityRelationArtifactFileEmptyIsNoop(t *testing.T) {
	tmp := t.TempDir()
	rec := DocMetadataInputRecord{StagingFilename: "x.pdf", ParserName: "marker"}
	if err := writeEntityRelationArtifactFile(tmp, 100, rec, nil, ".entities", "MID_TEST"); err != nil {
		t.Fatalf("write nil: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "0")); !os.IsNotExist(err) {
		t.Errorf("expected no directory for empty rows, got err=%v", err)
	}
}

func TestEntityRelationExtractionContractShape(t *testing.T) {
	contract := entityRelationExtractionContract()
	if contract.Name == "" {
		t.Fatal("contract name empty")
	}
	if len(contract.Schema) == 0 {
		t.Fatal("contract schema empty")
	}
	var schema map[string]any
	if err := json.Unmarshal(contract.Schema, &schema); err != nil {
		t.Fatalf("schema unmarshal: %v", err)
	}
	props, _ := schema["properties"].(map[string]any)
	for _, key := range []string{"language", "entities", "relations"} {
		if _, ok := props[key]; !ok {
			t.Errorf("schema properties missing %q: %v", key, props)
		}
	}
}

func TestEntityRelationExtractionUsesFallbackOnPrimaryError(t *testing.T) {
	// fake pops one item per call from each slice. First call: (nil, error).
	// Second call (fallback model): (FDA payload, nil).
	fake := &fakeJSONExtractor{
		outs: []map[string]any{
			nil,
			{
				"language": "en",
				"entities": []any{
					map[string]any{"entity": "FDA", "desc": "agency"},
				},
				"relations": []any{},
			},
		},
		errs: []error{
			errors.New("primary llm transport error"),
			nil,
		},
	}
	p := &EntityRelationProcessor{
		Logger:            loggerutil.CreateDefaultLogger("TEST_ENT_REL"),
		Extractor:         fake,
		ModelName:         "primary",
		FallbackModelName: "fallback",
	}
	payload, modelName, err := p.extractEntityRelationWithFallback(context.Background(), "irrelevant")
	if err != nil {
		t.Fatalf("expected fallback to succeed, got err=%v", err)
	}
	if modelName != "fallback" {
		t.Errorf("modelName=%q, want fallback", modelName)
	}
	ents, _ := payload["entities"].([]any)
	if len(ents) != 1 {
		t.Errorf("entities len=%d, want 1: %#v", len(ents), payload)
	}
}
