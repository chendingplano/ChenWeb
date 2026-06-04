package docprocessing

import (
	"strings"
	"testing"
)

func TestBuildProvisionSearchDocument_DedupesSourceTextAndProvision(t *testing.T) {
	source := "设有应急照明的场所，疏散照明的照度值不应低于10lx。"

	got := buildProvisionSearchDocument(map[string]any{
		"prov_name":             "应急照明疏散照度值",
		"prov_name_en":          "emergency_lighting_evacuation_illuminance",
		"provision_type":        "mandatory",
		"source_text":           source,
		"provision":             source,
		"provision_en":          "In places with emergency lighting, the illuminance of evacuation lighting shall not be less than 10 lx.",
		"provision_subject":     "养老院应急照明疏散照度要求",
		"provision_subject_en":  "Evacuation lighting illuminance requirement for nursing homes",
		"prov_desc":             "规定应急照明场所疏散照度的最低值",
		"prov_desc_en":          "Specifies the minimum illuminance level for evacuation lighting in emergency lighting areas",
		"prov_context":          "养老院建筑电气设计规范",
		"prov_context_en":       "Code for electrical design of nursing home buildings",
		"provision_keywords":    []any{"应急照明", "疏散照明", "照度值", "不低于"},
		"provision_keywords_en": []any{"emergency lighting", "evacuation lighting", "illuminance", "not less than"},
	}, true)

	if strings.Count(got, source) != 1 {
		t.Fatalf("source/provision count=%d, want 1 in %q", strings.Count(got, source), got)
	}
}
