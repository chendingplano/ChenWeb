package docprocessing

import (
	"context"
	"testing"
)

// fakeOpenValueJudge answers from a table keyed by metric_name and records what it was asked.
type fakeOpenValueJudge struct {
	answers map[string]map[string]any
	asked   []string
}

func (f *fakeOpenValueJudge) JudgeOpenValueRows(_ context.Context, _ int64, rows []map[string]any) []map[string]any {
	out := make([]map[string]any, len(rows))
	for i, r := range rows {
		name := asString(r["metric_name"])
		f.asked = append(f.asked, name)
		out[i] = f.answers[name]
	}
	return out
}

func openValueRow(name string) map[string]any {
	return map[string]any{"metric_name": name, "value_class": "requirement", "value_range_type": "limit_absent"}
}

func decisionAnswer(choice string, p float64) map[string]any {
	probs := map[string]float64{"activity_schedule": 0, "not_a_quantity": 0, "object_quantity": 0}
	probs[choice] = p
	return map[string]any{"choice": choice, "probabilities": probs, "policy_id": int64(3), "policy_version": 1,
		"named": 1.0, "provision_only": 0.0}
}

// inferredQuantityAnswer is object_quantity with the given named / provision_only answers.
func inferredQuantityAnswer(named, provision float64) map[string]any {
	a := decisionAnswer("object_quantity", 1.0)
	a["named"], a["provision_only"] = named, provision
	return a
}

func TestJudgeOpenValueRows(t *testing.T) {
	judge := &fakeOpenValueJudge{answers: map[string]map[string]any{
		"餐厨垃圾收运时间和频次": decisionAnswer("activity_schedule", 1.0),
		"边界":          decisionAnswer("activity_schedule", 0.9),
		"不确定":         decisionAnswer("activity_schedule", 0.89),
		"比能耗":         decisionAnswer("object_quantity", 1.0),
		"主体工艺":        decisionAnswer("not_a_quantity", 0.95),
		"工艺不确定":       decisionAnswer("not_a_quantity", 0.6),
		"失败":          {"error": "response has no logprobs"},
		"设备配备数量":      inferredQuantityAnswer(0.0, 0.38),
		"表头尺寸":        inferredQuantityAnswer(0.02, 0.01),
	}}
	p := NewMetricsProcessor(&fakeDocMetadataStore{}, &fakeMetricsStore{}, &fakeJSONExtractor{}, nil)
	p.OpenValueJudge = judge
	p.OpenValueDropMinP = 0.9
	numeric := map[string]any{"metric_name": "总砷", "value_class": "requirement", "value_range_type": "upper_bound", "metric_value": "15"}
	testParam := map[string]any{"metric_name": "浸提时间", "value_class": "requirement", "value_range_type": "limit_absent",
		"reasoning_tags": []any{"test_condition"}}
	rows := []map[string]any{numeric, testParam, openValueRow("餐厨垃圾收运时间和频次"), openValueRow("边界"), openValueRow("不确定"),
		openValueRow("比能耗"), openValueRow("主体工艺"), openValueRow("工艺不确定"), openValueRow("失败"), openValueRow("设备配备数量"), openValueRow("表头尺寸")}

	kept, dropped := p.judgeOpenValueRows(context.Background(), 416, rows)

	// Only requirement_value_open rows are judged; numeric and test-parameter rows never are.
	if len(judge.asked) != 9 {
		t.Fatalf("asked %v, want only the 9 open-value rows", judge.asked)
	}
	// Two activity schedules at p >= 0.9, and one quantity the clause never names in a provision clause.
	wantDropped := []struct{ name, reason, code string }{
		{"餐厨垃圾收运时间和频次", "activity_schedule", openValueReasonScheduleDropped},
		{"边界", "activity_schedule", openValueReasonScheduleDropped},
		{"主体工艺", "not_a_quantity", "not_a_quantity_confident"},
		{"设备配备数量", "no_named_quantity", openValueReasonNoNamedDropped},
	}
	if len(dropped) != len(wantDropped) {
		t.Fatalf("dropped = %v, want %v", dropped, wantDropped)
	}
	for i, d := range dropped {
		w := wantDropped[i]
		if d.Row["metric_name"] != w.name || d.Stage != metricDropStageDecision || d.Reason != w.reason ||
			d.Decision["outcome"] != "dropped" || d.Decision["reason"] != w.code || d.Decision["policy_id"] != int64(3) {
			t.Fatalf("dropped[%d] = %+v", i, d)
		}
	}
	if len(kept) != 7 {
		t.Fatalf("kept %d rows, want 7", len(kept))
	}
	// Every judged kept row says it was examined, that it was kept, and why.
	wantReason := map[string]string{
		"不确定":   openValueReasonScheduleBelow,
		"表头尺寸":  openValueReasonNoNamedVetoed,
		"比能耗":   openValueReasonObjectQuantity,
		"工艺不确定": "not_a_quantity_below_threshold",
		"失败":    openValueReasonError,
	}
	for _, m := range kept {
		name := asString(m["metric_name"])
		ext, _ := m["ext_info"].(map[string]any)
		decision, judged := ext["open_value_decision"].(map[string]any)
		if name == "总砷" || name == "浸提时间" {
			if judged {
				t.Fatalf("%s must not carry a decision", name)
			}
			continue
		}
		if !judged || decision["examined"] != true || decision["outcome"] != "kept" ||
			decision["reason"] != wantReason[name] || asString(decision["reason_text"]) == "" ||
			decision["threshold"] != 0.9 || decision["judged_at"] == nil {
			t.Fatalf("%s decision = %v", name, decision)
		}
		if name == "失败" && decision["error"] != "response has no logprobs" {
			t.Fatalf("failed row must keep its error, got %v", decision)
		}
		if name == "比能耗" && decision["choice_meaning"] != openValueOptions["object_quantity"] {
			t.Fatalf("choice_meaning = %v", decision["choice_meaning"])
		}
	}
}

