package docprocessing

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

// record416Lines mirrors record 416 lines 113–117: prose, caption, image, table, heading.
func record416Lines() []Line {
	return []Line{
		{LineNo: 113, PageNo: 6, LineType: "paragraph", Content: "易腐垃圾及其他垃圾的主要处理模式见表1。"},
		{LineNo: 114, PageNo: 6, LineType: "table-caption", Content: "表1 易腐垃圾及其他垃圾主要处理模式"},
		{LineNo: 115, PageNo: 6, LineType: "table-image", Content: "images/b1af.jpg"},
		{LineNo: 116, PageNo: 6, LineType: "table", Content: record416Table1},
		{LineNo: 117, PageNo: 7, LineType: "heading-2", Content: "9.3 易腐垃圾处理管理要求"},
	}
}

func TestBuildTableMetricContext_Record416SpecificEnergy(t *testing.T) {
	idx := newTableLineIndex(record416Lines())
	m := map[string]any{
		"metric_id":         "416_mtc_31",
		"metric_name":       "比能耗",
		"source_line_spans": []any{"116"},
		"context":           "表1 易腐垃圾及其他垃圾主要处理模式 易腐垃圾-机器成肥",
	}
	res := buildTableMetricContext(idx, m)
	if res.Outcome != tableRowsFromEvidence {
		t.Fatalf("outcome = %q, want evidence", res.Outcome)
	}
	want := "表1 易腐垃圾及其他垃圾主要处理模式\n[r1] 序号: 1 | 垃圾类型: 易腐垃圾 | 处理模式: 机器成肥 | 技术要求: 采用机械成肥设备，经破碎预处理、好氧堆肥发酵和除杂，处理易腐垃圾。设备应明确主体工艺、比能耗、发酵周期等运行技术参数以及菌种来源要求，堆肥发酵过程符合CJJ 52无害化要求。 | 适用范围: 人口密度高，有机肥需求量较大的农村地区。"
	if res.Context != want {
		t.Fatalf("context =\n%s\nwant\n%s", res.Context, want)
	}
	if len(res.Refs) != 1 || res.Refs[0].Line != 116 || strings.Join(res.Refs[0].Rows, ",") != "r1" || len(res.Refs[0].RowHash["r1"]) != 12 {
		t.Fatalf("refs = %+v", res.Refs)
	}
	for _, other := range []string{"太阳能辅助堆肥", "厌氧产沼发酵", "卫生填埋"} {
		if strings.Contains(res.Context, other) {
			t.Errorf("stored context contains neighbor row text %q", other)
		}
	}
}

func TestBuildTableMetricContext_EvidenceTieIsNotAWinner(t *testing.T) {
	// "易腐垃圾" appears in r1, r2 and r3 — not a single winner. The table has 5 data
	// rows, so it falls through to the whole table.
	idx := newTableLineIndex(record416Lines())
	res := buildTableMetricContext(idx, map[string]any{"metric_name": "易腐垃圾", "source_line_spans": []any{"116"}})
	if res.Outcome != tableRowsWholeTable || len(res.Refs[0].Rows) != 5 {
		t.Fatalf("outcome=%q refs=%+v, want whole_table with 5 rows", res.Outcome, res.Refs)
	}
}

func TestBuildTableMetricContext_LLMRowsUsedWhenEvidenceMissing(t *testing.T) {
	idx := newTableLineIndex(record416Lines())
	m := map[string]any{
		"metric_name":       "处理能力", // not in any cell
		"source_line_spans": []any{"116"},
		"source_table_rows": tableRowRefsFromValue([]any{"116#r3", "116#r9"}),
	}
	res := buildTableMetricContext(idx, m)
	if res.Outcome != tableRowsFromLLM || strings.Join(res.Refs[0].Rows, ",") != "r3" {
		t.Fatalf("outcome=%q refs=%+v, want llm_rows [r3] (r9 dropped)", res.Outcome, res.Refs)
	}
}

func TestBuildTableMetricContext_StoredRefsWinWhenHashMatches(t *testing.T) {
	idx := newTableLineIndex(record416Lines())
	g, _ := ParseTableGrid(record416Table1)
	r4, _ := g.Row("r4")
	m := map[string]any{
		"metric_name":       "比能耗", // evidence would pick r1
		"source_line_spans": []any{"116"},
		"source_table_rows": []TableRowRef{{Line: 116, Rows: []string{"r4"}, RowHash: map[string]string{"r4": r4.Hash}}},
	}
	if res := buildTableMetricContext(idx, m); res.Outcome != tableRowsFromStoredRefs || res.Refs[0].Rows[0] != "r4" {
		t.Fatalf("got %+v, want stored r4", res)
	}
	// A stale hash is ignored and evidence decides.
	m["source_table_rows"] = []TableRowRef{{Line: 116, Rows: []string{"r4"}, RowHash: map[string]string{"r4": "000000000000"}}}
	if res := buildTableMetricContext(idx, m); res.Outcome != tableRowsFromEvidence || res.Refs[0].Rows[0] != "r1" {
		t.Fatalf("stale hash: got %+v, want evidence r1", res)
	}
}

