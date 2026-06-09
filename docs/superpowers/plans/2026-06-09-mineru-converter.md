# MinerU Converter Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `ConvertMineruFile` in the file-converters package so that `parser_name="mineru"` records are converted to Line Files.

**Architecture:** A new `mineru.go` file alongside `opendata.go` provides `ConvertMineruFile`. It unmarshals the MinerU JSON (flat page→items structure), maps each item type to `extractedOpenDataLine` structs (the same internal type used by the opendata converter), and reuses `formatOpenDataLines` and `filterRepeatedContentLines` for output. HTML tables are parsed with `golang.org/x/net/html` (already an indirect dep). `service.go` is updated to call the new function for the `"mineru"` case.

**Tech Stack:** Go, `golang.org/x/net/html` (tokenizer), standard library `encoding/json`, `os`, `path/filepath`.

---

## Pre-flight: verify current test state

```bash
cd ChenWeb && go test ./server/api/file-converters/...
```

Expected: one failing test (`TestHandleRequestUnsupportedParserIsNonRetryable` — it uses `"mineru"` but expects `ErrUnsupportedParserName`; this test is fixed in Task 4).

---

## Chunk 1: Core implementation (`mineru.go`)

### Task 1: Create `mineru.go` with data structures, extractor, and HTML parser

**Files:**
- Create: `ChenWeb/server/api/file-converters/mineru.go`

- [ ] **Step 1: Write the failing test — basic type mapping**

Add to `ChenWeb/server/api/file-converters/service_test.go`:

```go
func TestConvertMineruFile_BasicTypes(t *testing.T) {
	tmp := t.TempDir()
	in := filepath.Join(tmp, "result_mineru.json")
	content := `{
  "pages": [
    {
      "page_number": 1,
      "items": [
        {"type":"text","text":"Title","text_level":1,"bbox":[10,20,30,40]},
        {"type":"text","text":"Body text","bbox":[10,50,30,60]},
        {"type":"header","text":"HEADER","bbox":[0,0,10,10]},
        {"type":"footer","text":"FOOTER","bbox":[0,0,10,10]},
        {"type":"page_number","text":"1","bbox":[0,0,10,10]},
        {"type":"list","list_items":["item one","item two"],"bbox":[10,70,30,90]},
        {"type":"equation","text":"$$E=mc^2$$","bbox":[10,100,30,110]}
      ]
    }
  ]
}`
	if err := os.WriteFile(in, []byte(content), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	out, err := ConvertMineruFile(in)
	if err != nil {
		t.Fatalf("ConvertMineruFile: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(got)), "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 lines (heading, paragraph, 2 list-items, equation), got %d:\n%s", len(lines), string(got))
	}
	if !strings.Contains(lines[0], "1\t1\theading(1)\tunknown-font\t12\t[10,20,30,40]\tTitle") {
		t.Fatalf("unexpected heading line: %s", lines[0])
	}
	if !strings.Contains(lines[1], "2\t1\tparagraph\tunknown-font\t12\t[10,50,30,60]\tBody text") {
		t.Fatalf("unexpected paragraph line: %s", lines[1])
	}
	if !strings.Contains(lines[2], "3\t1\tlist-item\tunknown-font\t12\t[10,70,30,90]\titem one") {
		t.Fatalf("unexpected list-item 1: %s", lines[2])
	}
	if !strings.Contains(lines[3], "4\t1\tlist-item\tunknown-font\t12\t[10,70,30,90]\titem two") {
		t.Fatalf("unexpected list-item 2: %s", lines[3])
	}
	if !strings.Contains(lines[4], "5\t1\tequation\tunknown-font\t12\t[10,100,30,110]\t$$E=mc^2$$") {
		t.Fatalf("unexpected equation line: %s", lines[4])
	}
	// header, footer, page_number must not appear
	for _, line := range lines {
		for _, banned := range []string{"HEADER", "FOOTER", "\t1\n"} {
			if strings.Contains(line, banned) {
				t.Fatalf("unexpected content %q in output line: %s", banned, line)
			}
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd ChenWeb && go test ./server/api/file-converters/... -run TestConvertMineruFile_BasicTypes -v
```

