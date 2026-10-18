package docprocessing

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitLegacyConditionMovesKnownLabels(t *testing.T) {
	cases := []struct {
		cond, condEn string
		wantCond     string
		wantCondEn   string
		check        func(t *testing.T, f *metricFrame)
	}{
		{
			cond: "适用人群：幼儿；试验装置：运动模拟装置", condEn: "Population: preschool children; Test apparatus: motion simulator",
			wantCond: "适用人群：幼儿", wantCondEn: "Population: preschool children",
			check: func(t *testing.T, f *metricFrame) {
				if len(f.Uses) != 1 || f.Uses[0].Role != "apparatus" || f.Uses[0].Name != "运动模拟装置" || f.Uses[0].NameEn != "motion simulator" {
					t.Fatalf("uses = %+v", f.Uses)
				}
			},
		},
		{
			cond: "适用人群：成人；给药途径：静脉推注", wantCond: "适用人群：成人",
			check: func(t *testing.T, f *metricFrame) {
				if len(f.Method) != 1 || f.Method[0].Label != "给药途径" || f.Method[0].Value != "静脉推注" {
					t.Fatalf("method = %+v", f.Method)
				}
			},
		},
		{
			cond: "触发：血糖＜2.8mmol/L；治疗阶段：意识转清后", wantCond: "触发：血糖＜2.8mmol/L",
			check: func(t *testing.T, f *metricFrame) {
				if f.Procedure == nil || f.Procedure.Stage != "意识转清后" {
					t.Fatalf("procedure = %+v", f.Procedure)
				}
			},
		},
		{
			cond: "试验装置：5 等量块", wantCond: "",
			check: func(t *testing.T, f *metricFrame) {
				r := f.Uses[0]
				if r.Name != "量块" || len(r.Spec) != 1 || r.Spec[0].Value != "5 等" {
					t.Fatalf("uses = %+v", f.Uses)
				}
			},
		},
		{
			cond: "溶媒：5％葡萄糖注射液；药物浓度：1:1000", wantCond: "",
			check: func(t *testing.T, f *metricFrame) {
				r := f.Uses[0]
				if r.Role != "medium" || r.Name != "葡萄糖注射液" || len(r.Spec) != 2 || r.Spec[0].Value != "5" || r.Spec[0].Unit != "%" || r.Spec[1].Value != "1:1000" {
					t.Fatalf("uses = %+v", f.Uses)
				}
			},
		},
		{
			cond: "试验介质：空气；试验方法：人工计数", wantCond: "",
			check: func(t *testing.T, f *metricFrame) {
				if len(f.Uses) != 1 || f.Uses[0].Role != "medium" || len(f.Method) != 1 {
					t.Fatalf("frame = %+v", f)
				}
			},
		},
	}
	for _, c := range cases {
		m := map[string]any{"condition": c.cond, "condition_en": c.condEn}
		splitLegacyCondition(m)
		if got := asString(m["condition"]); got != c.wantCond {
			t.Errorf("%q: condition = %q, want %q", c.cond, got, c.wantCond)
		}
		if c.condEn != "" && asString(m["condition_en"]) != c.wantCondEn {
			t.Errorf("%q: condition_en = %q, want %q", c.cond, m["condition_en"], c.wantCondEn)
		}
		f := metricRowFrame(m)
		if f == nil {
			t.Fatalf("%q: no frame", c.cond)
		}
		c.check(t, f)
	}
}

func TestSplitLegacyConditionKeepsConditionsAndUnknownLabels(t *testing.T) {
	for _, cond := range []string{"取样方式：随机", "器材类型：游标式", "适用人群：儿童；用药情形：无甘露醇情况下", "称量：≤50 kg"} {
		m := map[string]any{"condition": cond}
		res := splitLegacyCondition(m)
		if asString(m["condition"]) != cond || m["frame"] != nil || len(res.Moved) != 0 {
			t.Errorf("%q changed: condition=%q frame=%v", cond, m["condition"], m["frame"])
		}
	}
}

