package docprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	llmclients "github.com/chendingplano/shared/go/api/llm"
)

// simpleMetricsExtractor records every request and answers with respond.
type simpleMetricsExtractor struct {
	mu      sync.Mutex
	inputs  []llmclients.JSONExtractionInput
	respond func(n int, in llmclients.JSONExtractionInput) (map[string]any, error)
}

func (e *simpleMetricsExtractor) ExtractJSON(_ context.Context, in llmclients.JSONExtractionInput) (map[string]any, error) {
	e.mu.Lock()
	e.inputs = append(e.inputs, in)
	n := len(e.inputs)
	e.mu.Unlock()
	return e.respond(n, in)
}

func makeTestSimpleMetricsProcessor(ext LLMJSONExtractor, store MetricsStore, inputStore DocMetadataStore) *MetricsProcessor {
	p := makeTestMetricsProcessor(ext, 1)
	p.name, p.simple = metricsSimpleProcessorName, true
	p.Store, p.InputStore = store, inputStore
	p.setSimplePrompt("simple prompt", "prompt-extract-metric-simple-v1.md", "", nil)
	p.setSimpleModel("test-simple-model", "", structureModelConfig{ModelName: "test-simple-model"}, nil)
	return p
}

func simpleLine(mark string, no int, lineType, content string) MarkedLine {
	return MarkedLine{Mark: mark, Line: Line{LineNo: no, LineType: lineType, Content: content}}
}

// chunkInputFlags returns line number → flag of a request's canonical chunk input.
func chunkInputFlags(t *testing.T, in llmclients.JSONExtractionInput) map[int]string {
	t.Helper()
	// Without a document context the canonical input is the bare lines array.
	var lines []lineForLLM
	if err := json.Unmarshal([]byte(in.InputText), &lines); err != nil {
		var doc struct {
			Lines []lineForLLM `json:"lines"`
		}
		if err := json.Unmarshal([]byte(in.InputText), &doc); err != nil {
			t.Fatalf("input text is not the canonical chunk JSON: %v\n%s", err, in.InputText)
		}
		lines = doc.Lines
	}
	out := map[int]string{}
	for _, l := range lines {
		out[l.LineNumber] = l.Flag
	}
	return out
}

func TestMetricsSimpleProcessorName(t *testing.T) {
	p := makeTestSimpleMetricsProcessor(&simpleMetricsExtractor{}, &fakeMetricsStore{}, nil)
	if p.Name() != "extract_metrics_simple" {
		t.Fatalf("Name()=%q", p.Name())
	}
	if (&MetricsProcessor{}).Name() != "extract_metrics" {
		t.Fatal("extract_metrics must keep its name")
	}
}