Expected: `FAIL` — `undefined: ConvertMineruFile`

- [ ] **Step 3: Create `mineru.go`**

```go
package fileconverters

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

type mineruDocument struct {
	Pages []mineruPage `json:"pages"`
}

type mineruPage struct {
	PageNumber int          `json:"page_number"`
	Items      []mineruItem `json:"items"`
}

type mineruItem struct {
	Type          string          `json:"type"`
	Text          string          `json:"text"`
	TextLevel     *int            `json:"text_level"`
	ListItems     []string        `json:"list_items"`
	TableCaption  []string        `json:"table_caption"`
	TableFootnote []string        `json:"table_footnote"`
	TableBody     string          `json:"table_body"`
	BBox          json.RawMessage `json:"bbox"`
}

func ConvertMineruFile(inputPath string) (string, error) {
	inputPath = strings.TrimSpace(inputPath)
	if inputPath == "" {
		return "", fmt.Errorf("input path is empty")
	}

	raw, err := os.ReadFile(inputPath)
	if err != nil {
		return "", fmt.Errorf("read input file: %w", err)
	}

	var doc mineruDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return "", fmt.Errorf("parse mineru json: %w", err)
	}

	items := extractMineruLineItems(doc.Pages)
	items = filterRepeatedContentLines(items, len(doc.Pages))
	lines := formatOpenDataLines(items)

	outputPath := mineruOutputPath(inputPath)
	content := strings.Join(lines, "\n")
	if len(lines) > 0 {
		content += "\n"
	}
	if err := os.WriteFile(outputPath, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write output file: %w", err)
	}
	originPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".origin"
	if err := writeReadOnlyFile(originPath, []byte(content), 0o444); err != nil {
		return "", fmt.Errorf("write origin file: %w", err)
	}

	return outputPath, nil
}

func mineruOutputPath(inputPath string) string {
	root := strings.TrimSuffix(inputPath, filepath.Ext(inputPath))
	if strings.HasSuffix(strings.ToLower(root), "_mineru") {
		return root + ".txt"
	}
	return root + "_mineru.txt"
}

func extractMineruLineItems(pages []mineruPage) []extractedOpenDataLine {
	var items []extractedOpenDataLine
	for _, page := range pages {
		pageStr := strconv.Itoa(page.PageNumber)
		for _, item := range page.Items {
			switch strings.ToLower(strings.TrimSpace(item.Type)) {
			case "header", "footer", "page_number":
				// page furniture; skip

			case "text":
				content := strings.TrimSpace(item.Text)
				if content == "" {
					continue
				}
				lineType, headingLevel := "paragraph", ""
				if item.TextLevel != nil && *item.TextLevel > 0 {
					lineType = "heading"
					headingLevel = strconv.Itoa(*item.TextLevel)
				}
				items = append(items, extractedOpenDataLine{
					Page:         pageStr,
					Type:         lineType,
					HeadingLevel: headingLevel,
					BBox:         mineruBBoxStr(item.BBox),
					Content:      content,
				})

			case "list":
				bbox := mineruBBoxStr(item.BBox)
				for _, s := range item.ListItems {
					if content := strings.TrimSpace(s); content != "" {
						items = append(items, extractedOpenDataLine{
							Page:    pageStr,
							Type:    "list-item",
							BBox:    bbox,
							Content: content,
						})
					}
				}

			case "equation":
				if content := strings.TrimSpace(item.Text); content != "" {
					items = append(items, extractedOpenDataLine{
						Page:    pageStr,
						Type:    "equation",
						BBox:    mineruBBoxStr(item.BBox),
						Content: content,
					})
				}

			case "table":
				bbox := mineruBBoxStr(item.BBox)
				for _, cap := range item.TableCaption {
					if c := strings.TrimSpace(cap); c != "" {
						items = append(items, extractedOpenDataLine{
							Page:    pageStr,
							Type:    "table-caption",
							BBox:    bbox,
							Content: c,
						})
					}
				}
				for _, row := range parseMineruHTMLTableRows(item.TableBody) {
					items = append(items, extractedOpenDataLine{
						Page:    pageStr,
						Type:    "table-row",
						BBox:    bbox,
						Content: markdownRow(row),
					})
				}
				for _, fn := range item.TableFootnote {
					if f := strings.TrimSpace(fn); f != "" {
						items = append(items, extractedOpenDataLine{
							Page:    pageStr,
							Type:    "table-footnote",
							BBox:    bbox,
							Content: f,
						})
					}
				}
			}
		}
	}
	return items
}

func parseMineruHTMLTableRows(htmlBody string) [][]string {
	if strings.TrimSpace(htmlBody) == "" {
		return nil
	}
	z := html.NewTokenizer(strings.NewReader(htmlBody))
	var rows [][]string
	var currentRow []string
	var cellBuf strings.Builder
	inCell := false
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		switch tt {
		case html.StartTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "tr":
				currentRow = []string{}
			case "td", "th":
				inCell = true
				cellBuf.Reset()
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "td", "th":
				if inCell {
					currentRow = append(currentRow, strings.TrimSpace(cellBuf.String()))
					inCell = false
				}
			case "tr":
				if currentRow != nil {
					rows = append(rows, currentRow)
					currentRow = nil
				}
			}
		case html.TextToken:
			if inCell {
				cellBuf.Write(z.Text())
			}
		}
	}
	return rows
}

func mineruBBoxStr(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return "[]"
	}
	return s
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd ChenWeb && go test ./server/api/file-converters/... -run TestConvertMineruFile_BasicTypes -v
```