func TestJudgeOpenValueRowsWithoutModelKeepsEverything(t *testing.T) {
	p := NewMetricsProcessor(&fakeDocMetadataStore{}, &fakeMetricsStore{}, &fakeJSONExtractor{}, nil)
	p.OpenValueJudge = nil
	p.OpenValueJudgeErr = nil
	rows := []map[string]any{openValueRow("餐厨垃圾收运时间和频次")}
	kept, dropped := p.judgeOpenValueRows(context.Background(), 416, rows)
	if len(kept) != 1 || len(dropped) != 0 {
		t.Fatalf("kept=%v dropped=%v", kept, dropped)
	}
	decision := kept[0]["ext_info"].(map[string]any)["open_value_decision"].(map[string]any)
	if decision["error"] != "decision model not configured" || decision["examined"] != false ||
		decision["reason"] != openValueReasonNotConfigured || decision["outcome"] != "kept" {
		t.Fatalf("decision = %v", decision)
	}
}

func TestFinalizeChunkBatch_ValuelessRowsNeverReachDecisionModel(t *testing.T) {
	extractor := &fakeJSONExtractor{outs: []map[string]any{{"metrics": []any{
		map[string]any{"metric_name": "餐厨垃圾收运时间和频次", "value_class": "requirement", "value_range_type": "limit_absent",
			"threshold_or_target": "由收运单位与集中供餐单位约定", "source_line_spans": []any{"2"}},
		map[string]any{"metric_name": "比能耗", "value_class": "requirement", "value_range_type": "limit_absent",
			"threshold_or_target": "由设备明确", "source_line_spans": []any{"2"}},
	}, "uncertain_metrics": []any{}}}}
	metricsStore := &fakeMetricsStore{}
	p := NewMetricsProcessor(&fakeDocMetadataStore{rec: DocMetadataInputRecord{ID: 416}}, metricsStore, extractor, nil)
	judge := &fakeOpenValueJudge{answers: map[string]map[string]any{
		"餐厨垃圾收运时间和频次": decisionAnswer("activity_schedule", 1.0),
		"比能耗":         decisionAnswer("object_quantity", 1.0),
	}}
	p.OpenValueJudge = judge
	p.OpenValueDropMinP = 0.9
	p.batchRecordID = 416
	p.batchForceClear = true
	p.ObjectStore = &fakeArtifactObjectsStore{}
	p.batchChunks = metricsBlocksToChunks([]Block{makeMetricsBlock(0, 2)})
	p.batchMentions = []metricCandidateMention{{MetricNameHint: "比能耗", SourceLineSpans: []string{"2"}, HasNormalEvidence: true}}
	if err := p.FinalizeChunkBatch(context.Background()); err != nil {
		t.Fatalf("FinalizeChunkBatch: %v", err)
	}
	// Both rows leave the value open, so the no_value check sets them aside before the
	// open-value decision: the decision model is not asked.
	if saved := metricsStore.lastSave.Metrics; len(saved) != 0 {
		t.Fatalf("saved = %v, want none", saved)
	}
	if len(judge.asked) != 0 {
		t.Fatalf("decision model asked %v, want nothing", judge.asked)
	}
	if len(metricsStore.dropped) != 2 {
		t.Fatalf("dropped = %+v, want 2", metricsStore.dropped)
	}
	for _, d := range metricsStore.dropped {
		if d.Stage != metricDropStageNoValue {
			t.Fatalf("dropped %s at stage %s, want no_value", d.Row["metric_name"], d.Stage)
		}
	}
}