func TestBuildTableMetricContext_UnmatchedLargeTableKeepsLLMContext(t *testing.T) {
	var b strings.Builder
	b.WriteString("<table><tr><td>项目</td><td>值</td></tr>")
	for i := 0; i < 8; i++ {
		b.WriteString("<tr><td>项</td><td>1</td></tr>")
	}
	b.WriteString("</table>")
	lines := []Line{{LineNo: 5, LineType: "table", Content: b.String()}}
	metrics := []map[string]any{{"metric_name": "温度", "source_line_spans": []any{"5"}, "context": "LLM text"}}
	counts := applyTableMetricContexts(lines, metrics, nil, 1)
	if counts[tableRowsUnmatched] != 1 || metrics[0]["context"] != "LLM text" || metrics[0]["source_table_rows"] != nil {
		t.Fatalf("counts=%v metric=%v", counts, metrics[0])
	}
}

func TestBuildTableMetricContext_LengthCaps(t *testing.T) {
	long := strings.Repeat("长", 600)
	h := "<table><tr><td>a</td><td>b</td></tr><tr><td>温度</td><td>" + long + "</td></tr></table>"
	idx := newTableLineIndex([]Line{{LineNo: 3, LineType: "table", Content: h}})
	res := buildTableMetricContext(idx, map[string]any{"metric_name": "温度", "source_line_spans": []any{"3"}})
	if !strings.HasSuffix(res.Context, "…") || utf8.RuneCountInString(res.Context) > tableContextMaxTotalRunes {
		t.Fatalf("context not capped: %d runes", utf8.RuneCountInString(res.Context))
	}
	if strings.Contains(res.Context, strings.Repeat("长", tableContextMaxCellRunes)) {
		t.Fatal("cell not capped at 400 runes")
	}
}

func TestApplyTableMetricContexts_NonTableMetricUntouched(t *testing.T) {
	metrics := []map[string]any{{"metric_name": "比能耗", "source_line_spans": []any{"113"}, "context": "prose"}}
	counts := applyTableMetricContexts(record416Lines(), metrics, nil, 416)
	if len(counts) != 0 || metrics[0]["context"] != "prose" {
		t.Fatalf("counts=%v metric=%v", counts, metrics[0])
	}
}

func TestApplyTableMetricContexts_ProseAndTableKeepsLLMContextFirst(t *testing.T) {
	metrics := []map[string]any{{"metric_name": "比能耗", "source_line_spans": []any{"113:116"}, "context": "见表1"}}
	applyTableMetricContexts(record416Lines(), metrics, nil, 416)
	ctx := asString(metrics[0]["context"])
	if !strings.HasPrefix(ctx, "见表1\n表1 易腐垃圾") || metrics[0]["metric_context"] != ctx {
		t.Fatalf("context = %q", ctx)
	}
}

func TestTableRowRefs_ParseMergeAndSQL(t *testing.T) {
	refs := tableRowRefsFromValue([]any{"116#r3", "116#r1", "bad", "116#x1", "90#h0", "116#r1"})
	bs, _ := json.Marshal(refs)
	if string(bs) != `[{"line":90,"rows":["h0"]},{"line":116,"rows":["r1","r3"]}]` {
		t.Fatalf("refs = %s", bs)
	}
	if sourceTableRowsSQLValue(nil) != nil || sourceTableRowsSQLValue([]any{}) != nil {
		t.Fatal("empty refs must bind NULL")
	}
	stored := parseTableRowRefs(`[{"line":116,"rows":["r1"],"row_hash":{"r1":"abc"}}]`)
	merged := mergeTableRowRefs(append(stored, tableRowRefsFromValue([]any{"116#r2"})...))
	if len(merged) != 1 || strings.Join(merged[0].Rows, ",") != "r1,r2" || merged[0].RowHash["r1"] != "abc" {
		t.Fatalf("merged = %+v", merged)
	}
}

func TestMergeStaticMetric_UnionsTableRows(t *testing.T) {
	existing := map[string]any{"metric_id": "1_mtc_1", "source_table_rows": []TableRowRef{{Line: 116, Rows: []string{"r1"}}}}
	cand := map[string]any{"source_table_rows": []TableRowRef{{Line: 116, Rows: []string{"r2"}}}}
	merged := mergeStaticMetric(existing, cand)
	refs := tableRowRefsFromValue(merged["source_table_rows"])
	if len(refs) != 1 || strings.Join(refs[0].Rows, ",") != "r1,r2" {
		t.Fatalf("refs = %+v", refs)
	}
}

