package kbhandler

import (
	"encoding/json"
	"reflect"
	"testing"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
)

func TestDroppedMetricRecordPreservesTableRowReferences(t *testing.T) {
	for _, refs := range []string{
		`[{"line":116,"rows":["r2","r3"],"row_hash":{"r2":"abc","r3":"def"}}]`,
		`["116#r2","116#r3"]`,
	} {
		t.Run(refs, func(t *testing.T) {
			r, err := droppedMetricRecord(7, 753, "753_drp_87", "no_value", "no_value", nil,
				[]byte(`{"metric_name":"水的质量流量","source_line_spans":["116"],"source_table_rows":`+refs+`}`))
			if err != nil {
				t.Fatal(err)
			}
			body, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			var response struct {
				SourceTableRows []docprocessing.TableRowRef `json:"source_table_rows"`
			}
			if err := json.Unmarshal(body, &response); err != nil {
				t.Fatal(err)
			}
			want := docprocessing.ParseTableRowRefs(refs)
			if !reflect.DeepEqual(response.SourceTableRows, want) {
				t.Fatalf("API table row references = %+v, want %+v", response.SourceTableRows, want)
			}
		})
	}
}

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

func TestBenchmarkDroppedMetricRecordPreservesCandidateFields(t *testing.T) {
	data := []byte(`{"candidate_id":"c7","decision":"excluded","rule_id":"X18","reason":"No value in the source",
		"metric_name":"Mass flow","metric_subject":"Water","metric_unit":"kg/s","metric_desc":"Definition only",
		"source_line_spans":["116"],"source_table_rows":[{"line":116,"rows":["r2"]}]}`)
	r, err := benchmarkDroppedMetricRecord(7, 753, "c7", "X18", data)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Dropped || *r.MetricID != "c7" || *r.MetricName != "Mass flow" || *r.MetricSubject != "Water" || *r.MetricUnit != "kg/s" || *r.MetricDesc != "Definition only" {
		t.Fatalf("lost candidate fields: %+v", r)
	}
	if *r.DropStage != "benchmark_rule" || *r.DropReason != "X18" || string(r.DropDecision) != string(data) || len(r.SourceTableRows) == 0 {
		t.Fatalf("lost candidate audit or table references: %+v", r)
	}
	legacy := []byte(`{"candidate_id":"c8","decision":"excluded","reason":"Heading","evidence_quote":"Scope","source_line_spans":["1"]}`)
	r, err = benchmarkDroppedMetricRecord(8, 753, "c8", "excluded", legacy)
	if err != nil || *r.MetricName != "Scope" || *r.MetricDesc != "Heading" || r.MetricValue != nil {
		t.Fatalf("legacy candidate fallback: %+v, %v", r, err)
	}
}
