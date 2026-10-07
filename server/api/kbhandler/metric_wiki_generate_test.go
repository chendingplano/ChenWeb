package kbhandler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunMetricWikiProseReasoningPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, policy, want string
		unset, fallback    bool
	}{
		{name: "unset", unset: true, want: "enabled"},
		{name: "empty", want: "enabled"},
		{name: "no reasoning", policy: "no-reasoning", want: "disabled"},
		{name: "normalized", policy: " NO-REASONING ", want: "disabled"},
		{name: "model configured", policy: "reasoning", want: "enabled"},
		{name: "fallback", policy: "no-reasoning", want: "disabled", fallback: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MODEL_DEFAULT_REASONING_POLICY", tc.policy)
			if tc.unset {
				if err := os.Unsetenv("MODEL_DEFAULT_REASONING_POLICY"); err != nil {
					t.Fatal(err)
				}
			}
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Thinking struct {
						Type string `json:"type"`
					} `json:"thinking"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body.Thinking.Type != tc.want {
					t.Errorf("request thinking.type = %q, want %q", body.Thinking.Type, tc.want)
				}
				calls++
				content := `{"lead":"A metric wiki lead"}`
				if tc.fallback && calls == 1 {
					content = `{}` // Unusable prose triggers the fallback model.
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"choices": []any{map[string]any{"message": map[string]any{"content": content}}},
				})
			}))
			defer server.Close()
			modelsPath := filepath.Join(t.TempDir(), ".models.toml")
			config := fmt.Sprintf("[wiki-primary]\nmodel_name = %q\napi_key = 'test-key'\nbase_url = %q\ntimeout_sec = 10\nthinking_type = 'enabled'\n[wiki-fallback]\nmodel_name = %q\napi_key = 'test-key'\nbase_url = %q\ntimeout_sec = 10\nthinking_type = 'enabled'\n", "wiki-primary", server.URL, "wiki-fallback", server.URL)
			if err := os.WriteFile(modelsPath, []byte(config), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("MODEL_DEF_FILE", modelsPath)
			t.Setenv("WIKIPAGE_CREATION_MODEL_NAME", "wiki-primary")
			t.Setenv("WIKIPAGE_CREATION_FALLBACK", "wiki-fallback")
			prose, model, err := runMetricWikiProse(context.Background(), nil, "Test prompt", "Test facts")
			if err != nil {
				t.Fatal(err)
			}
			wantModel, wantCalls := "wiki-primary", 1
			if tc.fallback {
				wantModel, wantCalls = "wiki-fallback", 2
			}
			if prose.Lead == "" || model != wantModel || calls != wantCalls {
				t.Fatalf("lead=%q model=%q calls=%d; want model=%q calls=%d", prose.Lead, model, calls, wantModel, wantCalls)
			}
		})
	}
}

func wikiStrPtr(s string) *string { return &s }

func sampleMetricContext() metricWikiContext {
	return metricWikiContext{
		MetricID: "5_3",
		RecordID: 5,
		Metric: metricRecord{
			MetricID:            wikiStrPtr("5_3"),
			InputRecordID:       5,
			MetricName:          wikiStrPtr("Switching Frequency"),
			MetricSubject:       wikiStrPtr("Inverter"),
			MetricUnit:          wikiStrPtr("kHz"),
			MetricValue:         wikiStrPtr("10"),
			ValueRangeType:      wikiStrPtr("range"),
			ThresholdOrTarget:   wikiStrPtr("<= 20"),
			MeasurementFreq:     wikiStrPtr("per cycle"),
			FormulaOrDefinition: wikiStrPtr("f_sw = 1 / T_sw"),
			MetricContext:       wikiStrPtr("Defined in section 4.2"),
		},
		Document: metricWikiDocMeta{RecordID: 5, Title: "Std 20039", FileName: "std.txt", Type: "txt"},
	}
}

func TestAssembleMetricWikiPageGroundedFields(t *testing.T) {
	mctx := sampleMetricContext()
	prose := metricWikiProse{
		Lead:           "Switching frequency is how often the inverter switches.",
		Background:     "general background",
		HowUsed:        "used in PWM",
		RelatedMetrics: []string{"Dead Time"},
	}
	page := assembleMetricWikiPage(mctx, prose, "test-model", "en")

	// Grounded structured fields come from the metric, not the LLM.
	if page.Infobox.Value != "10" || page.Infobox.Unit != "kHz" {
		t.Errorf("infobox value/unit = %q/%q, want 10/kHz", page.Infobox.Value, page.Infobox.Unit)
	}
	if page.Infobox.ThresholdOrTarget != "<= 20" || page.Infobox.MeasurementFreq != "per cycle" {
		t.Errorf("infobox threshold/freq = %q/%q", page.Infobox.ThresholdOrTarget, page.Infobox.MeasurementFreq)
	}
	if page.Title != "Switching Frequency" {
		t.Errorf("title = %q", page.Title)
	}
	if page.InThisCorpus.SourceDocument.Title != "Std 20039" {
		t.Errorf("source doc title = %q", page.InThisCorpus.SourceDocument.Title)
	}
	// Definition falls back to the metric's formula when prose omits it.
	if page.Definition != "f_sw = 1 / T_sw" {
		t.Errorf("definition = %q, want formula fallback", page.Definition)
	}
	// Prose carries through.
	if page.Lead != prose.Lead || page.Background != "general background" {
		t.Errorf("prose not carried through: lead=%q background=%q", page.Lead, page.Background)
	}
	// Generated metadata.
	if page.Generated.Model != "test-model" || page.Generated.Lang != "en" || page.Generated.SchemaVersion != metricWikiSchemaVersion {
		t.Errorf("generated meta = %+v", page.Generated)
	}
	if page.Generated.SourceHash == "" {
		t.Error("source_hash is empty")
	}
}

func TestMetricWikiSourceHashStable(t *testing.T) {
	a := metricWikiSourceHash(sampleMetricContext())
	b := metricWikiSourceHash(sampleMetricContext())
	if a == "" || a != b {
		t.Errorf("source hash not stable: %q vs %q", a, b)
	}
	// A change in the underlying metric changes the hash.
	other := sampleMetricContext()
	other.Metric.MetricValue = wikiStrPtr("20")
	if metricWikiSourceHash(other) == a {
		t.Error("source hash did not change when metric value changed")
	}
}

func TestParseMetricWikiProse(t *testing.T) {
	payload := map[string]any{
		"lead":            "a lead",
		"definition":      "a def",
		"how_used":        "usage text",
		"related_metrics": []any{"A", "", "B"},
	}
	prose := parseMetricWikiProse(payload)
	if prose.Lead != "a lead" || prose.Definition != "a def" || prose.HowUsed != "usage text" {
		t.Errorf("prose = %+v", prose)
	}
	if len(prose.RelatedMetrics) != 2 || prose.RelatedMetrics[0] != "A" || prose.RelatedMetrics[1] != "B" {
		t.Errorf("related = %v, want [A B] (empties dropped)", prose.RelatedMetrics)
	}
}

func TestMetricWikiPromptTextIncludesRequestedOutputLanguage(t *testing.T) {
	t.Setenv("WIKIPAGE_CREATION_PROMPT", "")

	zhPrompt := metricWikiPromptText("zh-cn")
	if !strings.Contains(zhPrompt, "Simplified Chinese (zh-CN)") {
		t.Fatalf("zh prompt missing Chinese instruction: %q", zhPrompt)
	}

	enPrompt := metricWikiPromptText("en")
	if !strings.Contains(enPrompt, "human-readable prose fields in English") {
		t.Fatalf("en prompt missing English instruction: %q", enPrompt)
	}
}