func TestBuildTableContextWindow_EdgeAndMiddle(t *testing.T) {
	refs := []TableRowRef{{Line: 116, Rows: []string{"r1"}}}
	w, ok := BuildTableContextWindow(116, record416Table1, "表1", refs, TableContextRadius)
	if !ok {
		t.Fatal("window not built")
	}
	var ids []string
	for _, r := range w.Rows {
		ids = append(ids, r.ID)
	}
	if strings.Join(ids, ",") != "h0,r1,r2" || !w.Rows[1].Matched || w.Rows[2].Matched || !w.Rows[0].Header {
		t.Fatalf("edge window rows = %v %+v", ids, w.Rows)
	}
	w, _ = BuildTableContextWindow(116, record416Table1, "", []TableRowRef{{Line: 116, Rows: []string{"r3"}}}, 1)
	ids = ids[:0]
	for _, r := range w.Rows {
		ids = append(ids, r.ID)
	}
	if strings.Join(ids, ",") != "h0,r2,r3,r4" {
		t.Fatalf("middle window rows = %v", ids)
	}
	if !strings.Contains(w.RenderNumbered(), "116#r3 (matched): 1 | 易腐垃圾 | 厌氧产沼发酵") {
		t.Fatalf("render = %s", w.RenderNumbered())
	}
	if _, ok := BuildTableContextWindow(116, record416Table1, "", []TableRowRef{{Line: 99, Rows: []string{"r1"}}}, 1); ok {
		t.Fatal("refs for another line must not build a window")
	}
}

func TestTableMetricContext_SearchDocumentExcludesNeighborRows(t *testing.T) {
	m := map[string]any{
		"metric_id":         "416_mtc_31",
		"metric_name":       "比能耗",
		"subject":           "机器成肥设备",
		"source_line_spans": []any{"116"},
		"context":           "表1 易腐垃圾及其他垃圾主要处理模式 易腐垃圾-机器成肥",
	}
	applyTableMetricContexts(record416Lines(), []map[string]any{m}, nil, 416)
	doc := buildMetricSearchDocument(m, false)
	if !strings.Contains(doc, "人口密度高，有机肥需求量较大的农村地区。") {
		t.Fatalf("search document lacks the matched row's 适用范围: %s", doc)
	}
	for _, neighbor := range []string{"太阳能辅助堆肥", "厌氧产沼发酵", "卫生填埋", "焚烧处理"} {
		if strings.Contains(doc, neighbor) {
			t.Errorf("search document contains neighbor row text %q", neighbor)
		}
	}
}

func TestApplyTableMetricContexts_ProseAndTableIsIdempotent(t *testing.T) {
	m := map[string]any{"metric_name": "比能耗", "source_line_spans": []any{"113:116"}, "context": "见表1"}
	applyTableMetricContexts(record416Lines(), []map[string]any{m}, nil, 416)
	first := asString(m["context"])
	applyTableMetricContexts(record416Lines(), []map[string]any{m}, nil, 416)
	if asString(m["context"]) != first {
		t.Fatalf("second run changed context:\n%s\n---\n%s", first, m["context"])
	}
}

func TestTableMetricContext_MultiLineCaptionAndFullWidthRow(t *testing.T) {
	h := `<table><tr><td>模式</td><td>袖带压力</td></tr><tr><td>新生儿模式</td><td>≤5 mmHg</td></tr><tr><td colspan="2">a 测定次数没有规定</td></tr></table>`
	lines := []Line{
		{LineNo: 90, LineType: "table-caption", Content: "表 1 电子数显指示表的主要尺寸"},
		{LineNo: 91, LineType: "table-caption", Content: "单位为毫米"},
		{LineNo: 92, LineType: "table-image", Content: "images/x.jpg"},
		{LineNo: 93, LineType: "table", Content: h},
	}
	idx := newTableLineIndex(lines)
	if got := idx.caption(93); got != "表 1 电子数显指示表的主要尺寸 单位为毫米" {
		t.Fatalf("caption = %q", got)
	}
	res := buildTableMetricContext(idx, map[string]any{"metric_name": "测定次数", "source_line_spans": []any{"93"}})
	if !strings.HasSuffix(res.Context, "\n[r2] a 测定次数没有规定") {
		t.Fatalf("full-width row rendered as %q", res.Context)
	}
	g, _ := ParseTableGrid(h)
	if !strings.HasSuffix(g.RenderNumbered(93), "93#r2: a 测定次数没有规定") {
		t.Fatalf("numbered = %q", g.RenderNumbered(93))
	}
	w, _ := BuildTableContextWindow(93, h, "", []TableRowRef{{Line: 93, Rows: []string{"r2"}}}, 1)
	last := w.Rows[len(w.Rows)-1]
	if !last.FullWidth || len(last.Cells) != 1 {
		t.Fatalf("window full-width row = %+v", last)
	}
}
