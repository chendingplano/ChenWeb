package kbhandler

import (
	"encoding/json"
	"testing"
)

func TestDroppedMetricRecordMapsRowData(t *testing.T) {
	rowData := []byte(`{"candidate_id":"6_2","metric_name":"餐厨垃圾收运时间和频次","subject":"集中供餐单位餐厨垃圾",
		"threshold_or_target":"由收运单位与集中供餐单位约定","value_class":"requirement","value_range_type":"limit_absent",
		"source_line_spans":["101"],"keywords":["收运频次"],"confidence":0.6,"is_explicit_metric":false,"unit":""}`)
	decision := []byte(`{"choice":"activity_schedule","probabilities":{"activity_schedule":1}}`)

	r, err := droppedMetricRecord(7, 416, "416_drp_3", "decision_model", "activity_schedule", decision, rowData)
	if err != nil {
		t.Fatalf("droppedMetricRecord: %v", err)
	}
	if r.ID != -7 || *r.MetricID != "416_drp_3" || !r.Dropped || *r.DropStage != "decision_model" || *r.DropReason != "activity_schedule" {
		t.Fatalf("identity fields = %+v", r)
	}
	if *r.MetricName != "餐厨垃圾收运时间和频次" || *r.MetricSubject != "集中供餐单位餐厨垃圾" || *r.ValueRangeType != "limit_absent" {
		t.Fatalf("row fields = %+v", r)
	}
	if r.MetricUnit != nil {
		t.Fatalf("empty unit must be omitted, got %q", *r.MetricUnit)
	}
	if string(r.SourceLineSpans) != `["101"]` || *r.Confidence != 0.6 || *r.IsExplicitMetric {
		t.Fatalf("spans/confidence/explicit = %s/%v/%v", r.SourceLineSpans, *r.Confidence, *r.IsExplicitMetric)
	}
	var d map[string]any
	if err := json.Unmarshal(r.DropDecision, &d); err != nil || d["choice"] != "activity_schedule" {
		t.Fatalf("decision = %s", r.DropDecision)
	}

	// Non-decision drops carry no decision.
	r, err = droppedMetricRecord(8, 416, "416_drp_4", "statement_kind", "inspection_requirement", nil, []byte(`{"metric_name":"垃圾桶加盖"}`))
	if err != nil || r.DropDecision != nil {
		t.Fatalf("statement_kind drop: %+v, %v", r, err)
	}
}
