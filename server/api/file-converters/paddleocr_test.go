package fileconverters

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePaddleFixture(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	in := filepath.Join(dir, "std_1_paddleocr.json")
	if err := os.WriteFile(in, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return in
}

func TestConvertPaddleOCRFileFlatShape(t *testing.T) {
	in := writePaddleFixture(t, `{
	  "pages": [
	    {"page_number": 1, "parsing_res_list": [
	      {"block_label": "header", "block_content": "GB/T 1", "block_bbox": [1,1,50,10], "page_number": 1, "image": "page_1.png"},
	      {"block_label": "doc_title", "block_content": "Title", "block_bbox": [10,20,300,40], "page_number": 1, "image": "page_1.png"},
	      {"block_label": "paragraph_title", "block_content": "1 Scope", "block_bbox": [10,50,200,60], "page_number": 1, "image": "page_1.png"},
	      {"block_label": "text", "block_content": "Body text.", "block_bbox": [10,70,400,90], "page_number": 1, "image": "page_1.png"},
	      {"block_label": "table", "block_content": "<table><tr><td>a</td></tr></table>", "block_bbox": [10,100,400,200], "page_number": 1, "image": "page_1.png"},
	      {"block_label": "table_title", "block_content": "Table 1", "block_bbox": [10,95,200,99], "page_number": 1, "image": "page_1.png"},
	      {"block_label": "image", "block_content": "", "block_bbox": [10,210,400,300], "page_number": 1, "image": "page_1.png"},
	      {"block_label": "figure_title", "block_content": "Figure 1", "block_bbox": [10,305,200,315], "page_number": 1, "image": "page_1.png"},
	      {"block_label": "formula", "block_content": "x = y", "block_bbox": [10,320,200,340], "page_number": 1, "image": "page_1.png"},
	      {"block_label": "number", "block_content": "3", "block_bbox": [300,800,310,810], "page_number": 1, "image": "page_1.png"}
	    ]},
	    {"page_number": 2, "parsing_res_list": [
	      {"block_label": "text", "block_content": "Page two.", "block_bbox": [10,20,400,40], "page_number": 2, "image": "page_2.png"}
	    ]}
	  ],
	  "total_pages": 2,
	  "engine": "paddleocr"
	}`)

	out, err := ConvertPaddleOCRFile(in)
	if err != nil {
		t.Fatalf("ConvertPaddleOCRFile: %v", err)
	}
	if got, want := filepath.Base(out), "std_1_paddleocr.txt"; got != want {
		t.Fatalf("output name = %s, want %s", got, want)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	want := []string{
		"1\t1\theading(1)\tunknown-font\t12\t[10, 20, 300, 40]\tTitle",
		"2\t1\theading(2)\tunknown-font\t12\t[10, 50, 200, 60]\t1 Scope",
		"3\t1\tparagraph\tunknown-font\t12\t[10, 70, 400, 90]\tBody text.",
		"4\t1\ttable\tunknown-font\t12\t[10, 100, 400, 200]\t<table><tr><td>a</td></tr></table>",
		"5\t1\ttable-caption\tunknown-font\t12\t[10, 95, 200, 99]\tTable 1",
		"6\t1\timage-caption\tunknown-font\t12\t[10, 305, 200, 315]\tFigure 1",
		"7\t1\tequation\tunknown-font\t12\t[10, 320, 200, 340]\tx = y",
		"8\t2\tparagraph\tunknown-font\t12\t[10, 20, 400, 40]\tPage two.",
	}
	if len(lines) != len(want) {
		t.Fatalf("got %d lines, want %d:\n%s", len(lines), len(want), raw)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d:\n got %q\nwant %q", i+1, lines[i], want[i])
		}
	}
}

func TestConvertPaddleOCRFileNestedResShape(t *testing.T) {
	in := writePaddleFixture(t, `{"pages": [
	  {"res": {"parsing_res_list": [
	    {"block_label": "text", "block_content": "Nested.", "block_bbox": [1,2,3,4]}
	  ]}}
	]}`)
	out, err := ConvertPaddleOCRFile(in)
	if err != nil {
		t.Fatalf("ConvertPaddleOCRFile: %v", err)
	}
	raw, _ := os.ReadFile(out)
	if want := "1\t1\tparagraph\tunknown-font\t12\t[1, 2, 3, 4]\tNested.\n"; string(raw) != want {
		t.Fatalf("output = %q, want %q", raw, want)
	}
}

func TestPaddleOCROutputPath(t *testing.T) {
	cases := map[string]string{
		"/a/std_1_paddleocr.json": "/a/std_1_paddleocr.txt",
		"/a/std_1.json":           "/a/std_1_paddleocr.txt",
	}
	for in, want := range cases {
		if got := paddleOCROutputPath(in); got != want {
			t.Errorf("paddleOCROutputPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestConvertOneParserFileDispatchesPaddleOCR(t *testing.T) {
	in := writePaddleFixture(t, `{"pages": [{"parsing_res_list": [{"block_label": "text", "block_content": "x", "block_bbox": [1,2,3,4]}]}]}`)
	out, err := convertOneParserFile(in, "paddleocr")
	if err != nil {
		t.Fatalf("convertOneParserFile: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("expected output %s: %v", out, err)
	}
}
