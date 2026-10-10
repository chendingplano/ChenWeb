package docprocessing

import "testing"

func TestRefDice(t *testing.T) {
	if refDice("粪大肠菌群数", "粪大肠菌群数") != 1 {
		t.Fatal("identical CJK names must score 1")
	}
	if s := refDice("粪大肠菌群数", "粪大肠菌群值"); s < 0.7 || s >= 1 {
		t.Fatalf("near CJK names: %v", s)
	}
	if s := refDice("Rated power fan motor", "fan motor rated power"); s < 0.99 {
		t.Fatalf("word order must not matter for English: %v", s)
	}
	if s := refDice("Rated power of fan motor", "fan motor rated power"); s < 0.8 || s >= 0.9 {
		t.Fatalf("an extra word lowers the score: %v", s)
	}
	if refDice("", "x") != 0 {
		t.Fatal("empty must score 0")
	}
}

func metricRefTestTargets() []metricRefTarget {
	return []metricRefTarget{
		{Table: "metrics", ArtifactID: "753_mtc_1", RowID: 1, Name: "粪大肠菌群数", Subject: "堆肥", Section: "表1 卫生要求"},
		{Table: "metrics", ArtifactID: "753_mtc_2", RowID: 2, Name: "蛔虫卵死亡率", Subject: "堆肥", Section: "表1 卫生要求"},
		{Table: "metrics", ArtifactID: "753_mtc_3", RowID: 3, Name: "含水率", Subject: "堆肥产品", Section: "表2 理化指标"},
	}
}

func TestScoreMetricReference_ClearMatch(t *testing.T) {
	m := RefMention{TargetContent: "criterion", MetricName: "粪大肠菌群数", MetricSubject: "堆肥"}
	out := scoreMetricReference(m, 753, metricRefTestTargets())
	if out.State != metricRefResolved || len(out.Candidates) == 0 || !out.Candidates[0].Resolved || out.Candidates[0].ArtifactID != "753_mtc_1" {
		t.Fatalf("got %+v", out)
	}
	for _, c := range out.Candidates[1:] {
		if c.Resolved {
			t.Fatalf("only the top candidate may resolve: %+v", out.Candidates)
		}
	}
}

func TestScoreMetricReference_AmbiguousNeedsReview(t *testing.T) {
	ts := []metricRefTarget{
		{Table: "metrics", ArtifactID: "a", RowID: 1, Name: "含水率", Section: "表2"},
		{Table: "metrics", ArtifactID: "b", RowID: 2, Name: "含水率", Section: "表3"},
	}
	out := scoreMetricReference(RefMention{TargetContent: "criterion", MetricName: "含水率"}, 1, ts)
	if out.State != metricRefNeedsReview || len(out.Candidates) != 2 {
		t.Fatalf("two equal candidates must go to review: %+v", out)
	}
}

func TestScoreMetricReference_NothingResemblesIsTypeDoubtful(t *testing.T) {
	out := scoreMetricReference(RefMention{TargetContent: "criterion", MetricName: "风扇电机额定功率"}, 1, metricRefTestTargets())
	if out.State != metricRefNoMatch || len(out.Notes) != 1 || out.Notes[0] != "type_doubtful" {
		t.Fatalf("got %+v", out)
	}
	if out := scoreMetricReference(RefMention{TargetContent: "criterion", MetricName: "x"}, 1, nil); out.State != metricRefNoMatch {
		t.Fatalf("no targets: %+v", out)
	}
}

func TestScoreMetricReference_CriteriaSetResolvesLocatedTable(t *testing.T) {
	m := RefMention{TargetContent: "criteria_set", TargetLocator: "表 1"}
	out := scoreMetricReference(m, 753, metricRefTestTargets())
	if out.State != metricRefResolved || len(out.Candidates) != 2 {
		t.Fatalf("table 1 has two metrics: %+v", out)
	}
	for _, c := range out.Candidates {
		if !c.Resolved || c.ArtifactID == "753_mtc_3" {
			t.Fatalf("got %+v", out.Candidates)
		}
	}
	out = scoreMetricReference(RefMention{TargetContent: "criteria_set", TargetLocator: "Annex Z"}, 753, metricRefTestTargets())
	if out.State != metricRefNeedsReview {
		t.Fatalf("unlocated set must go to review: %+v", out)
	}
}

func TestScoreMetricReference_DroppedDefinitionIsLinkedNotCopied(t *testing.T) {
	ts := []metricRefTarget{{Table: "metrics_dropped", ArtifactID: "9_drp_1", RowID: 7, Name: "额定功率"}}
	out := scoreMetricReference(RefMention{TargetContent: "criterion", MetricName: "额定功率"}, 9, ts)
	if out.State != metricRefResolved || out.Candidates[0].Table != "metrics_dropped" {
		t.Fatalf("got %+v", out)
	}
}

