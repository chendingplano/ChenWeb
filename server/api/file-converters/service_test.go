package fileconverters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeStore struct {
	rec           InputRecord
	getErr        error
	updatedStatus string
	updatedError  *string
	updateCalls   int
}

type fakeSubscriber struct {
	subject string
	durable string
	handler func(context.Context, []byte) error
	err     error
}

func boolPtr(v bool) *bool { return &v }

func expectedOpenDataOutputPath(inputJSONPath string) string {
	root := strings.TrimSuffix(inputJSONPath, filepath.Ext(inputJSONPath))
	return root + "_opendata.txt"
}

func (f *fakeSubscriber) Subscribe(_ context.Context, subject string, durable string, handler func(context.Context, []byte) error) error {
	f.subject = subject
	f.durable = durable
	f.handler = handler
	return f.err
}

func (f *fakeStore) GetInputRecord(_ context.Context, id int64) (InputRecord, error) {
	if f.getErr != nil {
		return InputRecord{}, f.getErr
	}
	if id != f.rec.ID {
		return InputRecord{}, ErrNotFound
	}
	return f.rec, nil
}

func (f *fakeStore) UpdateInputStatus(_ context.Context, id int64, statusJSON string, errorMsg *string) error {
	if id != f.rec.ID {
		return ErrNotFound
	}
	f.updateCalls++
	f.updatedStatus = statusJSON
	f.updatedError = errorMsg
	return nil
}

type fakePublisher struct {
	subject string
	payload []byte
	calls   int
}

func (f *fakePublisher) Publish(_ context.Context, subject string, payload []byte) error {
	f.subject = subject
	f.payload = append([]byte(nil), payload...)
	f.calls++
	return nil
}

func TestHasParsedSuccess(t *testing.T) {
	raw := `[{"operation":"parsed","proc-status":"success"}]`
	if !HasParsedSuccess(raw) {
		t.Fatalf("expected parsed success")
	}
}

func TestShouldForceReprocess_DefaultTrue(t *testing.T) {
	if !shouldForceReprocess(ConvertRequest{}) {
		t.Fatalf("expected default force=true")
	}
	if shouldForceReprocess(ConvertRequest{Force: boolPtr(false)}) {
		t.Fatalf("expected force=false when explicitly set")
	}
}