func TestSplitLegacyConditionSpecWithoutAgentStays(t *testing.T) {
	m := map[string]any{"condition": "适用人群：过敏性休克患者；给药途径：肌肉注射；药物浓度：1:1000"}
	res := splitLegacyCondition(m)
	if asString(m["condition"]) != "适用人群：过敏性休克患者；药物浓度：1:1000" || res.UnresolvedSpecs != 1 {
		t.Fatalf("condition=%q unresolved=%d", m["condition"], res.UnresolvedSpecs)
	}
}

func TestSplitLegacyConditionIdempotentAndMisalignedEnglish(t *testing.T) {
	m := map[string]any{"condition": "适用人群：成人；给药途径：静脉推注", "condition_en": "Adults, IV push"}
	splitLegacyCondition(m)
	before, _ := json.Marshal(m)
	if asString(m["condition_en"]) != "Adults, IV push" {
		t.Fatalf("misaligned condition_en changed: %q", m["condition_en"])
	}
	if res := splitLegacyCondition(m); len(res.Moved) != 0 {
		t.Fatalf("second split moved %v", res.Moved)
	}
	after, _ := json.Marshal(m)
	if string(before) != string(after) {
		t.Fatalf("not idempotent:\n%s\n%s", before, after)
	}
}

func TestMetricFrameFromValueToleratesModelOutput(t *testing.T) {
	raw := map[string]any{
		"uses": []any{
			map[string]any{"role": "Agent", "name": "葡萄糖注射液", "spec": []any{map[string]any{"property": "浓度", "value": 50.0, "unit": "%"}}},
			map[string]any{"role": "tool", "name": "注射器"},
			map[string]any{"role": "agent", "name": ""},
		},
		"method":    []any{map[string]any{"label": "给药途径", "value": "静脉推注"}, map[string]any{"label": "x", "value": ""}},
		"procedure": map[string]any{"step": "A.2.4"},
	}
	f, invalid := metricFrameFromValue(raw)
	if f == nil || invalid != 1 {
		t.Fatalf("frame=%v invalid=%d", f, invalid)
	}
	if len(f.Uses) != 2 || f.Uses[0].Role != "agent" || f.Uses[0].Spec[0].Value != "50" || f.Uses[1].Role != "" {
		t.Fatalf("uses = %+v", f.Uses)
	}
	if len(f.Method) != 1 || f.Procedure == nil || f.Procedure.Step != "A.2.4" {
		t.Fatalf("frame = %+v", f)
	}
	if empty, _ := metricFrameFromValue(map[string]any{"uses": []any{}, "procedure": map[string]any{}}); empty != nil {
		t.Fatalf("empty frame = %+v", empty)
	}
	b, _ := json.Marshal(f)
	if back, _ := metricFrameFromValue(b); back == nil || back.Uses[0].Name != "葡萄糖注射液" {
		t.Fatalf("round trip = %s", b)
	}
}

func TestMetricFrameIdentityKey(t *testing.T) {
	row := func(conc string, step string) map[string]any {
		return map[string]any{"frame": &metricFrame{
			Uses:      []metricFrameResource{{Role: "agent", Name: "葡萄糖注射液", Spec: []metricFrameSpec{{Property: "浓度", Value: conc, Unit: "%"}}}},
			Procedure: &metricFrameProcedure{Step: step},
		}}
	}
	if metricFramesEqual(row("25", "A.2.4"), row("10", "A.2.4")) {
		t.Fatal("different strengths compare equal")
	}
	if !metricFramesEqual(row("50", "A.2.4"), row("50", "A.3.1")) {
		t.Fatal("procedure step changed identity")
	}
	if !metricFramesEqual(map[string]any{}, map[string]any{"frame": &metricFrame{Procedure: &metricFrameProcedure{Step: "1"}}}) {
		t.Fatal("procedure-only frame differs from no frame")
	}
}

