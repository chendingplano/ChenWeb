package kbhandler

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestApplySceneBlockExtInfoCopiesEnglishFields(t *testing.T) {
	record := sceneBlockRecord{
		Title:    "规范性引用文件的应用",
		Summary:  "标准中规范性引用文件的应用方式。",
		Keywords: json.RawMessage(`["引用","标准"]`),
		States:   json.RawMessage(`["有效"]`),
	}

	applySceneBlockExtInfo(&record, json.RawMessage(`{
		"title_en":"Application of normative references",
		"summary_en":"How normative references are applied in the standard.",
		"line_spans":["12:14","18"],
		"keywords_en":["references","standard"],
		"states_en":["active"]
	}`))

	if record.TitleEn != "Application of normative references" {
		t.Fatalf("TitleEn = %q", record.TitleEn)
	}
	if record.SummaryEn != "How normative references are applied in the standard." {
		t.Fatalf("SummaryEn = %q", record.SummaryEn)
	}
	if string(record.KeywordsEn) != `["references","standard"]` {
		t.Fatalf("KeywordsEn = %s", string(record.KeywordsEn))
	}
	if string(record.LineSpans) != `["12:14","18"]` {
		t.Fatalf("LineSpans = %s", string(record.LineSpans))
	}
	if string(record.StatesEn) != `["active"]` {
		t.Fatalf("StatesEn = %s", string(record.StatesEn))
	}
}

func TestApplySceneBlockExtInfoIgnoresBlankAndInvalidValues(t *testing.T) {
	record := sceneBlockRecord{}

	applySceneBlockExtInfo(&record, json.RawMessage(`{
		"title_en":"   ",
		"summary_en":"",
		"line_spans":["9"," ",12],
		"keywords_en":["keep"," ",12],
		"states_en":"bad-shape"
	}`))

	if record.TitleEn != "" || record.SummaryEn != "" {
		t.Fatalf("unexpected english text fields: %#v", record)
	}
	if string(record.KeywordsEn) != `["keep"]` {
		t.Fatalf("KeywordsEn = %s", string(record.KeywordsEn))
	}
	if string(record.LineSpans) != `["9"]` {
		t.Fatalf("LineSpans = %s", string(record.LineSpans))
	}
	if !reflect.DeepEqual(record.StatesEn, json.RawMessage(nil)) {
		t.Fatalf("StatesEn = %v", record.StatesEn)
	}
}

func TestApplySceneBlockExtInfoFallsBackToLegacyEvidenceLines(t *testing.T) {
	record := sceneBlockRecord{}

	applySceneBlockExtInfo(&record, json.RawMessage(`{
		"evidence_lines":["5-7","9"]
	}`))

	if string(record.LineSpans) != `["5-7","9"]` {
		t.Fatalf("LineSpans = %s", string(record.LineSpans))
	}
}