func TestMetricResolverTakes(t *testing.T) {
	r := metricReferenceResolver{}
	for content, want := range map[string]bool{"criterion": true, "criteria_set": true, "method": false, "requirements": false, "data": false} {
		if r.Takes(RefMention{TargetContent: content}) != want {
			t.Errorf("Takes(%s) != %v", content, want)
		}
	}
	if referenceResolverFor("metric") == nil {
		t.Fatal("metric resolver must be registered at init")
	}
}

func TestAssembleRefMentions(t *testing.T) {
	payload := map[string]any{"references": []any{
		map[string]any{"cited_raw": "GB 7959", "target_type": "standard", "mechanism": "standard_number", "target_content": "criterion",
			"relationships": []any{"requires"}, "target_scope": "data_element", "metric_name": "粪大肠菌群数", "source_line_spans": []any{"120"}, "in_requirement": true},
		// the same occurrence seen again by an overlapping chunk
		map[string]any{"cited_raw": "GB  7959", "target_type": "standard", "mechanism": "standard_number", "target_content": "criterion",
			"relationships": []any{"requires"}, "target_scope": "data_element", "metric_name": "粪大肠菌群数", "source_line_spans": []any{"120"}},
		map[string]any{"cited_raw": "GB 7959—2012", "ref_form": "listed", "list_kind": "normative", "target_type": "standard",
			"mechanism": "standard_number", "target_content": "general", "source_line_spans": []any{40}},
		map[string]any{"cited_raw": "GB/T 755", "target_type": "standard", "mechanism": "standard_number", "target_content": "method",
			"metric_name": "额定功率", "source_line_spans": []any{"196"}},
		map[string]any{"cited_raw": "", "source_line_spans": []any{"1"}},
	}}
	ms := assembleRefMentions(753, parseRefMentions(payload))
	if len(ms) != 3 {
		t.Fatalf("want 3 mentions (duplicate and empty dropped), got %d: %+v", len(ms), ms)
	}
	if ms[0].RefForm != "listed" || ms[0].RefID != "753_ref_1" || ms[1].RefID != "753_ref_2" {
		t.Fatalf("reading order / ids: %+v", ms)
	}
	if ms[1].Normativity != "normative" || ms[1].ListEntryIdx != 0 {
		t.Fatalf("in-text GB 7959 must be normative via the list: %+v", ms[1])
	}
	if ms[2].MetricName != "" || ms[2].ListMismatch != "" {
		t.Fatalf("method reference keeps no quantity and is not in a requirement: %+v", ms[2])
	}
	if ms[1].MentionKey == "" || ms[1].Identifiers[0].Value != "GB 7959" {
		t.Fatalf("key/identifiers: %+v", ms[1])
	}
}

// Live run on record 753 (2026-10-11): overlapping chunks reported the same §2 line twice with
// different raw text, and manufacturer's documents were flagged as missing from §2.
func TestAssembleRefMentions_LiveRegressions(t *testing.T) {
	payload := map[string]any{"references": []any{
		map[string]any{"cited_raw": "GB/T755", "ref_form": "listed", "list_kind": "normative", "target_type": "standard", "mechanism": "standard_number", "target_content": "general", "source_line_spans": []any{"47"}},
		map[string]any{"cited_raw": "GB/T755 旋转电机 定额和性能", "ref_form": "listed", "list_kind": "normative", "target_type": "standard", "mechanism": "standard_number", "target_content": "general", "source_line_spans": []any{"47"}},
		map[string]any{"cited_raw": "GB/T755", "target_type": "standard", "mechanism": "standard_number", "target_content": "method", "in_requirement": true, "source_line_spans": []any{"196"}},
		map[string]any{"cited_raw": "制造商的说明书", "target_type": "document", "mechanism": "indirect_reference", "target_content": "requirements", "in_requirement": true, "source_line_spans": []any{"230"}},
	}}
	ms := assembleRefMentions(753, parseRefMentions(payload))
	if len(ms) != 3 {
		t.Fatalf("one list entry per line: got %d mentions", len(ms))
	}
	if ms[0].ListMismatch != "" || ms[1].ListEntryIdx != 0 || ms[1].Normativity != "normative" {
		t.Fatalf("the single §2 entry is used by the in-text citation: %+v / %+v", ms[0], ms[1])
	}
	if ms[2].ListMismatch != "" {
		t.Fatalf("a manufacturer's document cannot be a normative reference, so no not_listed: %+v", ms[2])
	}
}
