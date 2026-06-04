package kbhandler

import "testing"

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
