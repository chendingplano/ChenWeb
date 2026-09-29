package docprocessing

import (
	"strings"
	"testing"
)

// record416Table1 is record 416 (农村生活垃圾分类处理规范) line 116, verbatim MinerU output.
const record416Table1 = `<table><tr><td>序号</td><td>垃圾类型</td><td>处理模式</td><td>技术要求</td><td>适用范围</td></tr><tr><td>1</td><td>易腐垃圾</td><td>机器成肥</td><td>采用机械成肥设备，经破碎预处理、好氧堆肥发酵和除杂，处理易腐垃圾。设备应明确主体工艺、比能耗、发酵周期等运行技术参数以及菌种来源要求，堆肥发酵过程符合CJJ 52无害化要求。</td><td>人口密度高，有机肥需求量较大的农村地区。</td></tr><tr><td rowspan="2">1</td><td rowspan="2">易腐垃圾</td><td>太阳能辅助堆肥</td><td>利用太阳能辅助堆肥方式处理易腐垃圾,应符合CJJ 52的要求。堆肥设施(阳光房)应根据垃圾日处理量合理设置单室体积,具备密封性、保温性,配备污水收集或废水和恶臭污染物达标排放处理系统。</td><td>人口密度不高,日人均生活垃圾量也相对稳定的农村地区。</td></tr><tr><td>厌氧产沼发酵</td><td>利用微生物厌氧发酵技术将易腐垃圾转化为清洁燃料沼气进行资源化利用的处理方式。设施选址应符合沼气工程安全防护要求,容积在50立方米以下的农村户用沼气池应符合NY/T 90的要求,农村沼气集中供气工程应符合NY/T 2371的要求。沼渣和沼液应有合理消纳途径。</td><td>人口密度较高、易腐垃圾量相对较大、易腐垃圾纯度高、有沼渣沼液消纳利用途径和一定沼气池使用经验的农村地区。</td></tr><tr><td rowspan="2">2</td><td rowspan="2">其他垃圾</td><td>卫生填埋</td><td>处理技术应符合GB 50869的要求,污染控制应符合GB 16889的要求。</td><td>所属区域建有生活垃圾卫生填埋场的建制村。</td></tr><tr><td>焚烧处理</td><td>处理技术应符合CJJ 90的要求,垃圾焚烧炉焚烧尾气应达标排放,飞灰、炉渣得到有效处置,污染控制应符合GB 18485的要求。</td><td>所属区域建有生活垃圾焚烧厂的建制村。</td></tr></table>`

func TestParseTableGrid_Record416RowspanExpansion(t *testing.T) {
	g, err := ParseTableGrid(record416Table1)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(g.Headers) != 1 || g.Headers[0].ID != "h0" {
		t.Fatalf("headers = %+v, want one h0", g.Headers)
	}
	wantCols := []string{"序号", "垃圾类型", "处理模式", "技术要求", "适用范围"}
	if strings.Join(g.Columns, ",") != strings.Join(wantCols, ",") {
		t.Fatalf("columns = %v, want %v", g.Columns, wantCols)
	}
	if len(g.Rows) != 5 {
		t.Fatalf("got %d data rows, want 5", len(g.Rows))
	}
	for _, id := range []string{"r2", "r3"} {
		r, _ := g.Row(id)
		if r.Cells[0] != "1" || r.Cells[1] != "易腐垃圾" {
			t.Errorf("%s = %v, want rowspan cells 1/易腐垃圾 carried", id, r.Cells)
		}
	}
	r3, _ := g.Row("r3")
	if r3.Cells[2] != "厌氧产沼发酵" || !strings.HasPrefix(r3.Cells[3], "利用微生物厌氧发酵") {
		t.Errorf("r3 misaligned: %v", r3.Cells)
	}
	r5, _ := g.Row("r5")
	if r5.Cells[1] != "其他垃圾" || r5.Cells[2] != "焚烧处理" {
		t.Errorf("r5 = %v", r5.Cells)
	}
	for _, r := range g.Rows {
		if len(r.Cells) != 5 {
			t.Errorf("%s has %d cells", r.ID, len(r.Cells))
		}
	}
}

func TestParseTableGrid_MultiLevelHeader(t *testing.T) {
	h := `<table><tr><td rowspan="2">项目</td><td colspan="2">性能指标</td></tr>` +
		`<tr><td>温度</td><td>湿度</td></tr>` +
		`<tr><td>A</td><td>20</td><td>50</td></tr></table>`
	g, err := ParseTableGrid(h)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Headers) != 2 {
		t.Fatalf("headers = %d, want 2", len(g.Headers))
	}
	want := []string{"项目", "性能指标/温度", "性能指标/湿度"}
	if strings.Join(g.Columns, ",") != strings.Join(want, ",") {
		t.Fatalf("columns = %v, want %v", g.Columns, want)
	}
	if len(g.Rows) != 1 || g.Rows[0].ID != "r1" || g.Rows[0].Cells[2] != "50" {
		t.Fatalf("rows = %+v", g.Rows)
	}
}

