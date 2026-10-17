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
		{
			name: "definitional numbers: population age bands and counting convention",
			lines: []Line{
				{LineNo: 97, Content: "4 分类"},
				{LineNo: 98, Content: "测试器材通常可按下述分类,其分类对应关系见附录 A。"},
				{LineNo: 100, Content: "b) 按适用人群分为: 幼儿、儿童青少年(学生)、成年人和老年人测试器材。"},
				{LineNo: 102, Content: "注 1: 3 周岁～6 周岁为幼儿,7 周岁～19 周岁为儿童青少年(学生),20 周岁～59 周岁为成年人,60 周岁～79 周岁为老年人。"},
				{LineNo: 288, Content: "6.5.21 跳绳测试仪"},
				{LineNo: 291, Content: "6.5.21.3 采用跳绳绕轴 $360^{\\circ}$ 计一圈的方法计数的模拟装置，在1次 $/ \\mathrm{min} \\sim 400$ 次/min范围内的两端数值及其中间区段的任意2个～3个数值上，进行 $1\\mathrm{min}$ 的模拟试验，每个试验次数不应少于3次。"},
			},
			want: []wantMetric{
				{291, "1", "min", "exact", statementKindTestParameter, ""},
			},
		},
		{
			name: "prescribed-value table versus data-set records",
			lines: []Line{
				{LineNo: 227, PageNo: 17, LineType: "paragraph", Content: "6.5.5.3 采用误差不大于1%标准电阻模型测试仪器输出的交流阻抗。试验环境温度(20±5)℃, 标准电阻模型如图1所示, 测试用电阻RA、LA、TR、RL、LL取值见表3。在5 kHz、50 kHz、250 kHz输出频率上分别对每个模型进行测试, 重复测量次数不应少于3次。"},
				{LineNo: 230, PageNo: 17, LineType: "table-caption", Content: "表 3 标准电阻模型阻值"},
				{LineNo: 231, PageNo: 17, LineType: "table-caption", Content: "单位为欧"},
				{LineNo: 233, PageNo: 17, LineType: "table", Content: "<table><tr><td>模型</td><td>RA</td><td>LA</td><td>TR</td><td>RL</td><td>LL</td></tr><tr><td>模型 1</td><td>150</td><td>150</td><td>10</td><td>150</td><td>150</td></tr><tr><td>模型 2</td><td>300</td><td>300</td><td>30</td><td>300</td><td>300</td></tr><tr><td>模型 3</td><td>430</td><td>430</td><td>51</td><td>430</td><td>430</td></tr><tr><td>模型 4</td><td>590</td><td>590</td><td>75</td><td>590</td><td>590</td></tr></table>"},
				{LineNo: 371, PageNo: 30, LineType: "paragraph", Content: "体脂率模拟测试参数库体脂率模拟测试参数库见表E.1。"},
				{LineNo: 372, PageNo: 31, LineType: "table-caption", Content: "表 E.1 体指率模拟测试参数库"},
				{LineNo: 374, PageNo: 31, LineType: "table", Content: "<table><tr><td rowspan=\"3\">序号</td><td colspan=\"31\">输入参数</td><td>输出标准值</td><td></td><td></td><td></td></tr><tr><td rowspan=\"2\">性别</td><td rowspan=\"2\">年龄岁</td><td rowspan=\"2\">身高cm</td><td rowspan=\"2\">体重kg</td><td colspan=\"5\">1 kHz</td><td colspan=\"5\">5 kHz</td><td colspan=\"5\">50 kHz</td><td colspan=\"5\">250 kHz</td><td colspan=\"5\">500 kHz</td><td colspan=\"3\">1 000 kHz</td><td>体脂率/%</td><td></td><td></td></tr><tr><td>左上肢</td><td>右上肢</td><td>躯干</td><td>左下肢</td><td>右下肢</td><td>左上肢</td><td>右上肢</td><td>躯干</td><td>左下肢</td><td>右下肢</td><td>左上肢</td><td>右上肢</td><td>躯干</td><td>左下肢</td><td>右下肢</td><td>左上肢</td><td>右上肢</td><td>躯干</td><td>左下肢</td><td>右下肢</td><td>左上肢</td><td>右上肢</td><td>躯干</td><td>左下肢</td><td>右下肢</td><td>左上肢</td><td>右上肢</td><td>躯干</td><td>左下肢</td><td>右下肢</td><td></td></tr><tr><td>1</td><td>女</td><td>17</td><td>165.0</td><td>48.0</td><td>510.2</td><td>499.7</td><td>26.8</td><td>281.7</td><td>282.1</td><td>503.9</td><td>493.9</td><td>26.0</td><td>271.3</td><td>277.7</td><td>465.0</td><td>455.9</td><td>23.0</td><td>254.1</td><td>251.8</td><td>428.9</td><td>420.2</td><td>20.1</td><td>232.8</td><td>229.8</td><td>416.1</td><td>407.5</td><td>19.0</td><td>228.7</td><td>225.4</td><td>407.0</td><td>399.9</td><td>17.7</td><td>224.6</td><td>221.3</td><td>18.8</td></tr><tr><td>2</td><td>女</td><td>18</td><td>170.0</td><td>51.0</td><td>581.1</td><td>583.4</td><td>35.6</td><td>354.1</td><td>350.0</td><td>575.3</td><td>576.5</td><td>34.7</td><td>348.7</td><td>344.8</td><td>539.7</td><td>535.8</td><td>31.3</td><td>318.7</td><td>317.3</td><td>501.9</td><td>496.6</td><td>28.0</td><td>291.7</td><td>292.3</td><td>487.0</td><td>481.7</td><td>26.6</td><td>287.0</td><td>287.1</td><td>478.8</td><td>474.8</td><td>24.9</td><td>282.6</td><td>282.7</td><td>29.6</td></tr><tr><td>3</td><td>女</td><td>18</td><td>171.0</td><td>58.0</td><td>492.3</td><td>455.8</td><td>31.3</td><td>290.6</td><td>287.2</td><td>483.8</td><td>448.0</td><td>30.3</td><td>285.4</td><td>282.4</td><td>445.9</td><td>409.0</td><td>26.6</td><td>260.5</td><td>257.9</td><td>410.8</td><td>375.3</td><td>23.5</td><td>237.7</td><td>235.3</td><td>397.9</td><td>363.7</td><td>22.3</td><td>233.1</td><td>230.7</td><td>386.6</td><td>358.2</td><td>21.0</td><td>229.0</td><td>226.6</td><td>26.3</td></tr><tr><td>4</td><td>女</td><td>18</td><td>152.0</td><td>58.0</td><td>403.6</td><td>407.0</td><td>25.1</td><td>265.5</td><td>269.3</td><td>398.4</td><td>401.8</td><td>24.3</td><td>260.5</td><td>264.4</td><td>372.6</td><td>375.2</td><td>21.7</td><td>235.4</td><td>238.9</td><td>345.9</td><td>348.7</td><td>19.1</td><td>214.1</td><td>218.3</td><td>336.6</td><td>339.6</td><td>18.0</td><td>209.6</td><td>213.8</td><td>330.8</td><td>335.0</td><td>17.2</td><td>208.9</td><td>209.7</td><td>35.4</td></tr></table>"},
			},
			want: []wantMetric{
				{233, "10", "Ω", "exact", statementKindTestParameter, "模型 1"},
				{233, "30", "Ω", "exact", statementKindTestParameter, "模型 2"},
				{233, "51", "Ω", "exact", statementKindTestParameter, "模型 3"},
				{233, "75", "Ω", "exact", statementKindTestParameter, "模型 4"},
			},
		},
		{
			name: "size written as a product is one metric",
			lines: []Line{
				{LineNo: 191, PageNo: 15, LineType: "paragraph", Content: "6.4.2.3 直接承载人体载荷或承受拉压载荷的测试器材(如:身高、体重、握力、背力、台阶试验等)的超负载能力试验,按下列规定进行。"},
				{LineNo: 192, PageNo: 15, LineType: "paragraph", Content: "a) 对承受人体站姿载荷的器材,应在人体站立的中心位置  $250 \\, mm \\times 200 \\, mm$  的面上施加按 5.4.3.3 规定的单一垂直静载荷,并保持 1 min;对于承载多人使用的器材受力构件(如:台阶试验中适用于多人踩踏的踏台等),则应按照 GB 19272—2011 中 6.4.4 和该条款的负载规定,实施 A 法(分布载荷法)或者 B 法(集中载荷法)的多人承载能力试验,并保持 1 min。"},
				{LineNo: 119, PageNo: 12, LineType: "table", Content: "<table><tr><td>测试器材</td><td>适用人群</td><td>有效量程或仪器规格要求</td><td>分度值</td><td>允许误差</td></tr><tr><td rowspan=\"2\">15 m 绕障碍跑测试仪</td><td rowspan=\"2\">幼儿</td><td>0 s~999.9 s</td><td>0.1 s</td><td>±1.5%</td></tr><tr><td>障碍物为锥桶形状,塑料或橡胶材质,底座尺寸为长300 mm×宽 300 mm 的方形或直径为 300 mm 的圆形,锥桶高度为 550 mm</td><td>—</td><td>±5 mm</td></tr></table>"},
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
				if metricLineSpansOverlap(m, map[string]any{"source_line_spans": []string{"374"}}) {
					t.Errorf("data-set record (Table E.1) emitted as a metric: %v", m)
				}
				if metricRowUnit(m) == "周岁" || (compactMetricValue(asString(m["metric_value"])) == "360" && metricRowUnit(m) == "°") {
					t.Errorf("definitional number (category boundary or counting convention) emitted as a metric: %v", m)
				}
			}
			if strings.HasPrefix(tc.name, "size written as a product") {
				sizes := map[string]int{}
				for _, m := range r.Metrics {
					v := strings.NewReplacer("x", "×", "X", "×", "*", "×").Replace(compactMetricValue(asString(m["metric_value"])))
					for _, line := range []string{"192", "119"} {
						if !metricLineSpansOverlap(m, map[string]any{"source_line_spans": []string{line}}) || metricRowUnit(m) != "mm" {
							continue
						}
						if v == "250×200" || v == "300×300" {
							sizes[line+"|"+v]++
						}
						if v == "250" || v == "200" || (line == "119" && v == "300" && !strings.Contains(asString(m["metric_name"]), "直径")) {
							t.Errorf("product size split into a single dimension: %v", m)
						}
					}
				}
				if sizes["192|250×200"] != 1 || sizes["119|300×300"] != 1 {
					t.Errorf("want one 250×200 mm row on 192 and one 300×300 mm row on 119, got %v", sizes)
				}
			}
			if strings.HasPrefix(tc.name, "prescribed-value table") {
				n := 0
				for _, m := range r.Metrics {
					if metricLineSpansOverlap(m, map[string]any{"source_line_spans": []string{"233"}}) && metricRowUnit(m) == "Ω" {
						n++
					}
				}
				if n != 20 {
					t.Errorf("Table 3 should give 20 resistor rows (one per cell), got %d", n)
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
