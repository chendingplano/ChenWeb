package docprocessing

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

// This opt-in check exercises both real LLM passes and normalization, without writing metrics
// to the database. Run with mise exec -- env METRIC_QUALITY_LIVE=1 go test
// ./server/api/doc-processing -run '^TestMetricExtractionQualityLive$' -count=1 -v.
// METRIC_QUALITY_OUTPUT optionally saves the returned rows for manual review.
func TestMetricExtractionQualityLive(t *testing.T) {
	if os.Getenv("METRIC_QUALITY_LIVE") != "1" {
		t.Skip("set METRIC_QUALITY_LIVE=1 to call the configured extraction model")
	}
	type wantMetric struct {
		line                                    int
		value, unit, rangeType, kind, condition string
	}
	cases := []struct {
		name  string
		lines []Line
		want  []wantMetric
	}{
		{
			name: "timing percentage positions and intermediate point counts",
			lines: []Line{
				{LineNo: 286, Content: "6.5.20.1 采用误差不大于表1中计时类项目规定误差1/10的标准时间间隔发生装置，分别在量程的20%、80%及其中间区段的任意2个～3个数值上的计时起点和终点，发出测试器材计时开始和结束的模拟触发信号，每一个规定间隔时间的测定次数不应少于3次。"},
				{LineNo: 287, Content: "6.5.20.2 采用误差不大于表1中计时类项目规定误差1/10的标准时间计量装置，分别在量程的20%、80%及其中间区段的任意2个～3个数值上的计时起点和终点，发出测试器材计时开始和结束的模拟触发信号，该触发信号同时接入标准时间计量装置，每一个规定间隔时间的测定次数不应少于3次。"},
			},
			want: []wantMetric{
				{286, "20", "%", "exact", statementKindTestParameter, ""},
				{286, "80", "%", "exact", statementKindTestParameter, ""},
				{286, "2~3", "个", "range", statementKindTestParameter, ""},
				{287, "20", "%", "exact", statementKindTestParameter, ""},
				{287, "80", "%", "exact", statementKindTestParameter, ""},
				{287, "2~3", "个", "range", statementKindTestParameter, ""},
			},
		},
		{
			name: "embedded apparatus settings in separate methods",
			lines: []Line{
				{LineNo: 263, Content: "6.5.13.1 将握力测试仪的手柄固定，向上施加拉力，加载装置宽度为80 mm，采用准确度为0.1%的标准测力计，分别测量最大、最小和中间3个力值，每个力值的测量次数不应少于3次。"},
				{LineNo: 264, Content: "6.5.13.2 将握力测试仪的手柄固定，向上施加拉力，加载装置宽度为80 mm，采用标准砝码，在无冲击的条件下，分别测量100 N~750 N范围内的两端数值及中间区段的任意2个~3个数值，每个数值测量次数不应少于3次。"},
			},
			want: []wantMetric{
				{263, "80", "mm", "exact", statementKindTestParameter, "标准测力计"},
				{263, "0.1", "%", "exact", statementKindTestParameter, "标准测力计"},
				{264, "80", "mm", "exact", statementKindTestParameter, "标准砝码"},
			},
		},
		{
			name:  "stopwatch accuracy and exact test counts",
			lines: []Line{{LineNo: 271, Content: "6.5.15.1 采用误差不大于0.01 s的电子秒表，对仰卧起坐测试仪的计时装置进行1 min的计时测试，并重复测试3次。"}},
			want: []wantMetric{
				{271, "0.01", "s", "upper_bound", statementKindTestParameter, "电子秒表"},
				{271, "1", "min", "exact", statementKindTestParameter, "电子秒表"},
				{271, "3", "次", "exact", statementKindTestParameter, "电子秒表"},
			},
		},
		{
			name: "equal table values under different populations",
			lines: []Line{
				{LineNo: 113, LineType: "table", Content: "<table><tr><td>器材</td><td>适用人群</td><td>基本参数</td><td>允许误差</td></tr><tr><td>台阶试验测试仪</td><td>成年人</td><td>节拍120次/min</td><td>±1次/min</td></tr><tr><td>台阶试验测试仪</td><td>儿童青少年(学生)</td><td>节拍120次/min</td><td>±1次/min</td></tr></table>"},
				{LineNo: 119, LineType: "table", Content: "<table><tr><td>器材</td><td>基本参数</td></tr><tr><td>跳绳测试仪</td><td>计数范围0~400次</td></tr></table>"},
			},
			want: []wantMetric{
				{113, "±1", "次/min", "upper_bound", statementKindRequirementWithCriterion, "成年人"},
				{113, "±1", "次/min", "upper_bound", statementKindRequirementWithCriterion, "儿童青少年"},
				{119, "0~400", "次", "range", statementKindRequirementWithCriterion, ""},
			},
		},
		{
			name: "acceptance criterion versus test setup",
			lines: []Line{
				{LineNo: 139, Content: "5.4.3.1 直立放置的测试器材在任意方向相对于其正常垂直位置倾斜10°时，测试器材不应有翻倒现象。"},
				{LineNo: 189, Content: "6.4.2.1 将测试器材放在与水平面成10°的倾斜平面上进行试验。"},
			},
			want: []wantMetric{
				{139, "10", "°", "lower_bound", statementKindRequirementWithCriterion, ""},
				{189, "10", "°", "exact", statementKindTestParameter, ""},
			},
		},
		{
			name: "alternative simulation methods name their apparatus",
			lines: []Line{
				{LineNo: 268, Content: "6.5.14.1 采用可以触动俯卧撑测试仪的运动起点和运动终点予以信号反应且具有调速功能的运动模拟装置，运动频率在1次/min~60次/min范围内，每组往复模拟运动试验不应少于10个循环，重复测试3次。"},
				{LineNo: 269, Content: "6.5.14.2 用模拟物在俯卧撑测试仪的运动起点和运动终点之间往复模拟运动，触发起点和终点信号，运动频率在1次/min~60次/min范围内，每组往复模拟运动试验不应少于10个循环，重复测试3次。"},
			},
			want: []wantMetric{
				{268, "1~60", "次/min", "range", statementKindTestParameter, "运动模拟装置"},
				{269, "1~60", "次/min", "range", statementKindTestParameter, "模拟物"},
			},
		},
		{
			name: "centred tolerances and adjustment trigger",
			lines: []Line{
				{LineNo: 360, Content: "沙包的边长或直径为65 mm±5 mm，质量为250 g±15 g。"},
				{LineNo: 366, Content: "压力校正方法：指针应在15 mm~25 mm范围内；如果指针超出25 mm以上，则通过旋钮增加压力，直至指针调至15 mm~25 mm范围内。"},
			},
			want: []wantMetric{
				{360, "65±5", "mm", "range", statementKindRequirementWithCriterion, ""},
				{360, "250±15", "g", "range", statementKindRequirementWithCriterion, ""},
				{366, "15~25", "mm", "range", statementKindRequirementWithCriterion, ""},
			},
		},
	}

	client, _, err := BuildReviewerLLMClient(os.Getenv("ENRICH_METRICS_MODEL_NAME"))
	if err != nil {
		t.Fatal(err)
	}
	p := NewMetricsProcessor(nil, nil, client, nil)
	p.ProcLogger = DocProcLogger{} // isolate this quality check from production persistence
	p.MaxTasks = 1
	for _, prompt := range []struct {
		ref string
		dst *string
	}{
		{p.MentionPromptRef, &p.MentionPromptText}, {p.RelationPromptRef, &p.RelationPromptText},
	} {
		b, err := os.ReadFile(filepath.Join("..", "..", "..", "prompts", prompt.ref))
		if err != nil {
			t.Fatal(err)
		}
		*prompt.dst = string(b)
	}
	previousStdio := loggerutil.SetStdioOutputEnabled(false)
	defer loggerutil.SetStdioOutputEnabled(previousStdio)
	results := map[string][]map[string]any{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(withLLMUserID(context.Background(), "metric-quality-test"), 5*time.Minute)
			defer cancel()
			chunk := Chunk{SeqNo: 1}
			for _, line := range tc.lines {
				chunk.Lines = append(chunk.Lines, MarkedLine{Mark: "n", Line: line})
			}
			r, err := p.extractMetricsFromChunksWithLLM(ctx, 0, []Chunk{chunk}, "")
			if err != nil {
				t.Fatal(err)
			}
			results[tc.name] = r.Metrics
			for _, want := range tc.want {
				found := 0
				for _, m := range r.Metrics {
					if !metricLineSpansOverlap(m, map[string]any{"source_line_spans": []string{strconv.Itoa(want.line)}}) {
						continue
					}
					value := compactMetricValue(asString(m["metric_value"]))
					if want.rangeType == "upper_bound" && !strings.HasPrefix(want.value, "±") {
						value = strings.TrimPrefix(value, "±") // equivalent error-magnitude notation
					}
					if value == want.value && metricRowUnit(m) == want.unit &&
						asString(m["value_range_type"]) == want.rangeType && metricStatementKind(m) == want.kind &&
						strings.Contains(asString(m["condition"]), want.condition) {
						found++
					}
				}
				if found != 1 {
					t.Errorf("expected one assertion, found %d: %+v", found, want)
					b, _ := json.Marshal(r.Metrics)
					t.Logf("returned metrics: %s", b)
				}
			}
			for _, m := range r.Metrics {
				if strings.HasPrefix(tc.name, "timing percentage") {
					if metricValueHoldsSeveralValues(asString(m["metric_value"])) {
						t.Errorf("combined timing values survived: %v", m)
					}
					if compactMetricValue(asString(m["metric_value"])) == "2~3" &&
						(!strings.Contains(asString(m["metric_name"])+asString(m["condition"]), "中间") ||
							toOptionalFloat(m["value_min"]) != float64(2) || toOptionalFloat(m["value_max"]) != float64(3)) {
						t.Errorf("point count lost intermediate-region identity or endpoints: %v", m)
					}
				}
				if strings.HasPrefix(tc.name, "alternative simulation") &&
					(strings.ContainsAny(asString(m["condition"]), "（(") || strings.Contains(asString(m["condition"]), "调速")) {
					t.Errorf("apparatus qualifier kept its capabilities instead of its name: %v", m)
				}
				if metricLineSpansOverlap(m, map[string]any{"source_line_spans": []string{"366"}}) && compactMetricValue(asString(m["metric_value"])) == "25" {
					t.Error("adjustment trigger incorrectly emitted as a separate metric")
				}
			}
			t.Logf("%d candidates, %d metrics", r.Candidates, len(r.Metrics))
		})
	}
	if path := os.Getenv("METRIC_QUALITY_OUTPUT"); path != "" {
		b, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