Expected: `PASS`

- [ ] **Step 5: Commit**

```bash
cd ChenWeb && git add server/api/file-converters/mineru.go server/api/file-converters/service_test.go
git commit -m "feat: add mineru→line-file converter core (extraction, HTML table parser)"
```

---

### Task 2: Tests for output path, origin file, and table conversion

**Files:**
- Modify: `ChenWeb/server/api/file-converters/service_test.go`

- [ ] **Step 1: Write the tests**

Add to `service_test.go`:

```go
func TestConvertMineruFile_OutputPathNaming(t *testing.T) {
	tmp := t.TempDir()
	// Input already ends in _mineru — must not double-suffix
	in := filepath.Join(tmp, "std_1521701_mineru.json")
	if err := os.WriteFile(in, []byte(`{"pages":[]}`), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}
	out, err := ConvertMineruFile(in)
	if err != nil {
		t.Fatalf("ConvertMineruFile: %v", err)
	}
	want := filepath.Join(tmp, "std_1521701_mineru.txt")
	if out != want {
		t.Fatalf("expected %s, got %s", want, out)
	}
	if strings.Contains(filepath.Base(out), "_mineru_mineru") {
		t.Fatalf("double _mineru suffix: %s", out)
	}
}

func TestConvertMineruFile_WritesReadOnlyOriginFile(t *testing.T) {
	tmp := t.TempDir()
	in := filepath.Join(tmp, "doc_mineru.json")
	content := `{"pages":[{"page_number":1,"items":[{"type":"text","text":"hello","bbox":[1,2,3,4]}]}]}`
	if err := os.WriteFile(in, []byte(content), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}
	out, err := ConvertMineruFile(in)
	if err != nil {
		t.Fatalf("ConvertMineruFile: %v", err)
	}
	originPath := strings.TrimSuffix(out, filepath.Ext(out)) + ".origin"
	origin, err := os.ReadFile(originPath)
	if err != nil {
		t.Fatalf("read origin: %v", err)
	}
	txt, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read txt: %v", err)
	}
	if string(origin) != string(txt) {
		t.Fatalf("origin mismatch: origin=%q txt=%q", string(origin), string(txt))
	}
	info, err := os.Stat(originPath)
	if err != nil {
		t.Fatalf("stat origin: %v", err)
	}
	if info.Mode().Perm() != 0o444 {
		t.Fatalf("origin mode=%#o, want 0444", info.Mode().Perm())
	}
}

func TestConvertMineruFile_TableWithCaptionBodyFootnote(t *testing.T) {
	tmp := t.TempDir()
	in := filepath.Join(tmp, "doc_mineru.json")
	content := `{
  "pages": [
    {
      "page_number": 3,
      "items": [
        {
          "type": "table",
          "table_caption": ["表3.1 评价等级"],
          "table_footnote": ["注：分数为整数"],
          "table_body": "<table><tr><td>评分</td><td>等级</td></tr><tr><td>90</td><td>优秀</td></tr></table>",
          "bbox": [100,200,500,400]
        }
      ]
    }
  ]
}`
	if err := os.WriteFile(in, []byte(content), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}
	out, err := ConvertMineruFile(in)
	if err != nil {
		t.Fatalf("ConvertMineruFile: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	text := string(got)
	if !strings.Contains(text, "table-caption\tunknown-font\t12\t[100,200,500,400]\t表3.1 评价等级") {
		t.Fatalf("missing table-caption: %s", text)
	}
	if !strings.Contains(text, "table-row\tunknown-font\t12\t[100,200,500,400]\t|评分|等级|") {
		t.Fatalf("missing table-row header: %s", text)
	}
	if !strings.Contains(text, "table-row\tunknown-font\t12\t[100,200,500,400]\t|90|优秀|") {
		t.Fatalf("missing table-row data: %s", text)
	}
	if !strings.Contains(text, "table-footnote\tunknown-font\t12\t[100,200,500,400]\t注：分数为整数") {
		t.Fatalf("missing table-footnote: %s", text)
	}
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines (caption+2 rows+footnote), got %d:\n%s", len(lines), text)
	}
}

func TestConvertMineruFile_TableHTMLEntityDecoding(t *testing.T) {
	tmp := t.TempDir()
	in := filepath.Join(tmp, "doc_mineru.json")
	content := `{
  "pages": [
    {
      "page_number": 1,
      "items": [
        {
          "type": "table",
          "table_caption": [],
          "table_footnote": [],
          "table_body": "<table><tr><td>A&lt;B</td><td>C&amp;D</td></tr></table>",
          "bbox": [0,0,100,100]
        }
      ]
    }
  ]
}`
	if err := os.WriteFile(in, []byte(content), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}
	out, err := ConvertMineruFile(in)
	if err != nil {
		t.Fatalf("ConvertMineruFile: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	// x/net/html tokenizer decodes entities
	if !strings.Contains(string(got), "|A<B|C&D|") {
		t.Fatalf("expected HTML entities decoded, got: %s", string(got))
	}
}

func TestConvertMineruFile_TablePipeEscaping(t *testing.T) {
	tmp := t.TempDir()
	in := filepath.Join(tmp, "doc_mineru.json")
	content := `{
  "pages": [
    {
      "page_number": 1,
      "items": [
        {
          "type": "table",
          "table_caption": [],
          "table_footnote": [],
          "table_body": "<table><tr><td>A|B</td><td>C</td></tr></table>",
          "bbox": [0,0,100,100]
        }
      ]
    }
  ]
}`
	if err := os.WriteFile(in, []byte(content), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}
	out, err := ConvertMineruFile(in)
	if err != nil {
		t.Fatalf("ConvertMineruFile: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !strings.Contains(string(got), `|A\|B|C|`) {
		t.Fatalf("expected pipe escaped in cell, got: %s", string(got))
	}
}

func TestConvertMineruFile_RemovesRepeatedContent(t *testing.T) {
	tmp := t.TempDir()
	in := filepath.Join(tmp, "doc_mineru.json")
	content := `{
  "pages": [
    {"page_number":1,"items":[
      {"type":"text","text":"running-header","bbox":[0,0,10,10]},
      {"type":"text","text":"page-1-body","bbox":[10,10,20,20]}
    ]},
    {"page_number":2,"items":[
      {"type":"text","text":"running-header","bbox":[0,0,10,10]},
      {"type":"text","text":"page-2-body","bbox":[10,10,20,20]}
    ]},
    {"page_number":3,"items":[
      {"type":"text","text":"running-header","bbox":[0,0,10,10]},
      {"type":"text","text":"page-3-body","bbox":[10,10,20,20]}
    ]}
  ]
}`
	if err := os.WriteFile(in, []byte(content), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}
	out, err := ConvertMineruFile(in)
	if err != nil {
		t.Fatalf("ConvertMineruFile: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	text := string(got)
	if strings.Contains(text, "running-header") {
		t.Fatalf("expected repeated running-header removed, got: %s", text)
	}
	if !strings.Contains(text, "page-1-body") || !strings.Contains(text, "page-2-body") || !strings.Contains(text, "page-3-body") {
		t.Fatalf("expected page body lines to remain, got: %s", text)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

```bash
cd ChenWeb && go test ./server/api/file-converters/... -run "TestConvertMineruFile_Output|TestConvertMineruFile_Writes|TestConvertMineruFile_Table|TestConvertMineruFile_Removes" -v
```

Expected: `FAIL` — `ConvertMineruFile` not yet implemented (you added the tests, not the code — that's already in `mineru.go` from Task 1; these tests exercise paths not yet written)

> **Note:** If `mineru.go` from Task 1 was already committed, these tests may PASS at this point. That is expected. Continue.

- [ ] **Step 3: Run all file-converter tests**

```bash
cd ChenWeb && go test ./server/api/file-converters/... -v 2>&1 | grep -E "^(---|PASS|FAIL|ok)"
```

All new `TestConvertMineruFile_*` tests should pass. `TestHandleRequestUnsupportedParserIsNonRetryable` still fails (fixed in Task 4).

- [ ] **Step 4: Commit the new tests**

```bash
cd ChenWeb && git add server/api/file-converters/service_test.go
git commit -m "test: add ConvertMineruFile tests (output naming, origin, table, repeat filter)"
```

---

## Chunk 2: Service wiring and test fixes

### Task 3: Wire `ConvertMineruFile` into `service.go`

**Files:**
- Modify: `ChenWeb/server/api/file-converters/service.go` — line 227

- [ ] **Step 1: Write the integration test first**

Add to `service_test.go`:

```go
func expectedMineruOutputPath(inputJSONPath string) string {
	root := strings.TrimSuffix(inputJSONPath, filepath.Ext(inputJSONPath))
	if strings.HasSuffix(strings.ToLower(root), "_mineru") {
		return root + ".txt"
	}
	return root + "_mineru.txt"
}