func TestInheritServedQuantityConditionsCopiesFrame(t *testing.T) {
	gauge := []metricFrameResource{{Role: "apparatus", Name: "握力测试仪(计)"}}
	served := map[string]any{"metric_name": "量程", "subject": "握力测试仪(计)", "source_line_spans": []any{"116"},
		"condition": "适用人群：幼儿", "frame": &metricFrame{Uses: gauge}}
	tol := map[string]any{"metric_name": "量程允许误差", "subject": "握力测试仪(计)", "source_line_spans": []any{"116"},
		"condition": "适用人群：幼儿"}
	if n := inheritServedQuantityConditions([]map[string]any{served, tol}); n != 1 {
		t.Fatalf("rows changed = %d, want 1", n)
	}
	if f := metricRowFrame(tol); f == nil || len(f.Uses) != 1 || f.Uses[0].Name != "握力测试仪(计)" {
		t.Fatalf("tolerance frame = %+v", f)
	}
	f := metricRowFrame(tol)
	f.Uses[0].Name = "changed"
	if metricRowFrame(served).Uses[0].Name != "握力测试仪(计)" {
		t.Fatal("served row's frame shares memory with the tolerance row")
	}
}

// 364 A.2.4 (line 185): the Pass 2 rows of run 235 with the frame the new prompts ask for. Only
// the dose rows stay; the 50 % and 10 % strength rows are set aside as resource_spec.
func TestDropResourceSpecRows364A24(t *testing.T) {
	glucose := func(conc string) *metricFrame {
		return &metricFrame{Uses: []metricFrameResource{{Role: "agent", Name: "葡萄糖注射液",
			Spec: []metricFrameSpec{{Property: "浓度", Value: conc, Unit: "%"}}}}}
	}
	row := func(id, name, value, unit string, f *metricFrame) map[string]any {
		m := map[string]any{"candidate_id": id, "metric_name": name, "metric_value": value, "unit": unit,
			"source_line_spans": []string{"185"}}
		if f != nil {
			m["frame"] = f
		}
		return m
	}
	rows := []map[string]any{
		row("c1", "血糖", "2.8", "mmol/L", nil),
		row("c2", "葡萄糖静推剂量", "40", "ml", glucose("50")),
		row("c3", "葡萄糖浓度", "50", "%", nil),
		row("c4", "葡萄糖剂量", "5~10", "ml/kg", glucose("10")),
		row("c5", "葡萄糖剂量", "2~4", "ml/kg", glucose("25")),
		row("c6", "10％葡萄糖静点浓度", "10", "％", nil),
	}
	kept, dropped := dropResourceSpecRows(rows)
	if len(kept) != 4 || len(dropped) != 2 {
		t.Fatalf("kept %d, dropped %d", len(kept), len(dropped))
	}
	for i, d := range dropped {
		host, _ := d.Row["resource_spec_host"].(map[string]any)
		if d.Reason != metricDropReasonResourceSpec || d.Stage != metricDropStageEnrich || host == nil {
			t.Fatalf("drop %d = %+v", i, d)
		}
	}
	if h := dropped[0].Row["resource_spec_host"].(map[string]any); h["candidate_id"] != "c2" {
		t.Fatalf("50 %% host = %v", h)
	}
	if h := dropped[1].Row["resource_spec_host"].(map[string]any); h["candidate_id"] != "c4" {
		t.Fatalf("10 %% host = %v", h)
	}
	if _, ok := rows[2]["resource_spec_host"]; ok {
		t.Fatal("the dropped row's source map was changed")
	}
}

