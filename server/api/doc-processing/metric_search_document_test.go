package docprocessing

import (
	"strings"
	"testing"
)

func TestBuildMetricSearchDocument_DedupesRepeatedSegments(t *testing.T) {
	desc := "气防站到防护范围内事故地点的行车距离不宜超过2.5km"
	descEn := "The driving distance from gas defense station to accident site within protection area should not exceed 2.5 km"

	got := buildMetricSearchDocument(map[string]any{
		"metric_name":           "行车距离",
		"metric_name_en":        "Driving distance",
		"subject":               "气防站到防护范围内事故地点",
		"subject_en":            "Driving distance from gas defense station to accident site within protection area",
		"keywords":              []any{"气防站", "行车距离", "事故地点"},
		"keywords_en":           []any{"gas defense station", "driving distance", "accident site"},
		"desc":                  desc,
		"desc_en":               descEn,
		"context":               desc,
		"context_en":            descEn,
		"unit":                  "km",
		"unit_en":               "km",
		"table_name_or_section": "",
	}, true)

	if strings.Count(got, desc) != 1 {
		t.Fatalf("desc count=%d, want 1 in %q", strings.Count(got, desc), got)
	}
	if strings.Count(got, descEn) != 1 {
		t.Fatalf("desc_en count=%d, want 1 in %q", strings.Count(got, descEn), got)
	}
	if strings.Count(joinUniqueSearchParts("km", "km"), "km") != 1 {
		t.Fatalf("unit dedupe failed")
	}
}

func TestBuildMetricSearchDocument_IncludesObjectNames(t *testing.T) {
	got := buildMetricSearchDocument(map[string]any{
		"metric_name": "maximum pressure",
		"subject":     "storage tank",
		"objects": []map[string]any{{
			"object_name":    "液化气储罐",
			"object_name_en": "LPG storage tank",
			"description":    "pressurized storage vessel",
			"aliases":        []string{"LPG tank"},
		}},
	}, true)

	for _, want := range []string{"液化气储罐", "LPG storage tank", "pressurized storage vessel", "LPG tank"} {
		if !strings.Contains(got, want) {
			t.Fatalf("search document %q does not contain object text %q", got, want)
		}
	}
}