func TestParseTableGrid_THHeaders(t *testing.T) {
	g, err := ParseTableGrid(`<table><tr><th>a</th><th>b</th></tr><tr><td>1</td><td>2</td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Headers) != 1 || len(g.Rows) != 1 || g.Rows[0].ID != "r1" {
		t.Fatalf("grid = %+v", g)
	}
}

func TestParseTableGrid_DeterministicAndHashSensitive(t *testing.T) {
	a, _ := ParseTableGrid(record416Table1)
	b, _ := ParseTableGrid(record416Table1)
	if a.RenderNumbered(116) != b.RenderNumbered(116) {
		t.Fatal("rendering not deterministic")
	}
	for i := range a.Rows {
		if a.Rows[i].Hash != b.Rows[i].Hash || len(a.Rows[i].Hash) != 12 {
			t.Fatalf("hash mismatch on %s", a.Rows[i].ID)
		}
	}
	c, _ := ParseTableGrid(strings.Replace(record416Table1, "机器成肥", "机器堆肥", 1))
	if c.Rows[0].Hash == a.Rows[0].Hash {
		t.Fatal("hash unchanged after cell edit")
	}
	if c.Rows[1].Hash != a.Rows[1].Hash {
		t.Fatal("unrelated row hash changed")
	}
}

func TestParseTableGrid_NotATable(t *testing.T) {
	if _, err := ParseTableGrid("images/abc.jpg"); err == nil {
		t.Fatal("want error for non-table content")
	}
	if got := renderTableLineContent(7, "plain text"); got != "plain text" {
		t.Fatalf("fallback = %q", got)
	}
}

func TestRenderNumbered_Record416(t *testing.T) {
	g, _ := ParseTableGrid(record416Table1)
	out := g.RenderNumbered(116)
	lines := strings.Split(out, "\n")
	if lines[0] != "116#h0: 序号 | 垃圾类型 | 处理模式 | 技术要求 | 适用范围" {
		t.Fatalf("line 0 = %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "116#r1: 1 | 易腐垃圾 | 机器成肥 | 采用机械成肥设备") {
		t.Fatalf("line 1 = %q", lines[1])
	}
	if len(lines) != 6 {
		t.Fatalf("got %d lines, want 6", len(lines))
	}
}

func TestCanonicalChunkInputText_RendersTableRows(t *testing.T) {
	lines := []MarkedLine{
		{Line: Line{LineNo: 114, PageNo: 6, LineType: "table-caption", Content: "表1 易腐垃圾及其他垃圾主要处理模式"}, Mark: "r"},
		{Line: Line{LineNo: 116, PageNo: 6, LineType: "table", Content: record416Table1}, Mark: "r"},
		{Line: Line{LineNo: 117, PageNo: 7, LineType: "heading-2", Content: "9.3 易腐垃圾处理管理要求"}, Mark: "r"},
	}
	a := canonicalChunkInputText(lines, "doc")
	b := canonicalChunkInputText(lines, "doc")
	if a != b {
		t.Fatal("serialization not byte-identical")
	}
	if strings.Contains(a, "<table>") || strings.Contains(a, "<td>") {
		t.Fatal("raw HTML leaked into chunk input")
	}
	for _, want := range []string{`"line_number":116`, `116#h0: 序号 | 垃圾类型`, `116#r1: 1 | 易腐垃圾 | 机器成肥`, `"line_number":117`, `9.3 易腐垃圾处理管理要求`, `"content":"表1 易腐垃圾及其他垃圾主要处理模式"`} {
		if !strings.Contains(a, want) {
			t.Errorf("chunk input missing %q", want)
		}
	}
}

func TestStripTableRowRefs(t *testing.T) {
	cases := map[string]string{"116": "116", "116#r1": "116", "116#r1-116#r3": "116-116", "12:14": "12:14", "116#h0": "116"}
	for in, want := range cases {
		if got := stripTableRowRefs(in); got != want {
			t.Errorf("stripTableRowRefs(%q) = %q, want %q", in, got, want)
		}
	}
	if s, e, ok := parseMetricLineSpan("116#r1"); !ok || s != 116 || e != 116 {
		t.Errorf("parseMetricLineSpan(116#r1) = %d,%d,%v", s, e, ok)
	}
}