func TestConvertOpenDataFile(t *testing.T) {
	tmp := t.TempDir()
	in := filepath.Join(tmp, "result.json")
	content := `{
  "kids": [
    {"type":"paragraph","page number":1,"font":"HiddenHorzOCR","font size":11.04,"content":"hello","bounding box":[1,2,3,4]},
    {"type":"heading","page number":2,"heading level":"Doctitle","font":"SimHei","font size":15.96,"content":"Title","bounding box":[5,6,7,8]},
    {"type":"image","page number":2,"source":"img/a.png","bounding box":[9,10,11,12]}
  ]
}`
	if err := os.WriteFile(in, []byte(content), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	out, err := ConvertOpenDataFile(in)
	if err != nil {
		t.Fatalf("ConvertOpenDataFile: %v", err)
	}

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(got)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	originPath := strings.TrimSuffix(out, filepath.Ext(out)) + ".origin"
	origin, err := os.ReadFile(originPath)
	if err != nil {
		t.Fatalf("read origin output: %v", err)
	}
	if string(origin) != string(got) {
		t.Fatalf("origin output mismatch:\norigin=%q\ntext=%q", string(origin), string(got))
	}
	info, err := os.Stat(originPath)
	if err != nil {
		t.Fatalf("stat origin output: %v", err)
	}
	if info.Mode().Perm() != 0o444 {
		t.Fatalf("origin mode=%#o, want 0444", info.Mode().Perm())
	}
	if !strings.Contains(lines[0], "1\t1\tparagraph\tHiddenHorzOCR\t11\t[1,2,3,4]\thello") {
		t.Fatalf("unexpected line1: %s", lines[0])
	}
	if !strings.Contains(lines[1], "2\t2\theading(Doctitle)\tSimHei\t16\t[5,6,7,8]\tTitle") {
		t.Fatalf("unexpected line2: %s", lines[1])
	}
	if !strings.Contains(lines[2], "3\t2\timage\tunknown-font\t12\t[9,10,11,12]\timg/a.png") {
		t.Fatalf("unexpected line3: %s", lines[2])
	}
}

func TestConvertOpenDataFile_RewritesExistingReadOnlyOriginFile(t *testing.T) {
	tmp := t.TempDir()
	in := filepath.Join(tmp, "sample.json")
	input := `{
  "number of pages": 1,
  "kids": [
    {"page number": 1, "type": "paragraph", "content": "hello", "bounding box": [1,2,3,4]}
  ]
}`
	if err := os.WriteFile(in, []byte(input), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	out, err := ConvertOpenDataFile(in)
	if err != nil {
		t.Fatalf("first ConvertOpenDataFile: %v", err)
	}
	originPath := strings.TrimSuffix(out, filepath.Ext(out)) + ".origin"
	if err := os.Chmod(originPath, 0o644); err != nil {
		t.Fatalf("make origin writable for seed: %v", err)
	}
	if err := os.WriteFile(originPath, []byte("stale\n"), 0o644); err != nil {
		t.Fatalf("seed read-only origin: %v", err)
	}
	if err := os.Chmod(originPath, 0o444); err != nil {
		t.Fatalf("make seeded origin read-only: %v", err)
	}

	out, err = ConvertOpenDataFile(in)
	if err != nil {
		t.Fatalf("second ConvertOpenDataFile: %v", err)
	}
	got, err := os.ReadFile(originPath)
	if err != nil {
		t.Fatalf("read rewritten origin: %v", err)
	}
	txt, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output txt: %v", err)
	}
	if string(got) != string(txt) {
		t.Fatalf("rewritten origin mismatch:\norigin=%q\ntext=%q", string(got), string(txt))
	}
	info, err := os.Stat(originPath)
	if err != nil {
		t.Fatalf("stat rewritten origin: %v", err)
	}
	if info.Mode().Perm() != 0o444 {
		t.Fatalf("rewritten origin mode=%#o, want 0444", info.Mode().Perm())
	}
}

func TestConvertOpenDataFile_IgnoresHeaderAndExpandsListItems(t *testing.T) {
	tmp := t.TempDir()
	in := filepath.Join(tmp, "result.json")
	content := `{
  "kids": [
    {"type":"header","page number":3,"content":"T/CHIA 14.7—2018","bounding box":[446.98,758.876,538.749,770.57]},
    {"type":"heading","page number":3,"heading level":3,"content":"目 次","bounding box":[280.73,695.056,328.73,711.016]},
    {"type":"paragraph","page number":3,"content":"前言 ..................................................................................II","bounding box":[70.944,626.455,538.66,637.015]},
    {"type":"list","page number":3,"bounding box":[70.944,525.025,538.66,613.615],"list items":[
      {"type":"list item","page number":3,"content":"1 范围 .................................................................................1","bounding box":[70.944,603.055,538.66,613.615]},
      {"type":"list item","page number":3,"content":"2 规范性引用文件 .......................................................................1","bounding box":[70.944,583.465,538.66,594.025]}
    ]}
  ]
}`
	if err := os.WriteFile(in, []byte(content), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	out, err := ConvertOpenDataFile(in)
	if err != nil {
		t.Fatalf("ConvertOpenDataFile: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(got)), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d: %v", len(lines), lines)
	}
	for _, line := range lines {
		if strings.Contains(line, " header ") || strings.Contains(line, " list [") || strings.Contains(line, " footer ") {
			t.Fatalf("unexpected container line in output: %s", line)
		}
	}
	if !strings.Contains(lines[2], "list-item") || !strings.Contains(lines[3], "list-item") {
		t.Fatalf("expected list item lines, got: %v", lines)
	}
}

func TestConvertOpenDataFile_DropsLastNonFooterPageNumber(t *testing.T) {
	tmp := t.TempDir()
	in := filepath.Join(tmp, "result.json")
	content := `{
  "kids": [
    {"type":"paragraph","page number":3,"content":"5.3 数据元值域代码表 ..............................................................10","bounding box":[91.944,466.465,538.66,477.025]},
    {"type":"paragraph","page number":3,"content":"I","bounding box":[523.66,62.934,528.16,71.934]},
    {"type":"footer","page number":3,"content":"","bounding box":[523.66,56.574,528.16,65.574]},
    {"type":"paragraph","page number":4,"content":"本标准参与起草单位 ...","bounding box":[70.944,314.525,544.158,344.525]},
    {"type":"paragraph","page number":4,"content":"II","bounding box":[519.1,56.574,528.16,65.574]}
  ]
}`
	if err := os.WriteFile(in, []byte(content), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	out, err := ConvertOpenDataFile(in)
	if err != nil {
		t.Fatalf("ConvertOpenDataFile: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	text := string(got)
	if strings.Contains(text, " 3 paragraph I ") {
		t.Fatalf("unexpected page number line I in output: %s", text)
	}
	if strings.Contains(text, " 4 paragraph II ") {
		t.Fatalf("unexpected page number line II in output: %s", text)
	}
	if !strings.Contains(text, "5.3 数据元值域代码表") {
		t.Fatalf("expected real page content to remain: %s", text)
	}
}

func TestConvertOpenDataFile_ConvertsTableToTableRows(t *testing.T) {
	tmp := t.TempDir()
	in := filepath.Join(tmp, "result.json")
	content := `{
  "kids": [
    {"type":"table","page number":6,"number of rows":3,"number of columns":3,"rows":[
      {"type":"table row","row number":1,"cells":[
        {"type":"table cell","row number":1,"column number":1,"row span":1,"column span":1,"kids":[{"type":"paragraph","content":"元数据子集"}]},
        {"type":"table cell","row number":1,"column number":2,"row span":1,"column span":1,"kids":[{"type":"paragraph","content":"元数据项"}]},
        {"type":"table cell","row number":1,"column number":3,"row span":1,"column span":1,"kids":[{"type":"paragraph","content":"元数据值"}]}
      ]},
      {"type":"table row","row number":2,"cells":[
        {"type":"table cell","row number":2,"column number":1,"row span":2,"column span":1,"kids":[{"type":"paragraph","content":"标识信息子集"}]},
        {"type":"table cell","row number":2,"column number":2,"row span":1,"column span":1,"kids":[{"type":"paragraph","content":"数据命名表名称"}]},
        {"type":"table cell","row number":2,"column number":3,"row span":1,"column span":1,"kids":[{"type":"paragraph","content":"医疗健康物联网 感知设备通信数据命名表 第 7 部分：能量检测仪"}]}
      ]},
      {"type":"table row","row number":3,"cells":[
        {"type":"table cell","row number":3,"column number":2,"row span":1,"column span":1,"kids":[{"type":"paragraph","content":"通信数据命名表特征数 据元允许值值域代码表"}]},
        {"type":"table cell","row number":3,"column number":3,"row span":1,"column span":1,"kids":[{"type":"list","list items":[
          {"type":"list item","content":"CV MHIOT.03.007.002 能量检测仪设备故障代码值域代码表"},
          {"type":"list item","content":"CV MHIOT.04.007.007 能量检测仪测量人体部位值域代码表"}
        ]}]}
      ]}
    ]}
  ]
}`
	if err := os.WriteFile(in, []byte(content), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}
	out, err := ConvertOpenDataFile(in)
	if err != nil {
		t.Fatalf("ConvertOpenDataFile: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	text := strings.TrimSpace(string(got))
	if !strings.Contains(text, "1\t6\ttable-row\tunknown-font\t12\t[]\t|元数据子集|元数据项|元数据值|") {
		t.Fatalf("missing table-row header line: %s", text)
	}
	if !strings.Contains(text, "2\t6\ttable-row\tunknown-font\t12\t[]\t|标识信息子集<br><br>|数据命名表名称|医疗健康物联网 感知设备通信数据命名表 第 7 部分：能量检测仪|") {
		t.Fatalf("missing expected first data row: %s", text)
	}
	if !strings.Contains(text, "3\t6\ttable-row\tunknown-font\t12\t[]\t||通信数据命名表特征数 据元允许值值域代码表|CV MHIOT.03.007.002 能量检测仪设备故障代码值域代码表<br>CV MHIOT.04.007.007 能量检测仪测量人体部位值域代码表|") {
		t.Fatalf("missing expected list-item joined markdown: %s", text)
	}
	if strings.Contains(text, "|---|---|---|") {
		t.Fatalf("unexpected markdown separator row in table-row format: %s", text)
	}
}

func TestConvertOpenDataFile_MergesSplitTablesAcrossPages(t *testing.T) {
	tmp := t.TempDir()
	in := filepath.Join(tmp, "result.json")
	content := `{
  "kids": [
    {"type":"table","id":120,"page number":7,"bounding box":[63.12,73.8,548.86,105.96],"number of rows":1,"number of columns":3,"next table id":123,"rows":[
      {"type":"table row","row number":1,"cells":[
        {"type":"table cell","row number":1,"column number":1,"kids":[{"type":"paragraph","content":"A"}]},
        {"type":"table cell","row number":1,"column number":2,"kids":[{"type":"paragraph","content":"B"}]},
        {"type":"table cell","row number":1,"column number":3,"kids":[{"type":"paragraph","content":"C"}]}
      ]}
    ]},
    {"type":"footer","page number":7,"bounding box":[523.66,56.574,528.16,65.574]},
    {"type":"header","page number":8,"bounding box":[70.944,758.935,160.344,769.495]},
    {"type":"table","id":123,"page number":8,"bounding box":[63.12,585.19,548.86,743.74],"number of rows":2,"number of columns":3,"previous table id":120,"rows":[
      {"type":"table row","row number":1,"cells":[
        {"type":"table cell","row number":1,"column number":1,"kids":[{"type":"paragraph","content":"r1c1"}]},
        {"type":"table cell","row number":1,"column number":2,"kids":[{"type":"paragraph","content":"r1c2"}]},
        {"type":"table cell","row number":1,"column number":3,"kids":[{"type":"paragraph","content":"r1c3"}]}
      ]},
      {"type":"table row","row number":2,"cells":[
        {"type":"table cell","row number":2,"column number":1,"kids":[{"type":"paragraph","content":"r2c1"}]},
        {"type":"table cell","row number":2,"column number":2,"kids":[{"type":"paragraph","content":"r2c2"}]},
        {"type":"table cell","row number":2,"column number":3,"kids":[{"type":"paragraph","content":"r2c3"}]}
      ]}
    ]}
  ]
}`
	if err := os.WriteFile(in, []byte(content), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	out, err := ConvertOpenDataFile(in)
	if err != nil {
		t.Fatalf("ConvertOpenDataFile: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	text := strings.TrimSpace(string(got))
	if strings.Contains(text, "|---|---|---|") {
		t.Fatalf("did not expect markdown separator rows after merge: %s", text)
	}
	if !strings.Contains(text, "1\t7\ttable-row\tunknown-font\t12\t[63.12,73.8,548.86,105.96]\t|A|B|C|") {
		t.Fatalf("missing merged header row: %s", text)
	}
	if !strings.Contains(text, "2\t8\ttable-row\tunknown-font\t12\t[63.12,585.19,548.86,743.74]\t|r1c1|r1c2|r1c3|") ||
		!strings.Contains(text, "3\t8\ttable-row\tunknown-font\t12\t[63.12,585.19,548.86,743.74]\t|r2c1|r2c2|r2c3|") {
		t.Fatalf("missing merged data rows: %s", text)
	}
}

func TestConvertOpenDataFile_TableRowEscapesPipeAndUsesRowBBox(t *testing.T) {
	tmp := t.TempDir()
	in := filepath.Join(tmp, "result.json")
	content := `{
  "kids": [
    {"type":"table","id":201,"page number":5,"number of rows":1,"number of columns":2,"rows":[
      {"type":"table row","row number":1,"cells":[
        {"type":"table cell","row number":1,"column number":1,"bounding box":[10,20,30,40],"kids":[{"type":"paragraph","content":"A|B"}]},
        {"type":"table cell","row number":1,"column number":2,"bounding box":[25,18,55,45],"kids":[{"type":"paragraph","content":"C"}]}
      ]}
    ]}
  ]
}`
	if err := os.WriteFile(in, []byte(content), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	out, err := ConvertOpenDataFile(in)
	if err != nil {
		t.Fatalf("ConvertOpenDataFile: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	text := strings.TrimSpace(string(got))
	if !strings.Contains(text, `1	5	table-row	unknown-font	12	[10,18,55,45]	|A\|B|C|`) {
		t.Fatalf("expected escaped pipe and row bbox union, got: %s", text)
	}
}

func TestConvertOpenDataFile_RemovesRepeatedContentAcrossAllPagesByDefault(t *testing.T) {
	tmp := t.TempDir()
	in := filepath.Join(tmp, "result.json")
	content := `{
  "number of pages": 3,
  "kids": [
    {"type":"paragraph","page number":1,"content":"www.weboos.com","bounding box":[10,10,20,20]},
    {"type":"paragraph","page number":1,"content":"page-1-body","bounding box":[30,30,40,40]},
    {"type":"paragraph","page number":2,"content":"www.weboos.com","bounding box":[10,10,20,20]},
    {"type":"paragraph","page number":2,"content":"page-2-body","bounding box":[30,30,40,40]},
    {"type":"paragraph","page number":3,"content":"www.weboos.com","bounding box":[10,10,20,20]},
    {"type":"paragraph","page number":3,"content":"page-3-body","bounding box":[30,30,40,40]}
  ]
}`
	if err := os.WriteFile(in, []byte(content), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	out, err := ConvertOpenDataFile(in)
	if err != nil {
		t.Fatalf("ConvertOpenDataFile: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	text := string(got)
	if strings.Contains(text, "www.weboos.com") {
		t.Fatalf("expected repeated line to be removed, got: %s", text)
	}
	if !strings.Contains(text, "page-1-body") || !strings.Contains(text, "page-2-body") || !strings.Contains(text, "page-3-body") {
		t.Fatalf("expected page body lines to remain, got: %s", text)
	}
}

func TestConvertOpenDataFile_KeepsRepeatedContentWhenEnvDisabled(t *testing.T) {
	t.Setenv("LINE_FILE_REMOVE_REPEAT_LINES", "false")

	tmp := t.TempDir()
	in := filepath.Join(tmp, "result.json")
	content := `{
  "number of pages": 2,
  "kids": [
    {"type":"paragraph","page number":1,"content":"www.weboos.com","bounding box":[10,10,20,20]},
    {"type":"paragraph","page number":1,"content":"page-1-body","bounding box":[30,30,40,40]},
    {"type":"paragraph","page number":2,"content":"www.weboos.com","bounding box":[10,10,20,20]},
    {"type":"paragraph","page number":2,"content":"page-2-body","bounding box":[30,30,40,40]}
  ]
}`
	if err := os.WriteFile(in, []byte(content), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	out, err := ConvertOpenDataFile(in)
	if err != nil {
		t.Fatalf("ConvertOpenDataFile: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	text := string(got)
	if strings.Count(text, "www.weboos.com") != 2 {
		t.Fatalf("expected repeated line to remain when disabled, got: %s", text)
	}
}

func TestConvertOpenDataFile_RemovesRepeatedContentWhenAbovePercentThreshold(t *testing.T) {
	tmp := t.TempDir()
	in := filepath.Join(tmp, "result.json")
	content := `{
  "number of pages": 10,
  "kids": [
    {"type":"paragraph","page number":1,"content":"www.weboos.com","bounding box":[10,10,20,20]},
    {"type":"paragraph","page number":2,"content":"www.weboos.com","bounding box":[10,10,20,20]},
    {"type":"paragraph","page number":3,"content":"www.weboos.com","bounding box":[10,10,20,20]},
    {"type":"paragraph","page number":4,"content":"www.weboos.com","bounding box":[10,10,20,20]},
    {"type":"paragraph","page number":5,"content":"www.weboos.com","bounding box":[10,10,20,20]},
    {"type":"paragraph","page number":6,"content":"www.weboos.com","bounding box":[10,10,20,20]},
    {"type":"paragraph","page number":7,"content":"www.weboos.com","bounding box":[10,10,20,20]},
    {"type":"paragraph","page number":8,"content":"www.weboos.com","bounding box":[10,10,20,20]},
    {"type":"paragraph","page number":9,"content":"www.weboos.com","bounding box":[10,10,20,20]},
    {"type":"paragraph","page number":10,"content":"page-10-only","bounding box":[10,10,20,20]}
  ]
}`
	if err := os.WriteFile(in, []byte(content), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	out, err := ConvertOpenDataFile(in)
	if err != nil {
		t.Fatalf("ConvertOpenDataFile: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	text := string(got)
	if strings.Contains(text, "www.weboos.com") {
		t.Fatalf("expected repeated line above threshold to be removed, got: %s", text)
	}
	if !strings.Contains(text, "page-10-only") {
		t.Fatalf("expected non-matching lines to remain, got: %s", text)
	}
}

func TestConvertOpenDataFile_KeepsRepeatedContentWhenBelowPercentThreshold(t *testing.T) {
	t.Setenv("LINE_FILE_REMOVE_REPEAT_PERCENT", "95")

	tmp := t.TempDir()
	in := filepath.Join(tmp, "result.json")
	content := `{
  "number of pages": 10,
  "kids": [
    {"type":"paragraph","page number":1,"content":"www.weboos.com","bounding box":[10,10,20,20]},
    {"type":"paragraph","page number":2,"content":"www.weboos.com","bounding box":[10,10,20,20]},
    {"type":"paragraph","page number":3,"content":"www.weboos.com","bounding box":[10,10,20,20]},
    {"type":"paragraph","page number":4,"content":"www.weboos.com","bounding box":[10,10,20,20]},
    {"type":"paragraph","page number":5,"content":"www.weboos.com","bounding box":[10,10,20,20]},
    {"type":"paragraph","page number":6,"content":"www.weboos.com","bounding box":[10,10,20,20]},
    {"type":"paragraph","page number":7,"content":"www.weboos.com","bounding box":[10,10,20,20]},
    {"type":"paragraph","page number":8,"content":"www.weboos.com","bounding box":[10,10,20,20]},
    {"type":"paragraph","page number":9,"content":"www.weboos.com","bounding box":[10,10,20,20]},
    {"type":"paragraph","page number":10,"content":"page-10-only","bounding box":[10,10,20,20]}
  ]
}`
	if err := os.WriteFile(in, []byte(content), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	out, err := ConvertOpenDataFile(in)
	if err != nil {
		t.Fatalf("ConvertOpenDataFile: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	text := string(got)
	if strings.Count(text, "www.weboos.com") != 9 {
		t.Fatalf("expected repeated line below threshold to remain, got: %s", text)
	}
}

func TestHandleRequestSuccessAppendsConvertedStatus(t *testing.T) {
	tmp := t.TempDir()
	jsonPath := filepath.Join(tmp, "ocr_result.json")
	if err := os.WriteFile(jsonPath, []byte(`{"kids":[{"type":"paragraph","page number":1,"content":"ok","bounding box":[1,2,3,4]}]}`), 0o644); err != nil {
		t.Fatalf("write json: %v", err)
	}

	st := &fakeStore{rec: InputRecord{
		ID:             11,
		Type:           "pdf",
		ParserName:     "opendata",
		StatusRaw:      `[{"operation":"parsed","proc-status":"success"}]`,
		FileName:       filepath.Join(tmp, "source.pdf"),
		ResultFilename: filepath.Base(jsonPath),
	}}

	svc := NewService(st, slog.Default())
	pub := &fakePublisher{}
	svc.Publisher = pub
	now := time.Date(2026, 4, 9, 17, 0, 30, 0, time.Local)
	svc.Now = func() time.Time { return now }

	err := svc.HandleRequest(context.Background(), ConvertRequest{
		RecordID:       11,
		ResultFilename: filepath.Base(jsonPath),
		FileFormat:     "json",
	})
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if st.updateCalls != 1 {
		t.Fatalf("expected updateCalls=1, got %d", st.updateCalls)
	}
	if st.updatedError != nil {
		t.Fatalf("expected nil error msg, got %v", *st.updatedError)
	}
	if pub.calls != 1 {
		t.Fatalf("expected 1 publish call, got %d", pub.calls)
	}
	if pub.subject != DefaultLineFileGeneratedSubject {
		t.Fatalf("unexpected publish subject: %s", pub.subject)
	}

	var entries []map[string]any
	if err := json.Unmarshal([]byte(st.updatedStatus), &entries); err != nil {
		t.Fatalf("unmarshal status: %v", err)
	}
	last := entries[len(entries)-1]
	if last["operation"] != "converted" || last["proc-status"] != "success" {
		t.Fatalf("unexpected status entry: %+v", last)
	}

	outPath := expectedOpenDataOutputPath(jsonPath)
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("expected output txt: %v", err)
	}
	originPath := strings.TrimSuffix(outPath, filepath.Ext(outPath)) + ".origin"
	if _, err := os.Stat(originPath); err != nil {
		t.Fatalf("expected output origin: %v", err)
	}
	var ev LineFileGeneratedEvent
	if err := json.Unmarshal(pub.payload, &ev); err != nil {
		t.Fatalf("unmarshal publish payload: %v", err)
	}
	if ev.RecordID != 11 || ev.Status != "success" || ev.LineFileFilename != outPath {
		t.Fatalf("unexpected event payload: %+v", ev)
	}
}

func TestHandleRequestOpenDataPrefersDocumentJSONOverParseResultWrapper(t *testing.T) {
	tmp := t.TempDir()
	wrapperPath := filepath.Join(tmp, "parse_result_20.json")
	sourceJSON := filepath.Join(tmp, "stdGk_3032172.json")

	if err := os.WriteFile(wrapperPath, []byte(`{
  "input_id": 20,
  "source_pdf": "/tmp/stdGk_3032172.pdf",
  "generated_at": "2026-04-12T13:38:05.687550+00:00",
  "engine": "opendata",
  "pages": []
}`), 0o644); err != nil {
		t.Fatalf("write wrapper json: %v", err)
	}
	if err := os.WriteFile(sourceJSON, []byte(`{
  "file name":"stdGk_3032172.pdf",
  "kids":[{"type":"paragraph","page number":1,"content":"hello","bounding box":[1,2,3,4]}]
}`), 0o644); err != nil {
		t.Fatalf("write source json: %v", err)
	}

	st := &fakeStore{rec: InputRecord{
		ID:             20,
		Type:           "pdf",
		ParserName:     "opendata",
		StatusRaw:      `[{"operation":"parsed","proc-status":"success"}]`,
		FileName:       filepath.Join(tmp, "stdGk_3032172.pdf"),
		ResultFilename: filepath.Base(wrapperPath),
	}}

	svc := NewService(st, slog.Default())
	err := svc.HandleRequest(context.Background(), ConvertRequest{RecordID: 20})
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}

	wrapperTxt := expectedOpenDataOutputPath(wrapperPath)
	if _, err := os.Stat(wrapperTxt); !os.IsNotExist(err) {
		t.Fatalf("expected no wrapper txt output, got err=%v", err)
	}
	sourceTxt := expectedOpenDataOutputPath(sourceJSON)
	out, err := os.ReadFile(sourceTxt)
	if err != nil {
		t.Fatalf("read source txt: %v", err)
	}
	if !strings.Contains(string(out), "1\t1\tparagraph\tunknown-font\t12\t[1,2,3,4]\thello") {
		t.Fatalf("unexpected source txt content: %s", string(out))
	}
}

func TestHandleRequestFailureAppendsFailedStatus(t *testing.T) {
	st := &fakeStore{rec: InputRecord{
		ID:         12,
		Type:       "pdf",
		ParserName: "opendata",
		StatusRaw:  `[]`,
		FileName:   "/tmp/source.pdf",
	}}

	svc := NewService(st, slog.Default())
	err := svc.HandleRequest(context.Background(), ConvertRequest{RecordID: 12})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, ErrMissingParsedSuccess) {
		t.Fatalf("expected ErrMissingParsedSuccess, got: %v", err)
	}
	if st.updateCalls != 1 {
		t.Fatalf("expected update status call")
	}
	if st.updatedError == nil || *st.updatedError == "" {
		t.Fatalf("expected error message to be persisted")
	}
	if !strings.Contains(st.updatedStatus, `"operation":"converted"`) || !strings.Contains(st.updatedStatus, `"proc-status":"failed"`) {
		t.Fatalf("missing converted failed status: %s", st.updatedStatus)
	}
}

func TestHandleRequestFailureDoesNotPublishLineFileGeneratedEvent(t *testing.T) {
	st := &fakeStore{rec: InputRecord{
		ID:         12,
		Type:       "pdf",
		ParserName: "opendata",
		StatusRaw:  `[]`,
		FileName:   "/tmp/source.pdf",
	}}
	pub := &fakePublisher{}

	svc := NewService(st, slog.Default())
	svc.Publisher = pub

	err := svc.HandleRequest(context.Background(), ConvertRequest{RecordID: 12})
	if err == nil {
		t.Fatalf("expected error")
	}
	if pub.calls != 0 {
		t.Fatalf("expected no publish on failed conversion, got %d", pub.calls)
	}
}

func TestHandleRequestUnsupportedParserIsNonRetryable(t *testing.T) {
	tmp := t.TempDir()
	jsonPath := filepath.Join(tmp, "source.json")
	if err := os.WriteFile(jsonPath, []byte(`{"kids":[{"type":"paragraph","content":"x"}]}`), 0o644); err != nil {
		t.Fatalf("write json: %v", err)
	}

	st := &fakeStore{rec: InputRecord{
		ID:         29,
		Type:       "pdf",
		ParserName: "mineru",
		StatusRaw:  `[{"operation":"parsed","proc-status":"success"}]`,
		FileName:   filepath.Join(tmp, "source.pdf"),
	}}

	svc := NewService(st, slog.Default())
	err := svc.HandleRequest(context.Background(), ConvertRequest{
		RecordID:       29,
		ResultFilename: filepath.Base(jsonPath),
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, ErrUnsupportedParserName) {
		t.Fatalf("expected ErrUnsupportedParserName, got: %v", err)
	}
	if st.updateCalls != 1 {
		t.Fatalf("expected update status call")
	}
	if st.updatedError == nil || !strings.Contains(*st.updatedError, `unsupported parser_name="mineru"`) {
		t.Fatalf("expected persisted unsupported parser error, got: %v", st.updatedError)
	}
}

func TestAppendConvertedStatusOverridesExistingConvertedEntry(t *testing.T) {
	raw := `[{"operation":"parsed","proc-status":"success"},{"operation":"converted","proc-status":"success","start_time":"x","ms-used":1},{"operation":"converted","proc-status":"failed","start_time":"y","ms-used":2}]`
	start := time.Date(2026, 4, 12, 8, 57, 6, 0, time.Local)
	out, err := appendConvertedStatus(raw, start, 9, errors.New("boom"))
	if err != nil {
		t.Fatalf("appendConvertedStatus: %v", err)
	}

	var entries []map[string]any
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("unmarshal status: %v", err)
	}
	converted := 0
	for _, e := range entries {
		if strings.EqualFold(fmt.Sprint(e["operation"]), "converted") {
			converted++
			if fmt.Sprint(e["proc-status"]) != "failed" {
				t.Fatalf("expected converted proc-status failed, got %v", e["proc-status"])
			}
			if fmt.Sprint(e["error"]) != "boom" {
				t.Fatalf("expected converted error boom, got %v", e["error"])
			}
		}
	}
	if converted != 1 {
		t.Fatalf("expected exactly 1 converted entry, got %d", converted)
	}
}

func TestResolveInputFileFallsBackToAnyJSONInRecordDir(t *testing.T) {
	tmp := t.TempDir()
	pdfPath := filepath.Join(tmp, "stdGk_3032172.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF"), 0o644); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	jsonPath := filepath.Join(tmp, "stdGk_3032172.json")
	if err := os.WriteFile(jsonPath, []byte(`{"kids":[{"type":"paragraph","content":"ok"}]}`), 0o644); err != nil {
		t.Fatalf("write json: %v", err)
	}

	got, err := resolveInputFile(ConvertRequest{RecordID: 20}, InputRecord{
		ID:       20,
		FileName: pdfPath,
	})
	if err != nil {
		t.Fatalf("resolveInputFile: %v", err)
	}
	if got != jsonPath {
		t.Fatalf("expected json fallback %s, got %s", jsonPath, got)
	}
}

func TestHandleRequestSkipsWhenForceFalseAndConvertedAlreadySucceeded(t *testing.T) {
	st := &fakeStore{rec: InputRecord{
		ID:         13,
		Type:       "pdf",
		ParserName: "opendata",
		StatusRaw:  `[{"operation":"parsed","proc-status":"success"},{"operation":"converted","proc-status":"success"}]`,
		FileName:   "/tmp/source.pdf",
	}}

	svc := NewService(st, slog.Default())
	err := svc.HandleRequest(context.Background(), ConvertRequest{
		RecordID: 13,
		Force:    boolPtr(false),
	})
	if err != nil {
		t.Fatalf("expected skip without error, got: %v", err)
	}
	if st.updateCalls != 0 {
		t.Fatalf("expected no status update when skipped, got %d", st.updateCalls)
	}
}

func TestRunRegistersSubscription(t *testing.T) {
	st := &fakeStore{rec: InputRecord{ID: 1}}
	svc := NewService(st, slog.Default())
	sub := &fakeSubscriber{}

	if err := svc.Run(context.Background(), sub, "kb.pdf.parsed", "convertor"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if sub.subject != "kb.pdf.parsed" {
		t.Fatalf("unexpected subject: %s", sub.subject)
	}
	if sub.durable != "convertor" {
		t.Fatalf("unexpected durable: %s", sub.durable)
	}
	if sub.handler == nil {
		t.Fatalf("expected handler to be registered")
	}
}

func TestRunRejectsInvalidArgs(t *testing.T) {
	st := &fakeStore{rec: InputRecord{ID: 1}}
	svc := NewService(st, slog.Default())

	if err := svc.Run(context.Background(), nil, "x", "y"); err == nil {
		t.Fatalf("expected error for nil subscriber")
	}
	sub := &fakeSubscriber{}
	if err := svc.Run(context.Background(), sub, " ", "y"); err == nil {
		t.Fatalf("expected error for empty subject")
	}
}

func TestHandleMessageFiltersByTypeAndStatus(t *testing.T) {
	st := &fakeStore{rec: InputRecord{
		ID:             99,
		Type:           "pdf",
		ParserName:     "opendata",
		StatusRaw:      `[{"operation":"parsed","proc-status":"success"}]`,
		FileName:       "/tmp/a.pdf",
		ResultFilename: "/tmp/a.json",
	}}
	svc := NewService(st, slog.Default())

	err := svc.HandleMessage(context.Background(), []byte(`{"record_id":99,"type":"txt","status":"success"}`))
	if err != nil {
		t.Fatalf("HandleMessage should ignore non-pdf type: %v", err)
	}
	if st.updateCalls != 0 {
		t.Fatalf("expected no update for ignored message")
	}
}
