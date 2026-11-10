package docprocessing

import (
	"strings"
	"testing"
)

func compactTestLine(no int, lineType, content string) MarkedLine {
	return MarkedLine{Line: Line{LineNo: no, PageNo: 3, LineType: lineType, Content: content}, Mark: "r"}
}

func TestRenderLinesCompact(t *testing.T) {
	table := "<table><tr><th>项目</th><th>技术指标</th></tr><tr><td>水分</td><td>≤30</td></tr></table>"
	lines := []MarkedLine{
		compactTestLine(36, "heading-1", "3 术语和定义"),
		compactTestLine(43, "paragraph", "分类投放要定时。"),
		compactTestLine(45, "list-item", "4.1 第一\n第二"),
		compactTestLine(119, "table-caption", "表1 处理模式"),
		compactTestLine(120, "table-image", "images/abc.jpg"),
		compactTestLine(121, "table", table),
		compactTestLine(161, "equation", "$$x$$"),
		compactTestLine(162, "equation-image", "images/def.jpg"),
		compactTestLine(170, "footnote", "注：示例"),
		compactTestLine(171, "heading", "附录"),
	}
	got := RenderLinesCompact(lines, " 农村生活垃圾分类处理规范 | DB33/T 2030—2018 ")
	want := strings.Join([]string{
		"Document: 农村生活垃圾分类处理规范 | DB33/T 2030—2018",
		"[36 h1] 3 术语和定义",
		"[43] 分类投放要定时。",
		"[45] 4.1 第一 第二",
		"[119 caption] 表1 处理模式",
		"[121 table] " + llmLineContent("table", 121, table),
		"[161 eq] $$x$$",
		"[170 footnote] 注：示例",
		"[171 h] 附录",
	}, "\n")
	if got != want {
		t.Fatalf("RenderLinesCompact mismatch\n got:\n%s\nwant:\n%s", got, want)
	}
	if !strings.Contains(got, "121#") {
		t.Fatalf("table rows must keep their numbered row ids, got:\n%s", got)
	}
}

func TestRenderLinesCompactNoDocContext(t *testing.T) {
	got := RenderLinesCompact([]MarkedLine{compactTestLine(7, "text", "正文")}, "  ")
	if got != "[7] 正文" {
		t.Fatalf("got %q, want %q", got, "[7] 正文")
	}
}
