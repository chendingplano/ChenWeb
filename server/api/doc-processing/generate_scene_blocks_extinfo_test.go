package docprocessing

import (
	"reflect"
	"testing"
)

func TestBuildSceneBlockExtInfoCapturesEnglishVariants(t *testing.T) {
	block := map[string]any{
		"title_en":      "Reference application",
		"summary_en":    "How references are applied.",
		"evidence_lines": []any{
			"12:14",
			"18",
		},
		"keywords_en": []any{
			"reference",
			"application",
		},
		"states_en": []any{
			"active",
			"reviewed",
		},
	}

	got := buildSceneBlockExtInfo(block)

	want := map[string]any{
		"title_en":      "Reference application",
		"summary_en":    "How references are applied.",
		"evidence_lines": []string{
			"12:14",
			"18",
		},
		"keywords_en": []string{
			"reference",
			"application",
		},
		"states_en": []string{
			"active",
			"reviewed",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildSceneBlockExtInfo() mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestBuildSceneBlockExtInfoOmitsBlankAndInvalidValues(t *testing.T) {
	block := map[string]any{
		"title_en":      "   ",
		"summary_en":    "",
		"evidence_lines": []any{"9", " ", 12},
		"keywords_en":   []any{"keep", " ", 123},
		"states_en":     "not-an-array",
	}

	got := buildSceneBlockExtInfo(block)

	want := map[string]any{
		"evidence_lines": []string{"9"},
		"keywords_en":    []string{"keep"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildSceneBlockExtInfo() mismatch\n got: %#v\nwant: %#v", got, want)
	}
}
