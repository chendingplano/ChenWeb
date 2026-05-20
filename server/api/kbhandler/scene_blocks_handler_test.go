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
		"evidence_lines":["12:14","18"],
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
	if string(record.EvidenceLines) != `["12:14","18"]` {
		t.Fatalf("EvidenceLines = %s", string(record.EvidenceLines))
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
		"evidence_lines":["9"," ",12],
		"keywords_en":["keep"," ",12],
		"states_en":"bad-shape"
	}`))

	if record.TitleEn != "" || record.SummaryEn != "" {
		t.Fatalf("unexpected english text fields: %#v", record)
	}
	if string(record.KeywordsEn) != `["keep"]` {
		t.Fatalf("KeywordsEn = %s", string(record.KeywordsEn))
	}
	if string(record.EvidenceLines) != `["9"]` {
		t.Fatalf("EvidenceLines = %s", string(record.EvidenceLines))
	}
	if !reflect.DeepEqual(record.StatesEn, json.RawMessage(nil)) {
		t.Fatalf("StatesEn = %v", record.StatesEn)
	}
}
