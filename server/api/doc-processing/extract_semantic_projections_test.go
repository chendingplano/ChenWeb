package docprocessing

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestNormalizeSemanticProjection_Basic(t *testing.T) {
	raw := map[string]any{
		"language":            "zh",
		"descriptive_name":    "疫苗接种记录管理",
		"descriptive_name_en": "Vaccination Record Management",
		"keywords":            []any{"疫苗", "接种", "记录"},
		"keywords_en":         []any{"vaccine", "vaccination", "record"},
		"category_paths": []any{
			map[string]any{
				"category_path": []any{
					map[string]any{"name": "public_health", "keywords": []any{"health"}, "confidence": 0.95},
				},
				"path_keywords":   []any{"health"},
				"path_confidence": 0.95,
			},
		},
		"category_paths_en": []any{
			map[string]any{
				"category_path": []any{
					map[string]any{"name": "public_health", "keywords": []any{"health"}, "confidence": 0.95},
				},
				"path_keywords":   []any{"health"},
				"path_confidence": 0.95,
			},
		},
	}

	result := normalizeSemanticProjection(raw)

	if result["language"] != "zh" {
		t.Errorf("expected language=zh, got %v", result["language"])
	}
	if result["descriptive_name"] != "疫苗接种记录管理" {
		t.Errorf("expected descriptive_name, got %v", result["descriptive_name"])
	}
	if result["descriptive_name_en"] != "Vaccination Record Management" {
		t.Errorf("expected descriptive_name_en, got %v", result["descriptive_name_en"])
	}
	kw, ok := result["keywords"].([]string)
	if !ok || len(kw) != 3 {
		t.Errorf("expected 3 keywords, got %v", result["keywords"])
	}
	kwEn, ok := result["keywords_en"].([]string)
	if !ok || len(kwEn) != 3 {
		t.Errorf("expected 3 keywords_en, got %v", result["keywords_en"])
	}
	if result["category_paths"] == nil {
		t.Error("expected category_paths to be set")
	}
	if result["category_paths_en"] == nil {
		t.Error("expected category_paths_en to be set")
	}
}

func TestNormalizeSemanticProjection_EnglishSkipsEnFields(t *testing.T) {
	raw := map[string]any{
		"language":            "en",
		"descriptive_name":    "Vaccination Record Management",
		"descriptive_name_en": "Vaccination Record Management",
		"keywords":            []any{"vaccine", "vaccination", "record"},
		"keywords_en":         []any{"vaccine", "vaccination", "record"},
		"category_paths":      []any{},
		"category_paths_en":   []any{},
	}

	result := normalizeSemanticProjection(raw)

	if result["language"] != "en" {
		t.Errorf("expected language=en, got %v", result["language"])
	}
	// _en fields should still be normalized — language guard is at save time
	if result["descriptive_name"] != "Vaccination Record Management" {
		t.Errorf("expected descriptive_name, got %v", result["descriptive_name"])
	}
}

func TestAppendSemanticProjectionsStatus_Success(t *testing.T) {
	raw := "[]"
	from := semanticProjectionsStatusParams{
		RecordID:      42,
		FileType:      "pdf",
		InputFilename: "Artifacts/0/42/file.txt",
		DurationMs:    123,
	}
	out, err := appendSemanticProjectionsStatus(raw, from)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var entries []map[string]any
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	entry := entries[0]
	if entry["operation"] != "extract_semantic_projections" {
		t.Errorf("expected operation=extract_semantic_projections, got %v", entry["operation"])
	}
	if entry["proc_status"] != "success" {
		t.Errorf("expected proc_status=success, got %v", entry["proc_status"])
	}
	if entry["record_id"] != "42" {
		t.Errorf("expected record_id=42, got %v", entry["record_id"])
	}
	if _, hasErr := entry["error"]; hasErr {
		t.Error("success entry should not have error field")
	}
}

func TestAppendSemanticProjectionsStatus_Failure(t *testing.T) {
	raw := "[]"
	from := semanticProjectionsStatusParams{
		RecordID:      7,
		FileType:      "docx",
		InputFilename: "Artifacts/0/7/file.txt",
		DurationMs:    456,
		ProcErr:       fmt.Errorf("something went wrong"),
	}
	out, err := appendSemanticProjectionsStatus(raw, from)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var entries []map[string]any
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	entry := entries[0]
	if entry["proc_status"] != "failed" {
		t.Errorf("expected proc_status=failed, got %v", entry["proc_status"])
	}
	if entry["error"] != "something went wrong" {
		t.Errorf("expected error message, got %v", entry["error"])
	}
}

func TestAppendSemanticProjectionsStatus_Replaces(t *testing.T) {
	existing := `[{"operation":"extract_semantic_projections","proc_status":"failed","record_id":"5"}]`
	from := semanticProjectionsStatusParams{
		RecordID:   5,
		FileType:   "pdf",
		DurationMs: 10,
	}
	out, err := appendSemanticProjectionsStatus(existing, from)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var entries []map[string]any
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry after replace, got %d", len(entries))
	}
	if entries[0]["proc_status"] != "success" {
		t.Errorf("expected replaced entry to be success")
	}
}
