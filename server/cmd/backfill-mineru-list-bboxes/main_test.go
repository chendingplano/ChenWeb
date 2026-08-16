package main

import (
	"math"
	"strings"
	"testing"
)

// mkLine builds one canonical 7-field .txt line.
func mkLine(lineNo, page int, typ, bbox, content string) string {
	return strings.Join([]string{
		itoa(lineNo), itoa(page), typ, "unknown-font", "12", bbox, content,
	}, "\t")
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}

func candidateIdx(parsed []lineRec) []int {
	var out []int
	for i, l := range parsed {
		if l.ok {
			out = append(out, i)
		}
	}
	return out
}

// The real record-416 defect: a human corrected 震荡 -> 振荡 in the .txt after
// parse, so the list's stored text no longer matches content_list exactly and
// the whole run (both items) was skipped, leaving wrong-coordinate-space bboxes.
func TestFindContentRunFuzzyToleratesPostParseTextEdit(t *testing.T) {
	jsonItem0 := "B.1.1 称取新鲜物料试样3 个，盖紧瓶盖后垂直固定于往复式水平震荡机上，在室温下震荡浸提1h。"
	txtItem0 := "B.1.1 称取新鲜物料试样3 个，盖紧瓶盖后垂直固定于往复式水平振荡机上，在室温下振荡浸提1h。"
	item1 := "B.1.2 在生物培养皿内垫一张滤纸，均匀放入10 粒水芹菜种子。"

	file := strings.Join([]string{
		mkLine(1, 11, "heading-1", "[112,228,339,243]", "B.1 植物种子发芽试验方法"),
		mkLine(2, 11, "list-item", "[68, 223, 539, 313]", txtItem0),
		mkLine(3, 11, "list-item", "[69, 317, 537, 361]", item1),
	}, "\n")

	parsed := parseLineFile(file)
	cands := candidateIdx(parsed)

	if got := findContentRun(parsed, cands, map[int]bool{}, []string{jsonItem0, item1}); got != nil {
		t.Fatalf("exact matcher should NOT match the edited text, got %v", got)
	}

	got := findContentRunFuzzy(parsed, cands, map[int]bool{}, []string{jsonItem0, item1})
	if got == nil {
		t.Fatal("fuzzy matcher failed to match a 2-char post-parse edit")
	}
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("fuzzy matched wrong lines: %v (want [1 2])", got)
	}
}

func TestFindContentRunFuzzyRejectsGenuinelyDifferentText(t *testing.T) {
	file := strings.Join([]string{
		mkLine(1, 3, "list-item", "[1,2,3,4]", "a) 标准压力计，精度不低于 0.4 级；"),
		mkLine(2, 3, "list-item", "[5,6,7,8]", "b) 秒表，分度值 0.1 s；"),
	}, "\n")
	parsed := parseLineFile(file)
	cands := candidateIdx(parsed)

	want := []string{
		"1) 测量范围： 0 kPa ～ 40 kPa；",
		"2) 听诊器传音清晰，频响符合规定；",
	}
	if got := findContentRunFuzzy(parsed, cands, map[int]bool{}, want); got != nil {
		t.Fatalf("fuzzy matcher matched unrelated text: %v", got)
	}
}

// Only list-item lines may be patched: a list item merged into a paragraph by
// structure analysis must not have a list bbox written over it.
func TestFindContentRunFuzzySkipsNonListItemLines(t *testing.T) {
	text := "a) 生产企业名称、地址、联系方式；"
	file := mkLine(1, 2, "paragraph", "[1,2,3,4]", text)
	parsed := parseLineFile(file)
	if got := findContentRunFuzzy(parsed, candidateIdx(parsed), map[int]bool{}, []string{text}); got != nil {
		t.Fatalf("fuzzy matcher patched a non-list-item line: %v", got)
	}
}

func TestFindContentRunFuzzyHonoursConsumed(t *testing.T) {
	text := "a) 生产企业名称、地址、联系方式；"
	file := strings.Join([]string{
		mkLine(1, 2, "list-item", "[1,2,3,4]", text),
		mkLine(2, 2, "list-item", "[5,6,7,8]", text),
	}, "\n")
	parsed := parseLineFile(file)
	cands := candidateIdx(parsed)

	got := findContentRunFuzzy(parsed, cands, map[int]bool{0: true}, []string{text})
	if got == nil || len(got) != 1 || got[0] != 1 {
		t.Fatalf("fuzzy matcher ignored consumed set: %v", got)
	}
}

func TestRuneSimilarity(t *testing.T) {
	cases := []struct {
		a, b string
		want float64
	}{
		{"abc", "abc", 1.0},
		{"", "", 1.0},
		{"abcd", "abce", 0.75},
		{"abc", "xyz", 0.0},
	}
	for _, c := range cases {
		if got := runeSimilarity(c.a, c.b); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("runeSimilarity(%q,%q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// The converter emits several list-item variants; restricting the fuzzy pass
// to the bare "list-item" string would exclude ~2.3k numbered/symbol list
// lines in the corpus from ever being repaired.
func TestFindContentRunFuzzyAcceptsListItemVariants(t *testing.T) {
	for _, typ := range []string{"list-item", "list-item-num", "list-item_m-sym", "list-item-s-sym"} {
		jsonText := "1) 测量范围： 0 kPa ～ 40 kPa（0 mmHg～ 300 mmHg）；"
		txtText := "1) 测量范围： 0 kPa ～ 40 kPa（0 mmHg～ 300 mmHg）:"
		parsed := parseLineFile(mkLine(1, 6, typ, "[1,2,3,4]", txtText))
		got := findContentRunFuzzy(parsed, candidateIdx(parsed), map[int]bool{}, []string{jsonText})
		if got == nil {
			t.Errorf("type %q: fuzzy matcher rejected a list-item variant", typ)
		}
	}
	// ...but still not a paragraph or heading.
	for _, typ := range []string{"paragraph", "heading-1", "table-row", "toc"} {
		text := "1) 测量范围： 0 kPa ～ 40 kPa；"
		parsed := parseLineFile(mkLine(1, 6, typ, "[1,2,3,4]", text))
		if got := findContentRunFuzzy(parsed, candidateIdx(parsed), map[int]bool{}, []string{text}); got != nil {
			t.Errorf("type %q: fuzzy matcher should not patch a non-list line: %v", typ, got)
		}
	}
}