func TestAppendMetricsStatusKeepsTheOtherProcessorsEntry(t *testing.T) {
	raw, err := appendMetricsStatus(`[{"operation":"extract_metrics","proc_status":"success"}]`,
		metricsStatusParams{Operation: "extract_metrics_simple", RecordID: 1, Start: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, `"operation":"extract_metrics"`) || !strings.Contains(raw, `"operation":"extract_metrics_simple"`) {
		t.Fatalf("status=%s, want both entries", raw)
	}
	raw, _ = appendMetricsStatus(`[{"operation":"extract-metrics","proc_status":"failed"}]`,
		metricsStatusParams{RecordID: 1, Start: time.Now()})
	if strings.Contains(raw, "extract-metrics") {
		t.Fatalf("status=%s, extract_metrics must still replace its legacy entry", raw)
	}
}

func TestExpandSimpleMetricRow(t *testing.T) {
	idx := newSimpleChunkLines([]MarkedLine{
		simpleLine("n", 11, "heading-2", "4.2 重金属限量"),
		simpleLine("n", 12, "paragraph", "总砷应不大于 15 mg/kg。"),
	})
	row := expandSimpleMetricRow(map[string]any{
		"n": "总砷", "ne": "Total arsenic", "s": "肥料", "se": "Fertilizer", "v": "15", "u": "mg/kg",
		"r": "upper_bound", "k": "requirement", "l": []any{"12"}, "cat": []any{"heavy_metal_limit"},
	}, idx)
	want := map[string]any{
		"metric_name": "总砷", "metric_name_en": "Total arsenic", "subject": "肥料", "subject_en": "Fertilizer",
		"metric_value": "15", "unit": "mg/kg", "unit_en": "mg/kg", "value_range_type": "upper_bound",
		"value_class": "requirement", "value_class_en": "requirement", "condition": "",
		"context": "总砷应不大于 15 mg/kg。", "threshold_or_target": "总砷应不大于 15 mg/kg。",
		"table_name_or_section": "4.2 重金属限量", "location_type": "text", "value_data_type": "number",
		"is_explicit_metric": true, "confidence": metricSimpleConfidence,
	}
	for k, v := range want {
		if row[k] != v {
			t.Errorf("%s=%#v, want %#v", k, row[k], v)
		}
	}
	if spans := row["source_line_spans"].([]string); len(spans) != 1 || spans[0] != "12" {
		t.Errorf("source_line_spans=%v", spans)
	}
	if cats := row["metric_categories"].([]string); len(cats) != 1 || cats[0] != "heavy_metal_limit" {
		t.Errorf("metric_categories=%v", cats)
	}
	if kw := row["keywords"].([]string); len(kw) != 2 || kw[0] != "总砷" || kw[1] != "肥料" {
		t.Errorf("keywords=%v", kw)
	}
}

func TestExpandSimpleMetricRowTableAndDefaults(t *testing.T) {
	table := "<table><tr><td>序号</td><td>气体名称</td><td>充装系数</td></tr><tr><td>1</td><td>丙烷</td><td>0.41</td></tr></table>"
	idx := newSimpleChunkLines([]MarkedLine{
		simpleLine("n", 199, "table-caption", "表 A.1 常用液化气体特性"),
		simpleLine("n", 200, "table-image", "images/a.jpg"),
		simpleLine("n", 201, "table", table),
	})
	row := expandSimpleMetricRow(map[string]any{
		"n": "充装系数", "s": "丙烷", "v": "0.41", "u": "次", "r": "upper_bound", "k": "requirement",
		"l": []any{"201"}, "t": []any{"201#r1"},
	}, idx)
	if row["location_type"] != "table" || row["table_name_or_section"] != "表 A.1 常用液化气体特性" {
		t.Errorf("location_type=%v section=%v", row["location_type"], row["table_name_or_section"])
	}
	if row["unit_en"] != "times" {
		t.Errorf("unit_en=%v, want times", row["unit_en"])
	}
	if text := asString(row["threshold_or_target"]); !strings.Contains(text, "201#r1") || !strings.Contains(text, "丙烷") {
		t.Errorf("threshold_or_target=%q, want the cited table row", text)
	}
	if cats := row["metric_categories"].([]string); len(cats) != 1 || cats[0] != "requirement" {
		t.Errorf("metric_categories=%v, want the value class when the model gives none", cats)
	}
	if metricSimpleUnitEn("次/日") != "times/d" || metricSimpleUnitEn("mm") != "mm" || metricSimpleUnitEn("公顷") != "公顷" {
		t.Error("unit translation")
	}
	for value, want := range map[string]string{"15": "number", "5~25": "number", "±5": "number", "65±5": "number", "a..50": "string", "70+年龄×2": "string"} {
		if got := metricSimpleValueDataType(value, nil); got != want {
			t.Errorf("value_data_type(%q)=%s, want %s", value, got, want)
		}
	}
}

func TestExtractSimpleChunkRowsAndSetAsideItems(t *testing.T) {
	ext := &simpleMetricsExtractor{respond: func(int, llmclients.JSONExtractionInput) (map[string]any, error) {
		return map[string]any{"lang": "zh", "m": []any{
			map[string]any{"n": "噪声", "s": "设备", "v": "55", "u": "dB(A)", "r": "upper_bound", "k": "requirement", "l": []any{"10"}},
			map[string]any{"n": "仅重叠", "s": "设备", "v": "3", "l": []any{"30"}},
			map[string]any{"n": "无行号", "s": "设备", "v": "1"},
			map[string]any{"n": "试验方法数", "s": "试验", "v": "2", "l": []any{"11"}, "g": []any{"procedure_count"}},
		}, "x": []any{
			map[string]any{"l": []any{"11"}, "n": "量热计压力", "r": "term_definition"},
			map[string]any{"l": []any{"30"}, "n": "重叠项", "r": "term_definition"},
		}}, nil
	}}
	p := makeTestSimpleMetricsProcessor(ext, &fakeMetricsStore{}, nil)
	chunk := Chunk{SeqNo: 4, Lines: []MarkedLine{
		simpleLine("n", 10, "paragraph", "噪声应不大于 55 dB(A)"),
		simpleLine("n", 11, "paragraph", "3.7 量热计压力：量热容器的二次流体侧压力"),
		simpleLine("o", 30, "paragraph", "overlap text"),
	}}
	res, err := p.extractSimpleChunk(context.Background(), 7, chunk, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if res.language != "zh" {
		t.Errorf("language=%q", res.language)
	}
	if len(res.rows) != 1 || res.rows[0]["metric_name"] != "噪声" || res.rows[0]["candidate_id"] != "4_1" {
		t.Fatalf("rows=%v, want the one normal-line row as 4_1", res.rows)
	}
	if len(res.tagged) != 1 || res.tagged[0]["candidate_id"] != "4_2" {
		t.Fatalf("tagged=%v, want the drop-tagged row as 4_2", res.tagged)
	}
	if len(res.dropped) != 2 {
		t.Fatalf("dropped=%d rows, want invalid_row and the definition", len(res.dropped))
	}
	invalid, def := res.dropped[0], res.dropped[1]
	if invalid.Stage != metricDropStageEnrich || invalid.Reason != "invalid_row" || invalid.Row["candidate_id"] != "4_3" {
		t.Errorf("invalid row: stage=%s reason=%s id=%v", invalid.Stage, invalid.Reason, invalid.Row["candidate_id"])
	}
	if def.Stage != metricDropStageEnrich || def.Reason != "term_definition" || def.Row["metric_name"] != "量热计压力" ||
		!strings.Contains(asString(def.Row["threshold_or_target"]), "量热容器") || def.Row["candidate_id"] != "4_4" {
		t.Errorf("definition: %+v", def)
	}
}

func TestSimpleRequestLayoutIsTaskFirstAndConstant(t *testing.T) {
	ext := &simpleMetricsExtractor{respond: func(int, llmclients.JSONExtractionInput) (map[string]any, error) {
		return map[string]any{"lang": "en", "m": []any{}}, nil
	}}
	p := makeTestSimpleMetricsProcessor(ext, &fakeMetricsStore{}, nil)
	chunks := metricsBlocksToChunks([]Block{makeMetricsBlock(0, 10), makeMetricsBlock(1, 20)})
	if _, err := p.extractMetricsFromChunksWithLLM(context.Background(), 5, chunks, "Doc | GB 1"); err != nil {
		t.Fatal(err)
	}
	if len(ext.inputs) != 2 {
		t.Fatalf("calls=%d, want one per chunk", len(ext.inputs))
	}
	for i, in := range ext.inputs {
		if in.DocumentFirst {
			t.Errorf("call %d is document-first; the instructions must be the system message", i)
		}
		if in.PromptText != ext.inputs[0].PromptText || in.PromptText != "simple prompt" {
			t.Errorf("call %d prompt differs: %q", i, in.PromptText)
		}
		if in.CallReason != "extract_metrics_simple" {
			t.Errorf("call %d call reason=%q", i, in.CallReason)
		}
	}
	if !strings.Contains(ext.inputs[0].InputText, "metric text 10") || !strings.Contains(ext.inputs[1].InputText, "metric text 20") {
		t.Error("each call must carry its own chunk")
	}
}

func TestSimpleFailedCallIsRetriedAsHalves(t *testing.T) {
	ext := &simpleMetricsExtractor{respond: func(n int, in llmclients.JSONExtractionInput) (map[string]any, error) {
		if n == 1 {
			return nil, errors.New("truncated JSON")
		}
		// Each half's response cites all four lines; only its own normal lines may survive.
		var rows []any
		for _, line := range []string{"1", "2", "3", "4"} {
			rows = append(rows, map[string]any{"n": "m" + line, "s": "s", "v": "1", "l": []any{line}})
		}
		return map[string]any{"lang": "zh", "m": rows}, nil
	}}
	p := makeTestSimpleMetricsProcessor(ext, &fakeMetricsStore{}, nil)
	chunk := Chunk{SeqNo: 2, Lines: []MarkedLine{
		simpleLine("n", 1, "paragraph", "a"), simpleLine("n", 2, "paragraph", "b"),
		simpleLine("n", 3, "paragraph", "c"), simpleLine("n", 4, "paragraph", "d"),
	}}
	res, err := p.extractSimpleChunk(context.Background(), 7, chunk, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(ext.inputs) != 3 {
		t.Fatalf("calls=%d, want the failed call and two halves", len(ext.inputs))
	}
	first, second := chunkInputFlags(t, ext.inputs[1]), chunkInputFlags(t, ext.inputs[2])
	if first[1] != "n" || first[2] != "n" || first[3] != "o" || first[4] != "o" {
		t.Errorf("first half flags=%v", first)
	}
	if second[1] != "o" || second[2] != "o" || second[3] != "n" || second[4] != "n" {
		t.Errorf("second half flags=%v", second)
	}
	if len(res.rows) != 4 {
		t.Errorf("rows=%d, want each half's own two lines", len(res.rows))
	}
	ids := map[any]bool{}
	for _, r := range res.rows {
		ids[r["candidate_id"]] = true
	}
	if len(ids) != 4 {
		t.Errorf("candidate ids=%v, want four distinct ids", ids)
	}
}

func TestSimpleFailedSingleLineChunkFails(t *testing.T) {
	ext := &simpleMetricsExtractor{respond: func(int, llmclients.JSONExtractionInput) (map[string]any, error) {
		return nil, errors.New("timeout")
	}}
	p := makeTestSimpleMetricsProcessor(ext, &fakeMetricsStore{}, nil)
	chunk := Chunk{SeqNo: 1, Lines: []MarkedLine{simpleLine("n", 1, "table", "<table></table>")}}
	if _, err := p.extractSimpleChunk(context.Background(), 7, chunk, "", 1); err == nil {
		t.Fatal("want an error: a one-line chunk cannot be split")
	}
	if len(ext.inputs) != 1 {
		t.Fatalf("calls=%d, want no retry", len(ext.inputs))
	}
}

func TestMetricWarmupGate(t *testing.T) {
	for _, tc := range []struct {
		name      string
		warm      bool
		wantSleep time.Duration
	}{
		{"cold cache waits until 60 s after the first call started", false, 35 * time.Second},
		{"warm cache releases at once", true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
			clock := start
			gate := newMetricWarmupGate(func() time.Time { return clock }, 60*time.Second)
			var slept time.Duration
			gate.sleep = func(_ context.Context, d time.Duration) { slept = d }
			var order []string
			var mu sync.Mutex
			leaderRunning := make(chan struct{})
			finish := make(chan struct{})
			go gate.do(context.Background(), func() bool {
				close(leaderRunning)
				<-finish
				clock = start.Add(25 * time.Second)
				mu.Lock()
				order = append(order, "leader")
				mu.Unlock()
				return tc.warm
			})
			<-leaderRunning
			done := make(chan struct{})
			go func() {
				gate.do(context.Background(), func() bool {
					mu.Lock()
					order = append(order, "follower")
					mu.Unlock()
					return false
				})
				close(done)
			}()
			time.Sleep(20 * time.Millisecond)
			mu.Lock()
			if len(order) != 0 {
				t.Fatalf("follower ran before the first call returned: %v", order)
			}
			mu.Unlock()
			close(finish)
			<-done
			if len(order) != 2 || order[0] != "leader" || order[1] != "follower" {
				t.Fatalf("order=%v", order)
			}
			if tc.warm && slept != 0 || !tc.warm && slept != tc.wantSleep {
				t.Fatalf("slept=%v, want %v", slept, tc.wantSleep)
			}
		})
	}
}

func TestMetricWarmupGateStopsWaitingFollowers(t *testing.T) {
	gate := newMetricWarmupGate(time.Now, time.Minute)
	block := make(chan struct{})
	go gate.do(context.Background(), func() bool { <-block; return true })
	time.Sleep(10 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	if gate.do(ctx, func() bool { called = true; return true }) || called {
		t.Fatal("a stopped follower must not call")
	}
	close(block)
}

func TestMetricsSimpleChunkBatchSavesRowsAndSetAsides(t *testing.T) {
	const recordID = int64(9911)
	ext := &simpleMetricsExtractor{respond: func(_ int, in llmclients.JSONExtractionInput) (map[string]any, error) {
		if strings.Contains(in.InputText, "metric text 10") {
			return map[string]any{"lang": "zh", "m": []any{
				map[string]any{"n": "吞吐量", "ne": "Throughput", "s": "系统", "se": "System", "v": "1000", "u": "rps",
					"r": "lower_bound", "k": "requirement", "l": []any{"10"}, "cat": []any{"throughput"}},
				map[string]any{"n": "额定功率", "s": "设备", "r": "exact", "k": "requirement", "l": []any{"10"}},
			}}, nil
		}
		return map[string]any{"lang": "zh", "m": []any{}, "x": []any{
			map[string]any{"l": []any{"20"}, "n": "术语", "r": "term_definition"},
		}}, nil
	}}
	store := &fakeMetricsStore{}
	inputStore := &fakeDocMetadataStore{rec: DocMetadataInputRecord{ID: recordID, StatusRaw: "[]"}}
	p := makeTestSimpleMetricsProcessor(ext, store, inputStore)
	p.ChunkDir = t.TempDir()
	chunks := metricsBlocksToChunks([]Block{makeMetricsBlock(0, 10), makeMetricsBlock(1, 20)})
	ctx := withDocProcessorFlags(context.Background(), true, true)
	if err := p.InitChunkBatch(ctx, recordID, chunks, ""); err != nil {
		t.Fatal(err)
	}
	for i := range chunks {
		if err := p.ProcessChunk(ctx, i); err != nil {
			t.Fatalf("ProcessChunk(%d): %v", i, err)
		}
	}
	if err := p.FinalizeChunkBatch(ctx); err != nil {
		t.Fatal(err)
	}
	if len(ext.inputs) != 2 {
		t.Errorf("calls=%d, want one per chunk", len(ext.inputs))
	}
	if store.saveCalled != 1 || len(store.lastSave.Metrics) != 1 {
		t.Fatalf("saveCalled=%d metrics=%d, want one saved row", store.saveCalled, len(store.lastSave.Metrics))
	}
	saved := store.lastSave.Metrics[0]
	if saved["metric_name"] != "吞吐量" || saved["metric_id"] != "9911_mtc_1" || saved["candidate_id"] != "0_1" {
		t.Errorf("saved row=%v", saved)
	}
	if store.lastSave.PromptName != "prompt-extract-metric-simple-v1.md" || store.lastSave.ModelName != "test-simple-model" {
		t.Errorf("prompt=%q model=%q", store.lastSave.PromptName, store.lastSave.ModelName)
	}
	reasons := map[string]string{}
	for _, d := range store.dropped {
		reasons[asString(d.Row["metric_name"])] = d.Stage + "/" + d.Reason
	}
	if reasons["额定功率"] != "no_value/no_value" || reasons["术语"] != "enrich/term_definition" {
		t.Errorf("set-aside rows=%v", reasons)
	}
}

func TestMetricProcessorConflict(t *testing.T) {
	metrics := &MetricsProcessor{}
	simple := makeTestSimpleMetricsProcessor(&simpleMetricsExtractor{}, &fakeMetricsStore{}, nil)
	if err := metricProcessorConflict([]Processor{metrics, simple}); err == nil ||
		!strings.Contains(err.Error(), "extract_metrics and extract_metrics_simple") {
		t.Fatalf("err=%v, want a refusal naming both processors", err)
	}
	for _, alone := range [][]Processor{{metrics}, {simple}, {nil, simple}} {
		if err := metricProcessorConflict(alone); err != nil {
			t.Fatalf("one metric processor refused: %v", err)
		}
	}
}

func TestBothMetricProcessorsCanBeConfigured(t *testing.T) {
	if _, ok := CanonicalOptionalProductionProcessor("extract_metrics_simple"); !ok {
		t.Fatal("extract_metrics_simple must be a selectable processor")
	}
	plan, err := BuildProductionProcessorPlan([]string{"extract_metrics", "extract_metrics_simple"})
	if err != nil {
		t.Fatalf("a runtime with both configured must build: %v", err)
	}
	if !strings.Contains(strings.Join(plan.ExecutionOrder(), ","), "extract_metrics_simple") {
		t.Fatalf("execution order=%v", plan.ExecutionOrder())
	}
}