func TestDropResourceSpecRowsKeepsRowsWithoutHost(t *testing.T) {
	// A product standard: the content is the object's own property, nothing uses it.
	content := map[string]any{"metric_name": "葡萄糖含量", "metric_value": "95.0~105.0", "unit": "%",
		"source_line_spans": []string{"12"}}
	strength := map[string]any{"metric_name": "葡萄糖浓度", "metric_value": "5", "unit": "%",
		"source_line_spans": []string{"12"}}
	other := map[string]any{"metric_name": "剂量", "metric_value": "250", "unit": "ml", "source_line_spans": []string{"40"},
		"frame": &metricFrame{Uses: []metricFrameResource{{Role: "agent", Name: "葡萄糖注射液",
			Spec: []metricFrameSpec{{Property: "浓度", Value: "5", Unit: "%"}}}}}}
	kept, dropped := dropResourceSpecRows([]map[string]any{content, strength, other})
	if len(kept) != 3 || len(dropped) != 0 {
		t.Fatalf("kept %d, dropped %d (host on another line must not count)", len(kept), len(dropped))
	}
	ratio := map[string]any{"metric_name": "肾上腺素浓度", "metric_value": "1:1000", "unit": "ratio", "source_line_spans": []string{"300"}}
	dose := map[string]any{"metric_name": "肾上腺素肌肉注射剂量", "metric_value": "0.3~0.5", "unit": "mg", "source_line_spans": []string{"300"},
		"frame": &metricFrame{Uses: []metricFrameResource{{Role: "agent", Name: "肾上腺素",
			Spec: []metricFrameSpec{{Property: "浓度", Value: "1:1000"}}}}}}
	if _, dropped := dropResourceSpecRows([]map[string]any{ratio, dose}); len(dropped) != 1 {
		t.Fatalf("dilution row not set aside")
	}
}

func TestSplitLegacyConditionDrugHint(t *testing.T) {
	m := map[string]any{"condition": "适用人群：成人；药物：50％葡萄糖；给药途径：静脉推注"}
	splitLegacyCondition(m)
	f := metricRowFrame(m)
	if asString(m["condition"]) != "适用人群：成人" || f == nil || f.Uses[0].Role != "agent" || f.Uses[0].Name != "葡萄糖" ||
		f.Uses[0].Spec[0].Value != "50" || len(f.Method) != 1 {
		t.Fatalf("condition=%q frame=%+v", m["condition"], f)
	}
}

// 364 A.2.4: the strength of the glucose a dose uses became its own row (mtc_30, mtc_33).
func TestMetricPromptsKeepResourceSpecsInTheFrame(t *testing.T) {
	for file, wants := range map[string][]string{
		"prompt-extract-metric-candidates-v29.md": {"## Resources And Their Specification", "药物：50％葡萄糖",
			"之后以10％葡萄糖静点", "葡萄糖含量 95.0~105.0 %", "试剂：0.1 mol/L 盐酸溶液"},
		"prompt-enrich-metrics-v28.md": {"## The Metric Frame", "### A Resource's Specification Is Not A Row",
			"`resource_spec`", "not `50％葡萄糖静推剂量`", "`器材类型：游标式`", "\"frame\": {"},
		"prompt-merge-resolve-metrics-v3.md": {"`frame.procedure` (the step or stage) is\nnot identity"},
	} {
		b, err := os.ReadFile(filepath.Join("..", "..", "..", "prompts", file))
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range wants {
			if !strings.Contains(string(b), want) {
				t.Errorf("%s missing %q", file, want)
			}
		}
	}
}

func TestDropResourceSpecRowsKeepsApparatusAccuracy(t *testing.T) {
	// The model wrote the dynamometer's accuracy as a spec; the accuracy row must survive.
	width := map[string]any{"metric_name": "加载装置宽度", "metric_value": "80", "unit": "mm", "source_line_spans": []string{"263"},
		"frame": &metricFrame{Uses: []metricFrameResource{{Role: "apparatus", Name: "标准测力计",
			Spec: []metricFrameSpec{{Property: "准确度", PropertyEn: "accuracy", Value: "0.1", Unit: "%"}}}}}}
	accuracy := map[string]any{"metric_name": "标准测力计准确度", "metric_value": "0.1", "unit": "%", "source_line_spans": []string{"263"}}
	if _, dropped := dropResourceSpecRows([]map[string]any{width, accuracy}); len(dropped) != 0 {
		t.Fatalf("accuracy row set aside: %+v", dropped)
	}
}