func TestHandleRequestMineruSuccess(t *testing.T) {
	tmp := t.TempDir()
	jsonPath := filepath.Join(tmp, "std_1521701_mineru.json")
	inputJSON := `{
  "pages": [
    {"page_number":1,"items":[
      {"type":"text","text":"Hello","bbox":[1,2,3,4]}
    ]}
  ]
}`
	if err := os.WriteFile(jsonPath, []byte(inputJSON), 0o644); err != nil {
		t.Fatalf("write json: %v", err)
	}

	st := &fakeStore{rec: InputRecord{
		ID:             203,
		Type:           "pdf",
		ParserName:     "mineru",
		StatusRaw:      `[{"operation":"parsed","proc-status":"success"}]`,
		FileName:       filepath.Join(tmp, "std_1521701.pdf"),
		ResultFilename: filepath.Base(jsonPath),
	}}

	svc := NewService(st, slog.Default())
	pub := &fakePublisher{}
	svc.Publisher = pub

	err := svc.HandleRequest(context.Background(), ConvertRequest{
		RecordID:       203,
		ResultFilename: filepath.Base(jsonPath),
	})
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if st.updateCalls != 1 {
		t.Fatalf("expected updateCalls=1, got %d", st.updateCalls)
	}
	if st.updatedError != nil {
		t.Fatalf("expected nil error, got %v", *st.updatedError)
	}
	if pub.calls != 1 {
		t.Fatalf("expected 1 publish, got %d", pub.calls)
	}

	outPath := expectedMineruOutputPath(jsonPath)
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("expected output txt: %v", err)
	}
	originPath := strings.TrimSuffix(outPath, filepath.Ext(outPath)) + ".origin"
	if _, err := os.Stat(originPath); err != nil {
		t.Fatalf("expected origin file: %v", err)
	}

	var ev LineFileGeneratedEvent
	if err := json.Unmarshal(pub.payload, &ev); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	if ev.RecordID != 203 || ev.Status != "success" || ev.LineFileFilename != outPath {
		t.Fatalf("unexpected event: %+v", ev)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd ChenWeb && go test ./server/api/file-converters/... -run TestHandleRequestMineruSuccess -v
```

Expected: `FAIL` — `converter for parser_name="mineru" not supported yet: parser converter is not implemented`

- [ ] **Step 3: Update `service.go` — replace the `"mineru"` stub**

In `ChenWeb/server/api/file-converters/service.go`, find the `case "mineru":` block (currently around line 227):

```go
	case "mineru":
		return "", fmt.Errorf("(MID_26042602) converter for parser_name=%q not supported yet: %w", rec.ParserName, ErrParserNotImplemented)
```

Replace with:

```go
	case "mineru":
		out, err := ConvertMineruFile(inputFile)
		if err != nil {
			return "", fmt.Errorf("(MID_26042602) mineru convert failed: %w", err)
		}
		return out, nil
```

- [ ] **Step 4: Run the integration test**

```bash
cd ChenWeb && go test ./server/api/file-converters/... -run TestHandleRequestMineruSuccess -v
```

Expected: `PASS`

- [ ] **Step 5: Commit**

```bash
cd ChenWeb && git add server/api/file-converters/service.go server/api/file-converters/service_test.go
git commit -m "feat: wire ConvertMineruFile into service.go for parser_name=mineru"
```

---

### Task 4: Fix `TestHandleRequestUnsupportedParserIsNonRetryable`

This test currently uses `"mineru"` as an "unsupported" parser but expects `ErrUnsupportedParserName`. Now that mineru is supported, the test must use a genuinely unsupported parser name.

**Files:**
- Modify: `ChenWeb/server/api/file-converters/service_test.go`

- [ ] **Step 1: Update the test**

Find `TestHandleRequestUnsupportedParserIsNonRetryable` in `service_test.go` and replace the `ParserName: "mineru"` with `"xyz"`, and update the error message check:

Old (around line 776):
```go
	st := &fakeStore{rec: InputRecord{
		ID:         29,
		Type:       "pdf",
		ParserName: "mineru",
		StatusRaw:  `[{"operation":"parsed","proc-status":"success"}]`,
		FileName:   filepath.Join(tmp, "source.pdf"),
	}}
	...
	if st.updatedError == nil || !strings.Contains(*st.updatedError, `unsupported parser_name="mineru"`) {
		t.Fatalf("expected persisted unsupported parser error, got: %v", st.updatedError)
	}
```

New:
```go
	st := &fakeStore{rec: InputRecord{
		ID:         29,
		Type:       "pdf",
		ParserName: "xyz",
		StatusRaw:  `[{"operation":"parsed","proc-status":"success"}]`,
		FileName:   filepath.Join(tmp, "source.pdf"),
	}}
	...
	if st.updatedError == nil || !strings.Contains(*st.updatedError, `unsupported parser_name="xyz"`) {
		t.Fatalf("expected persisted unsupported parser error, got: %v", st.updatedError)
	}
```

- [ ] **Step 2: Run full test suite**

```bash
cd ChenWeb && go test ./server/api/file-converters/... -v 2>&1 | grep -E "^(---|PASS|FAIL|ok)"
```

Expected: all tests `PASS`, suite result `ok`.

- [ ] **Step 3: Commit**

```bash
cd ChenWeb && git add server/api/file-converters/service_test.go
git commit -m "fix: update unsupported-parser test to use 'xyz' now that mineru is implemented"
```

---

## Final verification

- [ ] **Run the full package test suite one more time**

```bash
cd ChenWeb && go test ./server/api/file-converters/... -count=1
```

Expected: `ok github.com/chendingplano/deepdoc/server/api/file-converters`

- [ ] **Build the whole module to catch any import or compilation issues**

```bash
cd ChenWeb && go build ./...
```

Expected: no errors.

- [ ] **Run vet**

```bash
cd ChenWeb && go vet ./server/api/file-converters/...
```

Expected: no issues.
