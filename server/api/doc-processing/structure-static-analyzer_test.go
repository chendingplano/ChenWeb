package docprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	llmclients "github.com/chendingplano/shared/go/api/llm"
)

type fakeTOCStructuredExtractor struct {
	calls       []string
	results     map[string]map[string]any
	errors      map[string]error
	callByModel map[string]int
}

func (f *fakeTOCStructuredExtractor) ExtractJSON(_ context.Context, in llmclients.JSONExtractionInput) (map[string]any, error) {
	if f.callByModel == nil {
		f.callByModel = map[string]int{}
	}
	model := strings.TrimSpace(in.ModelName)
	f.calls = append(f.calls, model)
	f.callByModel[model]++
	if err := f.errors[model]; err != nil {
		return nil, err
	}
	if out := f.results[model]; out != nil {
		return out, nil
	}
	return map[string]any{}, nil
}

func (f *fakeTOCStructuredExtractor) ExtractStructuredJSON(_ context.Context, in llmclients.JSONExtractionInput, _ llmclients.StructuredOutputContract) (*llmclients.StructuredOutputResult, error) {
	if f.callByModel == nil {
		f.callByModel = map[string]int{}
	}
	model := strings.TrimSpace(in.ModelName)
	f.calls = append(f.calls, model)
	f.callByModel[model]++
	if err := f.errors[model]; err != nil {
		return nil, err
	}
	if out := f.results[model]; out != nil {
		return &llmclients.StructuredOutputResult{Parsed: out}, nil
	}
	return &llmclients.StructuredOutputResult{Parsed: map[string]any{}}, nil
}

func TestStaticAnalyzer_SuccessWritesCorrectedAndStatus(t *testing.T) {
	tmp := t.TempDir()
	recordID := int64(9001)
	lineFile := filepath.Join(tmp, "ocr_rslt_9001_opendata.txt")
	body := strings.Join([]string{
		"1\t1\tparagraph\tF\t12\t[0,0,1,1]\t1 Scope",
		"2\t1\tlist-item\tF\t12\t[0,0,1,1]\t1) first item",
		"3\t1\tparagraph\tF\t12\t[0,0,1,1]\tTable of Content",
		"4\t1\tparagraph\tF\t12\t[0,0,1,1]\t1 Scope........1",
		"5\t1\tparagraph\tF\t12\t[0,0,1,1]\t2 Terms........2",
		"BROKEN\tLINE",
		"6\t2\tlist-item\tF\t12\t[0,0,1,1]\t- bullet",
		"7\t2\tlist-item\tF\t12\t[0,0,1,1]\ta) letter item",
	}, "\n")
	if err := os.WriteFile(lineFile, []byte(body), 0o644); err != nil {
		t.Fatalf("write line file: %v", err)
	}

	t.Setenv("ARTIFACT_DIR", tmp)
	t.Setenv("EXTRACT_DOCMETA_PROMPT", "false")
	store := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              recordID,
		ParserName:      "opendata",
		ResultFilename:  filepath.Join(tmp, "ocr_rslt_9001.json"),
		StagingFilename: filepath.Join(tmp, "ocr_rslt_9001.pdf"),
		FileName:        "source.pdf",
		StatusRaw:       "[]",
	}}
	p := NewStaticAnalyzerProcessor(store, nil, nil)

	if err := p.HandleEvent(context.Background(), []byte(`{"record_id":"9001"}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if store.updateCalled != 1 {
		t.Fatalf("updateCalled=%d, want 1", store.updateCalled)
	}
	if store.updateReq.ErrorMsg != nil {
		t.Fatalf("unexpected error: %v", *store.updateReq.ErrorMsg)
	}

	var status []map[string]any
	if err := json.Unmarshal([]byte(store.updateReq.StatusRaw), &status); err != nil {
		t.Fatalf("status json: %v", err)
	}
	if len(status) != 1 {
		t.Fatalf("status len=%d, want 1", len(status))
	}
	row := status[0]
	if got := strings.TrimSpace(asString(row["record_id"])); got != "9001" {
		t.Fatalf("record_id=%q, want 9001", got)
	}
	if got := strings.TrimSpace(asString(row["file_type"])); got != "pdf" {
		t.Fatalf("file_type=%q, want pdf", got)
	}
	if got := strings.TrimSpace(asString(row["operation"])); got != "static_analzyer" {
		t.Fatalf("operation=%q, want static_analzyer", got)
	}
	if got := strings.TrimSpace(asString(row["proc_status"])); got != "success" {
		t.Fatalf("proc_status=%q, want success", got)
	}
	if got := int(toFloat(row["num_lines"])); got != 8 {
		t.Fatalf("num_lines=%d, want 8", got)
	}
	if got := int(toFloat(row["num_labeled_lines"])); got != 7 {
		t.Fatalf("num_labeled_lines=%d, want 7", got)
	}

	outPath := filepath.Join(tmp, "9", "9001", "ocr_rslt_9001_opendata.corrected")
	bs, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read corrected file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(bs)), "\n")
	if len(lines) != 7 {
		t.Fatalf("corrected lines=%d, want 7", len(lines))
	}

	gotByLine := map[string]string{}
	for _, ln := range lines {
		fields := strings.Split(ln, "\t")
		if len(fields) != 8 {
			t.Fatalf("corrected field count=%d, want 8, line=%q", len(fields), ln)
		}
		gotByLine[fields[0]] = fields[3]
	}
	if gotByLine["1"] != "heading-1" {
		t.Fatalf("line1 corrected=%q, want heading-1", gotByLine["1"])
	}
	if gotByLine["2"] != "list-item-num" {
		t.Fatalf("line2 corrected=%q, want list-item-num", gotByLine["2"])
	}
	if gotByLine["3"] != "toc" || gotByLine["4"] != "toc" || gotByLine["5"] != "toc" {
		t.Fatalf("toc lines got: 3=%q 4=%q 5=%q, want all toc", gotByLine["3"], gotByLine["4"], gotByLine["5"])
	}
	if gotByLine["6"] != "list-item-s-sym" {
		t.Fatalf("line6 corrected=%q, want list-item-s-sym", gotByLine["6"])
	}
	if gotByLine["7"] != "list-item_m-sym" {
		t.Fatalf("line7 corrected=%q, want list-item_m-sym", gotByLine["7"])
	}
}

func TestStaticAnalyzer_MissingArtifactDirFailsFast(t *testing.T) {
	t.Setenv("ARTIFACT_DIR", "")
	t.Setenv("EXTRACT_DOCMETA_PROMPT", "false")
	store := &fakeDocMetadataStore{}
	p := NewStaticAnalyzerProcessor(store, nil, nil)
	err := p.HandleEvent(context.Background(), []byte(`{"record_id":"1"}`))
	if err == nil || !strings.Contains(err.Error(), "missing ARTIFACT_DIR") {
		t.Fatalf("err=%v, want missing ARTIFACT_DIR", err)
	}
}

func TestStaticAnalyzer_StatusReplacesExistingEntry(t *testing.T) {
	start := time.Date(2026, 4, 23, 10, 0, 0, 0, time.UTC)
	raw := `[{"operation":"static_analyzer","proc_status":"failed"},{"operation":"chunking","proc_status":"success"}]`
	got, err := appendStaticAnalyzerStatus(raw, staticStatusParams{
		RecordID:        42,
		FileType:        "pdf",
		InputFilename:   "x.txt",
		NumPages:        2,
		NumLines:        5,
		NumLabeledLines: 4,
		Start:           start,
		DurationMs:      12,
		ProcErr:         nil,
	})
	if err != nil {
		t.Fatalf("appendStaticAnalyzerStatus: %v", err)
	}
	var arr []map[string]any
	if err := json.Unmarshal([]byte(got), &arr); err != nil {
		t.Fatalf("status json: %v", err)
	}
	if len(arr) != 2 {
		t.Fatalf("status len=%d, want 2", len(arr))
	}
	if strings.TrimSpace(asString(arr[0]["record_id"])) != "42" {
		t.Fatalf("first record_id=%q, want 42", asString(arr[0]["record_id"]))
	}
	if strings.TrimSpace(asString(arr[0]["file_type"])) != "pdf" {
		t.Fatalf("first file_type=%q, want pdf", asString(arr[0]["file_type"]))
	}
	if strings.TrimSpace(asString(arr[0]["operation"])) != "static_analzyer" {
		t.Fatalf("first operation=%q, want static_analzyer", asString(arr[0]["operation"]))
	}
	if strings.TrimSpace(asString(arr[0]["proc_status"])) != "success" {
		t.Fatalf("first proc_status=%q, want success", asString(arr[0]["proc_status"]))
	}
}

func TestStaticAnalyzer_FailureStatusOmitsSuccessOnlyCounts(t *testing.T) {
	start := time.Date(2026, 4, 23, 10, 0, 0, 0, time.UTC)
	got, err := appendStaticAnalyzerStatus("[]", staticStatusParams{
		RecordID:        7,
		FileType:        "docx",
		InputFilename:   "broken.txt",
		NumPages:        2,
		NumLines:        5,
		NumLabeledLines: 4,
		Start:           start,
		DurationMs:      12,
		ProcErr:         errors.New("boom"),
	})
	if err != nil {
		t.Fatalf("appendStaticAnalyzerStatus: %v", err)
	}
	var arr []map[string]any
	if err := json.Unmarshal([]byte(got), &arr); err != nil {
		t.Fatalf("status json: %v", err)
	}
	if len(arr) != 1 {
		t.Fatalf("status len=%d, want 1", len(arr))
	}
	row := arr[0]
	if strings.TrimSpace(asString(row["operation"])) != "static_analzyer" {
		t.Fatalf("operation=%q, want static_analzyer", asString(row["operation"]))
	}
	if strings.TrimSpace(asString(row["proc_status"])) != "failed" {
		t.Fatalf("proc_status=%q, want failed", asString(row["proc_status"]))
	}
	if _, ok := row["num_pages"]; ok {
		t.Fatalf("num_pages should be omitted on failure, got %v", row["num_pages"])
	}
	if _, ok := row["num_lines"]; ok {
		t.Fatalf("num_lines should be omitted on failure, got %v", row["num_lines"])
	}
	if _, ok := row["num_labeled_lines"]; ok {
		t.Fatalf("num_labeled_lines should be omitted on failure, got %v", row["num_labeled_lines"])
	}
}

func TestAnalyzeStaticStructure_LogsProcessingSteps(t *testing.T) {
	logger := &fakeLogger{}
	body := strings.Join([]string{
		"1\t1\tparagraph\tF\t12\t[0,0,1,1]\t1 Scope",
		"2\t1\tparagraph\tF\t12\t[0,0,1,1]\t1.1 Purpose",
		"3\t1\tlist-item\tF\t12\t[0,0,1,1]\t1) first item",
	}, "\n")

	if _, err := analyzeStaticStructure([]byte(body), logger); err != nil {
		t.Fatalf("analyzeStaticStructure: %v", err)
	}

	wantMessages := []string{
		"remove full-page image artifact lines invoked",
		"remove weboos watermark lines invoked",
		"detect table of content invoked",
		"correct headings invoked",
		"detect headings invoked",
		"merge lines invoked",
		"detect item lists invoked",
	}
	for _, want := range wantMessages {
		if _, ok := findInfoLog(logger.infos, want); !ok {
			t.Fatalf("missing log %q in %#v", want, logger.infos)
		}
	}
}

func TestStaticAnalyzer_NumericalHeadingPrediction(t *testing.T) {
	body := strings.Join([]string{
		"1\t1\tparagraph\tF\t12\t[0,0,1,1]\t3 Main",
		"2\t1\tparagraph\tF\t12\t[0,0,1,1]\t3.1 Sub",
		"3\t1\tparagraph\tF\t12\t[0,0,1,1]\t3.1.1 Detail",
		"4\t1\tparagraph\tF\t12\t[0,0,1,1]\t3.1.3 Invalid jump",
		"5\t1\tparagraph\tF\t12\t[0,0,1,1]\t3.2 Next sibling",
		"6\t1\tparagraph\tF\t12\t[0,0,1,1]\t4.0.1 Discontinued style",
		"7\t1\tparagraph\tF\t12\t[0,0,1,1]\t4 Next top",
	}, "\n")
	out, err := analyzeStaticStructure([]byte(body), nil)
	if err != nil {
		t.Fatalf("analyzeStaticStructure: %v", err)
	}
	if got := out.CorrectedType[1]; got != "heading-1" {
		t.Fatalf("line1=%q, want heading-1", got)
	}
	if got := out.CorrectedType[2]; got != "heading-2" {
		t.Fatalf("line2=%q, want heading-2", got)
	}
	if got := out.CorrectedType[3]; got != "heading-3" {
		t.Fatalf("line3=%q, want heading-3", got)
	}
	if got := out.CorrectedType[4]; got != "unchanged" {
		t.Fatalf("line4=%q, want unchanged", got)
	}
	if got := out.CorrectedType[5]; got != "heading-2" {
		t.Fatalf("line5=%q, want heading-2", got)
	}
	if got := out.CorrectedType[6]; got != "unchanged" {
		t.Fatalf("line6=%q, want unchanged (before top-level transition)", got)
	}
	if got := out.CorrectedType[7]; got != "heading-1" {
		t.Fatalf("line7=%q, want heading-1", got)
	}
}

func TestStaticAnalyzer_AppendixHeadingPrediction(t *testing.T) {
	body := strings.Join([]string{
		"1\t1\tparagraph\tF\t12\t[0,0,1,1]\tA.1 Scope",
		"2\t1\tparagraph\tF\t12\t[0,0,1,1]\tA.1.1 Detail",
		"3\t1\tparagraph\tF\t12\t[0,0,1,1]\tA.1.3 Invalid jump",
		"4\t1\tparagraph\tF\t12\t[0,0,1,1]\tA.2 Next",
		"5\t1\tparagraph\tF\t12\t[0,0,1,1]\tB.1 New appendix",
	}, "\n")
	out, err := analyzeStaticStructure([]byte(body), nil)
	if err != nil {
		t.Fatalf("analyzeStaticStructure: %v", err)
	}
	if got := out.CorrectedType[1]; got != "heading-1" {
		t.Fatalf("line1=%q, want heading-1", got)
	}
	if got := out.CorrectedType[2]; got != "heading-2" {
		t.Fatalf("line2=%q, want heading-2", got)
	}
	if got := out.CorrectedType[3]; got != "unchanged" {
		t.Fatalf("line3=%q, want unchanged", got)
	}
	if got := out.CorrectedType[4]; got != "heading-1" {
		t.Fatalf("line4=%q, want heading-1", got)
	}
	if got := out.CorrectedType[5]; got != "heading-1" {
		t.Fatalf("line5=%q, want heading-1", got)
	}
}

func TestParseStaticNumericHeading_NormalizesOCRZero(t *testing.T) {
	parts, title, ok := parseStaticNumericHeading("1. O. 2 本标准适用于民用建筑绿色性能的评价。")
	if !ok {
		t.Fatalf("parseStaticNumericHeading returned ok=false")
	}
	if len(parts) != 3 || parts[0] != 1 || parts[1] != 0 || parts[2] != 2 {
		t.Fatalf("parts=%v, want [1 0 2]", parts)
	}
	if title != "本标准适用于民用建筑绿色性能的评价。" {
		t.Fatalf("title=%q, want expected body", title)
	}
}

func TestNormalizeStaticHeadingContent_OCRZeroSpacingVariants(t *testing.T) {
	testCases := []struct {
		name    string
		input   string
		want    string
		changed bool
	}{
		{
			name:    "already compact zero form",
			input:   "2.0.1 工作环境 working environment",
			want:    "2.0.1 工作环境 working environment",
			changed: false,
		},
		{
			name:    "space around zero",
			input:   "2. 0.1 工作环境 working environment",
			want:    "2.0.1 工作环境 working environment",
			changed: true,
		},
		{
			name:    "ocr o with both spaces",
			input:   "2. o. 2 工作地点 working site 时停留",
			want:    "2.0.2 工作地点 working site 时停留",
			changed: true,
		},
		{
			name:    "ocr o without spaces",
			input:   "2.o.3 有害气体 harmful g康造成危害的气",
			want:    "2.0.3 有害气体 harmful g康造成危害的气",
			changed: true,
		},
		{
			name:    "ocr o with left space only",
			input:   "2. o.4 有毒气体 toxic gas 通动物并能引起人体",
			want:    "2.0.4 有毒气体 toxic gas 通动物并能引起人体",
			changed: true,
		},
		{
			name:    "ocr uppercase o",
			input:   "3. O. 2 Heading",
			want:    "3.0.2 Heading",
			changed: true,
		},
		{
			name:    "ocr s becomes five",
			input:   "s.2 Heading",
			want:    "5.2 Heading",
			changed: true,
		},
		{
			name:    "ocr uppercase s becomes five",
			input:   "S.3 Heading",
			want:    "5.3 Heading",
			changed: true,
		},
		{
			name:    "ocr lowercase l becomes one",
			input:   "2.l.3 Heading",
			want:    "2.1.3 Heading",
			changed: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, changed := normalizeStaticHeadingContent(tc.input)
			if got != tc.want {
				t.Fatalf("normalizeStaticHeadingContent(%q)=%q, want %q", tc.input, got, tc.want)
			}
			if changed != tc.changed {
				t.Fatalf("changed=%v, want %v", changed, tc.changed)
			}
		})
	}
}

func TestStaticAnalyzer_NormalizesOCRZeroSpacingInDetectedHeadings(t *testing.T) {
	body := strings.Join([]string{
		"76\t7\theading-1\tHiddenHorzOCR\t7\t[121.2,326.71,172.33,336.01]\t2 术语",
		"77\t7\theading-2\tTimes-Roman\t7\t[49.16,296.826,170.259,304.526]\t2. 0.1 工作环境 working environment",
		"78\t7\tparagraph\tHiddenHorzOCR\t6\t[62.89,285.73,198.97,293.53]\t工作场所及周围空间的安全卫生状态和条件。",
		"79\t7\theading-2\tTimes-Roman\t10\t[49.16,264.61,244.571,286.01]\t2. o. 2 工作地点 working site 时停留",
		"80\t7\tparagraph\tHiddenHorzOCR\t6\t[49.44,254.05,72.72,261.85]\t的地点。",
		"81\t7\theading-2\tTimes-Roman\t10\t[49.16,233.23,245.053,254.56]\t2. o. 3 有害气体 harmful g康造成危害的气",
		"82\t7\tparagraph\tHiddenHorzOCR\t6\t[48.97,222.67,198.97,230.17]\t体、蒸汽、雾或含有有毒粉尘的混合气体的总称。",
		"83\t7\theading-2\tTimes-Roman\t10\t[49.16,201.73,245.291,223.13]\t2. o. 4 有毒气体 toxic gas 通动物并能引起人体",
	}, "\n")

	out, err := analyzeStaticStructure([]byte(body), nil)
	if err != nil {
		t.Fatalf("analyzeStaticStructure: %v", err)
	}

	wantContent := map[int]string{
		77: "2.0.1 工作环境 working environment",
		79: "2.0.2 工作地点 working site 时停留",
		81: "2.0.3 有害气体 harmful g康造成危害的气",
		83: "2.0.4 有毒气体 toxic gas 通动物并能引起人体",
	}
	wantTypes := map[int]string{
		76: "heading-1",
		77: "heading-2",
		79: "heading-2",
		81: "heading-2",
		83: "heading-2",
	}

	gotByLine := make(map[int]staticInputLine, len(out.Lines))
	for _, line := range out.Lines {
		gotByLine[line.LineNo] = line
	}

	for lineNo, want := range wantContent {
		line, ok := gotByLine[lineNo]
		if !ok {
			t.Fatalf("line %d missing from output", lineNo)
		}
		if line.Content != want {
			t.Fatalf("line %d content=%q, want %q", lineNo, line.Content, want)
		}
	}
	for lineNo, want := range wantTypes {
		if got := out.CorrectedType[lineNo]; got != want {
			t.Fatalf("line %d corrected=%q, want %q", lineNo, got, want)
		}
	}
}

func TestStaticAnalyzer_WriteCorrectedArtifact_PreservesNormalizedHeadingContent(t *testing.T) {
	tmp := t.TempDir()
	p := &StaticAnalyzerProcessor{
		ArtifactDir:    tmp,
		OverrideOrigin: false,
	}

	out := staticAnalyzeResult{
		Lines: []staticInputLine{
			{
				LineNo:            77,
				PageNo:            7,
				OriginalLineType:  "heading-2",
				OriginalLineLower: "heading-2",
				Font:              "Times-Roman",
				FontSize:          "7",
				Coordinate:        "[49.16,296.826,170.259,304.526]",
				Content:           "2.0.1 工作环境 working environment",
			},
			{
				LineNo:            79,
				PageNo:            7,
				OriginalLineType:  "heading-2",
				OriginalLineLower: "heading-2",
				Font:              "Times-Roman",
				FontSize:          "10",
				Coordinate:        "[49.16,264.61,244.571,286.01]",
				Content:           "2.0.2 工作地点 working site 时停留",
			},
		},
		CorrectedType: map[int]string{
			77: "heading-2",
			79: "heading-2",
		},
	}

	if err := p.writeCorrectedArtifact(97, "ocr_rslt_97_opendata.txt", filepath.Join(tmp, "ignored.txt"), out); err != nil {
		t.Fatalf("writeCorrectedArtifact: %v", err)
	}

	outPath := filepath.Join(tmp, "0", "97", "ocr_rslt_97_opendata.corrected")
	bs, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read corrected file: %v", err)
	}
	got := strings.TrimSpace(string(bs))
	want := strings.Join([]string{
		"1\t7\theading-2\theading-2\tTimes-Roman\t7\t[49.16,296.826,170.259,304.526]\t2.0.1 工作环境 working environment",
		"2\t7\theading-2\theading-2\tTimes-Roman\t10\t[49.16,264.61,244.571,286.01]\t2.0.2 工作地点 working site 时停留",
	}, "\n")
	if got != want {
		t.Fatalf("corrected artifact=\n%s\nwant=\n%s", got, want)
	}
}

func TestParseStaticNumericHeading_NormalizesAdditionalOCRDigits(t *testing.T) {
	parts, title, ok := parseStaticNumericHeading("S.l.3 Heading body")
	if !ok {
		t.Fatalf("parseStaticNumericHeading returned ok=false")
	}
	if len(parts) != 3 || parts[0] != 5 || parts[1] != 1 || parts[2] != 3 {
		t.Fatalf("parts=%v, want [5 1 3]", parts)
	}
	if title != "Heading body" {
		t.Fatalf("title=%q, want %q", title, "Heading body")
	}
}

func TestParseStaticInputLine_NormalizesLegacyHeadingType(t *testing.T) {
	line, err := parseStaticInputLine("1\t1\theading(2)\tF\t12\t[0,0,1,1]\tScope")
	if err != nil {
		t.Fatalf("parseStaticInputLine: %v", err)
	}
	if got := line.OriginalLineType; got != "heading-2" {
		t.Fatalf("OriginalLineType=%q, want heading-2", got)
	}
	if got := line.OriginalLineLower; got != "heading-2" {
		t.Fatalf("OriginalLineLower=%q, want heading-2", got)
	}
}

func TestParseStaticInputLine_NormalizesBareHeadingType(t *testing.T) {
	line, err := parseStaticInputLine("1\t1\theading \tF\t12\t[0,0,1,1]\tScope")
	if err != nil {
		t.Fatalf("parseStaticInputLine: %v", err)
	}
	if got := line.OriginalLineType; got != "heading-1" {
		t.Fatalf("OriginalLineType=%q, want heading-1", got)
	}
	if got := line.OriginalLineLower; got != "heading-1" {
		t.Fatalf("OriginalLineLower=%q, want heading-1", got)
	}
}

func TestStaticAnalyzer_DoesNotLeakTOCToNextPage(t *testing.T) {
	body := strings.Join([]string{
		"80\t7\tparagraph\tHiddenHorzOCR\t9\t[0,0,1,1]\t目录",
		"81\t7\tparagraph\tHiddenHorzOCR\t9\t[0,0,1,1]\t1 总则…………………… 1",
		"83\t7\tparagraph\tHiddenHorzOCR\t9\t[0,0,1,1]\t条文说明………………………………………………………… 4",
		"84\t8\timage\tunknown-font\t12\t[-4.68,-3.12,407.4,601.32]\tstd_1573283_images/imageFile8.png",
		"85\t8\tparagraph\tHiddenHorzOCR\t11\t[0,0,1,1]\t1 总则",
		"86\t8\tparagraph\tHiddenHorzOCR\t9\t[0,0,1,1]\t1.0.1 为贯彻落实绿色发展理念，推进绿色建筑高质量发展，",
		"87\t8\tparagraph\tHiddenHorzOCR\t9\t[0,0,1,1]\t节约资源，保护环境，满足人民日益增长的美好生活需要，制定",
		"88\t8\tparagraph\tHiddenHorzOCR\t9\t[0,0,1,1]\t本标准。",
		"89\t8\tlist-item\tHelvetica\t10\t[0,0,1,1]\t1. O. 2 本标准适用于民用建筑绿色性能的评价。",
	}, "\n")
	out, err := analyzeStaticStructure([]byte(body), nil)
	if err != nil {
		t.Fatalf("analyzeStaticStructure: %v", err)
	}
	if got := out.CorrectedType[85]; got != "heading-1" {
		t.Fatalf("line85=%q, want heading-1", got)
	}
	if got := out.CorrectedType[86]; got != "heading-2" {
		t.Fatalf("line86=%q, want heading-2", got)
	}
	if got := out.CorrectedType[87]; got == "toc" || got == "heading-1" || got == "heading-2" {
		t.Fatalf("line87=%q, expected body text classification", got)
	}
	if got := out.CorrectedType[88]; got == "toc" || got == "heading-1" || got == "heading-2" {
		t.Fatalf("line88=%q, expected body text classification", got)
	}
	if got := out.CorrectedType[89]; got == "toc" {
		t.Fatalf("line89=%q, want non-toc", got)
	}
}

func TestStaticAnalyzer_RemovesRepeatedPageImageArtifacts(t *testing.T) {
	body := strings.Join([]string{
		"1\t1\timage\tunknown-font\t12\t[0,0,624,879.12]\tstd_20039_images/imageFile1.png",
		"2\t1\tparagraph\tF\t12\t[0,0,1,1]\t1 Scope",
		"3\t1\tparagraph\tF\t12\t[0,0,1,1]\t1.1 Purpose",
		"10\t2\timage\tunknown-font\t12\t[0,0,624,879.12]\tstd_20039_images/imageFile2.png",
		"11\t2\tparagraph\tF\t12\t[0,0,1,1]\t2 Terms",
	}, "\n")

	out, err := analyzeStaticStructure([]byte(body), nil)
	if err != nil {
		t.Fatalf("analyzeStaticStructure: %v", err)
	}
	if got := out.NumLines; got != 5 {
		t.Fatalf("NumLines=%d, want 5", got)
	}
	if got := len(out.Lines); got != 3 {
		t.Fatalf("len(Lines)=%d, want 3 after removing page image artifacts", got)
	}
	for _, removed := range []int{1, 10} {
		if _, ok := out.CorrectedType[removed]; ok {
			t.Fatalf("line %d still present in corrected map", removed)
		}
	}
	if got := out.CorrectedType[2]; got != "heading-1" {
		t.Fatalf("line2=%q, want heading-1", got)
	}
	if got := out.CorrectedType[3]; got != "heading-2" {
		t.Fatalf("line3=%q, want heading-2", got)
	}
	if got := out.CorrectedType[11]; got != "heading-1" {
		t.Fatalf("line11=%q, want heading-1", got)
	}
}

func TestStaticAnalyzer_RemovesRepeatedPageImageArtifactsWithNearOriginCoordinates(t *testing.T) {
	body := strings.Join([]string{
		"18\t3\timage\tunknown-font\t12\t[-0.24,-0.24,281.28,397.2]\tstd_33830_images/imageFile3.png",
		"19\t3\tparagraph\tF\t12\t[0,0,1,1]\t3 Definitions",
		"42\t5\timage\tunknown-font\t12\t[-0.24,-0.24,281.28,397.2]\tstd_33830_images/imageFile5.png",
		"43\t5\tparagraph\tF\t12\t[0,0,1,1]\t5 Requirements",
		"257\t18\timage\tunknown-font\t12\t[-2.64,-1.92,283.44,398.88]\tstd_33830_images/imageFile18.png",
		"258\t18\tparagraph\tF\t12\t[0,0,1,1]\t18 Appendix",
	}, "\n")

	out, err := analyzeStaticStructure([]byte(body), nil)
	if err != nil {
		t.Fatalf("analyzeStaticStructure: %v", err)
	}
	if got := len(out.Lines); got != 3 {
		t.Fatalf("len(Lines)=%d, want 3 after removing near-origin image artifacts", got)
	}
	for _, removed := range []int{18, 42, 257} {
		if _, ok := out.CorrectedType[removed]; ok {
			t.Fatalf("line %d still present in corrected map", removed)
		}
	}
}

func TestStaticAnalyzer_KeepsPageImagesWhenDocumentIsImageOnly(t *testing.T) {
	// Image-only documents (scanned PDFs) have no text lines. Removing all image
	// lines would leave an empty document; the guard should preserve them.
	var lines []string
	for i := 1; i <= 40; i++ {
		lines = append(lines, fmt.Sprintf(
			"%d\t%d\timage\tunknown-font\t12\t[0,0,595.44,842.4]\tstdGk_3020436_images/imageFile%d.png",
			i, i, i,
		))
	}
	body := strings.Join(lines, "\n")

	out, err := analyzeStaticStructure([]byte(body), nil)
	if err != nil {
		t.Fatalf("analyzeStaticStructure: %v", err)
	}
	if got := len(out.Lines); got != 40 {
		t.Fatalf("len(Lines)=%d, want 40 (images must not be removed from image-only documents)", got)
	}
}

func TestStaticAnalyzer_RemovesWeBoosWatermarkLines(t *testing.T) {
	body := strings.Join([]string{
		"7\t1\tparagraph\tSimSun\t89\t[0,389.462,623.999,478.248]\twww.weboos.com",
		"8\t1\tparagraph\tF\t12\t[0,0,1,1]\t1 Scope",
		"26\t2\tparagraph\tSimSun\t89\t[0,389.462,623.999,478.248]\twww.weboos.com",
		"27\t2\tparagraph\tF\t12\t[0,0,1,1]\t1.1 Purpose",
		"35\t3\tparagraph\tSimSun\t89\t[0,389.462,623.999,478.248]\twww.weboos.com",
		"36\t3\tparagraph\tF\t12\t[0,0,1,1]\t2 Terms",
		"45\t4\tparagraph\tSimSun\t89\t[0,389.462,623.999,478.248]\twww.weboos.com",
		"272\t10\tparagraph\tSimSun\t89\t[0,389.462,623.999,478.248]\twww om",
		"683\t24\ttable-row\tunknown-font\t12\t[99.42,376.32,541.08,422.82]\t|注2 1高毒可燃气体按有毒气体检测<br>2 特定有毒气体指有相应传感器或气体检测管的有毒气体<br>3 符合本规范技术要求的其他类型直读式仪器也可以用于检测<br>www.weboos.com|||",
		"684\t24\tparagraph\tSimSun\t89\t[0,389.462,623.999,478.248]\twww m",
		"46\t4\tparagraph\tF\t12\t[0,0,1,1]\t2.1 Definitions",
	}, "\n")

	out, err := analyzeStaticStructure([]byte(body), nil)
	if err != nil {
		t.Fatalf("analyzeStaticStructure: %v", err)
	}
	if got := len(out.Lines); got != 5 {
		t.Fatalf("len(Lines)=%d, want 5 after removing watermark lines", got)
	}
	for _, removed := range []int{7, 26, 35, 45, 272, 684} {
		if _, ok := out.CorrectedType[removed]; ok {
			t.Fatalf("line %d still present in corrected map", removed)
		}
	}
	if got := out.CorrectedType[8]; got != "heading-1" {
		t.Fatalf("line8=%q, want heading-1", got)
	}
	if got := out.CorrectedType[27]; got != "heading-2" {
		t.Fatalf("line27=%q, want heading-2", got)
	}
	if got := out.CorrectedType[36]; got != "heading-1" {
		t.Fatalf("line36=%q, want heading-1", got)
	}
	if got := out.CorrectedType[46]; got != "heading-2" {
		t.Fatalf("line46=%q, want heading-2", got)
	}
	var line683 staticInputLine
	found683 := false
	for _, line := range out.Lines {
		if line.LineNo != 683 {
			continue
		}
		line683 = line
		found683 = true
		break
	}
	if !found683 {
		t.Fatalf("line 683 missing from output")
	}
	if got := line683.Content; got != "|注2 1高毒可燃气体按有毒气体检测<br>2 特定有毒气体指有相应传感器或气体检测管的有毒气体<br>3 符合本规范技术要求的其他类型直读式仪器也可以用于检测<br>www.weboos.com|||" {
		t.Fatalf("line683 content=%q, want embedded watermark preserved per spec result", got)
	}
}

func TestStaticAnalyzer_KeepsEmbeddedWeBoosWatermarkWhenRuleTriggers(t *testing.T) {
	body := strings.Join([]string{
		"7\t1\tparagraph\tSimSun\t89\t[0,389.462,623.999,478.248]\twww.weboos.com",
		"26\t2\tparagraph\tSimSun\t89\t[0,389.462,623.999,478.248]\twww.weboos.com",
		"35\t3\tparagraph\tSimSun\t89\t[0,389.462,623.999,478.248]\twww.weboos.com",
		"77\t4\ttable-row\tF\t12\t[0,0,1,1]\talpha www.weboos.com omega",
	}, "\n")

	out, err := analyzeStaticStructure([]byte(body), nil)
	if err != nil {
		t.Fatalf("analyzeStaticStructure: %v", err)
	}
	if got := len(out.Lines); got != 1 {
		t.Fatalf("len(Lines)=%d, want 1 after removing watermark paragraphs", got)
	}
	if got := out.Lines[0].Content; got != "alpha www.weboos.com omega" {
		t.Fatalf("line77 content=%q, want embedded watermark preserved", got)
	}
}

func TestStaticAnalyzer_KeepsWeBoosWatermarkLinesWhenPageHasMultipleMatches(t *testing.T) {
	body := strings.Join([]string{
		"7\t1\tparagraph\tSimSun\t89\t[0,389.462,623.999,478.248]\twww.weboos.com",
		"8\t1\tparagraph\tF\t12\t[0,0,1,1]\t1 Scope",
		"26\t2\tparagraph\tSimSun\t89\t[0,389.462,623.999,478.248]\twww.weboos.com",
		"27\t2\tparagraph\tSimSun\t89\t[0,389.462,623.999,478.248]\twww m",
		"35\t3\tparagraph\tSimSun\t89\t[0,389.462,623.999,478.248]\twww.weboos.com",
		"36\t3\tparagraph\tF\t12\t[0,0,1,1]\t2 Terms",
		"45\t4\tparagraph\tSimSun\t89\t[0,389.462,623.999,478.248]\twww.weboos.com",
	}, "\n")

	out, err := analyzeStaticStructure([]byte(body), nil)
	if err != nil {
		t.Fatalf("analyzeStaticStructure: %v", err)
	}
	if got := len(out.Lines); got != 7 {
		t.Fatalf("len(Lines)=%d, want 7 when a page has multiple watermark matches", got)
	}
	for _, kept := range []int{7, 26, 27, 35, 45} {
		if _, ok := out.CorrectedType[kept]; !ok {
			t.Fatalf("line %d unexpectedly removed", kept)
		}
	}
}

func TestStaticAnalyzer_MergesChineseStyleParagraphLines(t *testing.T) {
	body := strings.Join([]string{
		"74\t5\tparagraph\tHiddenHorzOCR\t9\t[104.88,496.943,339.001,508.549]\t职业健康监护 occupational health surveillance",
		"75\t5\tparagraph\tHiddenHorzOCR\t9\t[83.28,451.33,546.241,493.45]\t以预防为目的，根据职业危害因素对消防员健康的损害或影响，采取综合措施，保护",
		"77\t5\tparagraph\tHiddenHorzOCR\t9\t[83.76,436.27,297.121,447.37]\t消防员健康。",
		"78\t5\tparagraph\tHiddenHorzOCR\t9\t[105.36,390.37,546.721,401.77]\t用于消除或者减少职业危害因素对消防员健康的损害或影响，达到保护消防员健康目的的装备，主",
		"79\t5\tparagraph\tHiddenHorzOCR\t9\t[84,375.3,297.121,386.4]\t要包括侦检装备、个人防护装备、洗消装备等。",
		"80\t5\theading-2\tTimes-Roman\t11\t[83.86,360.006,99.898,372.058]\t3.9",
	}, "\n")

	out, err := analyzeStaticStructure([]byte(body), nil)
	if err != nil {
		t.Fatalf("analyzeStaticStructure: %v", err)
	}
	if got := len(out.Lines); got != 4 {
		t.Fatalf("len(Lines)=%d, want 4 after Chinese merge", got)
	}
	wantByLine := map[int]string{
		75: "以预防为目的，根据职业危害因素对消防员健康的损害或影响，采取综合措施，保护消防员健康。",
		78: "用于消除或者减少职业危害因素对消防员健康的损害或影响，达到保护消防员健康目的的装备，主要包括侦检装备、个人防护装备、洗消装备等。",
	}
	gotByLine := make(map[int]string, len(out.Lines))
	for _, line := range out.Lines {
		gotByLine[line.LineNo] = line.Content
	}
	for lineNo, want := range wantByLine {
		if got := gotByLine[lineNo]; got != want {
			t.Fatalf("line %d content=%q, want %q", lineNo, got, want)
		}
	}
	var merged staticInputLine
	found := false
	for _, line := range out.Lines {
		if line.LineNo != 78 {
			continue
		}
		merged = line
		found = true
		break
	}
	if !found {
		t.Fatalf("merged line 78 missing")
	}
	want := "用于消除或者减少职业危害因素对消防员健康的损害或影响，达到保护消防员健康目的的装备，主要包括侦检装备、个人防护装备、洗消装备等。"
	if merged.Content != want {
		t.Fatalf("merged content=%q, want %q", merged.Content, want)
	}
	for _, removed := range []int{77, 79} {
		for _, line := range out.Lines {
			if line.LineNo == removed {
				t.Fatalf("line %d should have been merged away", removed)
			}
		}
	}
}

func TestStaticAnalyzer_MergesEnglishStyleParagraphLines(t *testing.T) {
	body := strings.Join([]string{
		"1\t1\tparagraph\tTimes-Roman\t10\t[72,500,520,512]\tThis standard applies to buildings designed for rapid response and",
		"2\t1\tparagraph\tTimes-Roman\t10\t[72,486,519,498]\tcontinuous operational readiness under adverse field conditions,",
		"3\t1\tparagraph\tTimes-Roman\t10\t[72,472,250,484]\twith short final wrapping.",
		"4\t1\tparagraph\tTimes-Roman\t10\t[72,458,521,470]\tA separate paragraph begins here and should stay independent because",
		"5\t1\tparagraph\tTimes-Roman\t10\t[72,444,260,456]\tit ends on the next short line.",
	}, "\n")

	out, err := analyzeStaticStructure([]byte(body), nil)
	if err != nil {
		t.Fatalf("analyzeStaticStructure: %v", err)
	}
	if got := len(out.Lines); got != 2 {
		t.Fatalf("len(Lines)=%d, want 2 after English merge", got)
	}
	gotContent := map[int]string{}
	for _, line := range out.Lines {
		gotContent[line.LineNo] = line.Content
	}
	if got := gotContent[1]; got != "This standard applies to buildings designed for rapid response andcontinuous operational readiness under adverse field conditions,with short final wrapping." {
		t.Fatalf("line1 content=%q", got)
	}
	if got := gotContent[4]; got != "A separate paragraph begins here and should stay independent becauseit ends on the next short line." {
		t.Fatalf("line4 content=%q", got)
	}
}

func TestStaticAnalyzer_HandleEvent_WritesMergeOnlyOverrideOutput(t *testing.T) {
	tmp := t.TempDir()
	recordID := int64(9012)
	lineFile := filepath.Join(tmp, "ocr_rslt_9012_opendata.txt")
	originPath := filepath.Join(tmp, "ocr_rslt_9012_opendata.origin")
	body := strings.Join([]string{
		"1\t1\tparagraph\tTimes-Roman\t10\t[72,500,520,512]\tThis standard applies to buildings designed for rapid response and",
		"2\t1\tparagraph\tTimes-Roman\t10\t[72,486,519,498]\tcontinuous operational readiness under adverse field conditions,",
		"3\t1\tparagraph\tTimes-Roman\t10\t[72,472,250,484]\twith short final wrapping.",
	}, "\n")
	if err := os.WriteFile(lineFile, []byte(body), 0o644); err != nil {
		t.Fatalf("write line file: %v", err)
	}
	seedOrigin := "seed origin should remain untouched"
	if err := os.WriteFile(originPath, []byte(seedOrigin), 0o444); err != nil {
		t.Fatalf("write origin file: %v", err)
	}

	t.Setenv("ARTIFACT_DIR", tmp)
	store := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              recordID,
		ParserName:      "opendata",
		ResultFilename:  filepath.Join(tmp, "ocr_rslt_9012.json"),
		StagingFilename: filepath.Join(tmp, "ocr_rslt_9012.pdf"),
		StatusRaw:       "[]",
	}}
	logger := &fakeLogger{}
	p := NewStaticAnalyzerProcessor(store, nil, logger)

	if err := p.HandleEvent(context.Background(), []byte(`{"record_id":"9012"}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}

	if _, ok := findInfoLog(logger.infos, "static analyzer invoked"); !ok {
		t.Fatalf("expected invocation log, got %#v", logger.infos)
	}
	if originLog, ok := findInfoLog(logger.infos, "origin backup written"); ok {
		t.Fatalf("did not expect origin backup log, got %#v", originLog)
	}
	txtLog, ok := findInfoLog(logger.infos, "static analyzer output written")
	if !ok {
		t.Fatalf("expected output write log, got %#v", logger.infos)
	}
	if got, ok := logValue(txtLog.args, "output_path"); !ok || got != lineFile {
		t.Fatalf("output_path=%v, ok=%v", got, ok)
	}

	gotOrigin, err := os.ReadFile(originPath)
	if err != nil {
		t.Fatalf("read origin file: %v", err)
	}
	if string(gotOrigin) != seedOrigin {
		t.Fatalf("origin content changed: got %q want %q", string(gotOrigin), seedOrigin)
	}
	bs, err := os.ReadFile(lineFile)
	if err != nil {
		t.Fatalf("read rewritten file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(bs)), "\n")
	if len(lines) != 1 {
		t.Fatalf("rewritten lines=%d, want 1 merged line", len(lines))
	}
	if !strings.Contains(lines[0], "This standard applies to buildings designed for rapid response andcontinuous operational readiness under adverse field conditions,with short final wrapping.") {
		t.Fatalf("rewritten content=%q", lines[0])
	}
}

func TestStaticAnalyzer_WriteCorrectedArtifact_LeavesExistingOriginUntouched(t *testing.T) {
	tmp := t.TempDir()
	inputPath := filepath.Join(tmp, "ocr_rslt_93_opendata.txt")
	originPath := filepath.Join(tmp, "ocr_rslt_93_opendata.origin")

	currentInputBody := strings.Join([]string{
		"1\t1\tparagraph\tF\t12\t[0,0,1,1]\tCurrent OCR line 1",
		"7\t1\tparagraph\tF\t12\t[0,0,1,1]\tCurrent OCR line 7",
		"2\t1\tparagraph\tF\t12\t[0,0,1,1]\tCurrent OCR line 2",
	}, "\n")
	staleOriginBody := strings.Join([]string{
		"1\t1\tparagraph\tF\t12\t[0,0,1,1]\tStale origin line 1",
		"2\t1\tparagraph\tF\t12\t[0,0,1,1]\tStale origin line 2",
	}, "\n")
	if err := os.WriteFile(inputPath, []byte(currentInputBody), 0o644); err != nil {
		t.Fatalf("write input file: %v", err)
	}
	if err := os.WriteFile(originPath, []byte(staleOriginBody), 0o644); err != nil {
		t.Fatalf("write origin file: %v", err)
	}

	logger := &fakeLogger{}
	p := &StaticAnalyzerProcessor{OverrideOrigin: true, Logger: logger}
	out := staticAnalyzeResult{
		Lines: []staticInputLine{
			{
				LineNo:            2,
				PageNo:            1,
				OriginalLineType:  "paragraph",
				OriginalLineLower: "paragraph",
				Font:              "F",
				FontSize:          "12",
				Coordinate:        "[0,0,1,1]",
				Content:           "Changed line 2",
			},
		},
		CorrectedType: map[int]string{2: "heading-1"},
		OutputChanged: true,
	}

	if err := p.writeCorrectedArtifact(93, filepath.Base(inputPath), inputPath, out); err != nil {
		t.Fatalf("writeCorrectedArtifact: %v", err)
	}

	gotOrigin, err := os.ReadFile(originPath)
	if err != nil {
		t.Fatalf("read origin file: %v", err)
	}
	if string(gotOrigin) != staleOriginBody {
		t.Fatalf("origin content=\n%s\nwant unchanged stale origin=\n%s", string(gotOrigin), staleOriginBody)
	}
	if logEntry, ok := findInfoLog(logger.infos, "origin backup written"); ok {
		t.Fatalf("did not expect origin backup written log, got %#v", logEntry)
	}
	if _, ok := findInfoLog(logger.infos, "static analyzer output written"); !ok {
		t.Fatalf("expected output write log, got %#v", logger.infos)
	}
}

func TestStaticAnalyzer_WriteCorrectedArtifact_RemovesStaleChunksBesideInputFile(t *testing.T) {
	tmp := t.TempDir()
	inputPath := filepath.Join(tmp, "ocr_rslt_166_opendata.txt")
	chunkPath := filepath.Join(tmp, "std_33830_opendata.chunks")
	keepPath := filepath.Join(tmp, "std_33830_opendata.topics")

	if err := os.WriteFile(inputPath, []byte("1\t1\tparagraph\tF\t12\t[0,0,1,1]\tCurrent line\n"), 0o644); err != nil {
		t.Fatalf("write input file: %v", err)
	}
	if err := os.WriteFile(chunkPath, []byte("overlap: []\nlines: [1]\n"), 0o644); err != nil {
		t.Fatalf("write chunk file: %v", err)
	}
	if err := os.WriteFile(keepPath, []byte("keep"), 0o644); err != nil {
		t.Fatalf("write keep file: %v", err)
	}

	logger := &fakeLogger{}
	p := &StaticAnalyzerProcessor{
		OverrideOrigin: true,
		Logger:         logger,
	}
	out := staticAnalyzeResult{
		Lines: []staticInputLine{
			{
				LineNo:           1,
				PageNo:           1,
				OriginalLineType: "heading-1",
				Font:             "F",
				FontSize:         "12",
				Coordinate:       "[0,0,1,1]",
				Content:          "Updated line",
			},
		},
		CorrectedType: map[int]string{1: "unchanged"},
		OutputChanged: true,
	}

	if err := p.writeCorrectedArtifact(166, filepath.Base(inputPath), inputPath, out); err != nil {
		t.Fatalf("writeCorrectedArtifact: %v", err)
	}

	if _, err := os.Stat(chunkPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("chunk file exists=%v, want removed", err == nil)
	}
	if _, err := os.Stat(keepPath); err != nil {
		t.Fatalf("keep file stat: %v", err)
	}
	if _, ok := findInfoLog(logger.infos, "removed stale chunk artifact"); !ok {
		t.Fatalf("expected stale chunk removal log, got %#v", logger.infos)
	}
}

func TestApplyStaticTruncateDots(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "exactly 6 dots truncated to 5",
			input: `\dots \dots \dots \dots \dots \dots`,
			want:  `\dots \dots \dots \dots \dots`,
		},
		{
			name:  "long sequence from ADR example",
			input: `\eta \dots \dots \dots \dots \dots \dots \dots \dots \dots \dots \dots \dots`,
			want:  `\eta \dots \dots \dots \dots \dots`,
		},
		{
			name:  "exactly 5 dots unchanged",
			input: `\dots \dots \dots \dots \dots`,
			want:  `\dots \dots \dots \dots \dots`,
		},
		{
			name:  "4 dots unchanged",
			input: `\dots \dots \dots \dots`,
			want:  `\dots \dots \dots \dots`,
		},
		{
			name:  "no dots unchanged",
			input: `some text`,
			want:  `some text`,
		},
		{
			name:  "dots without spaces",
			input: `\dots\dots\dots\dots\dots\dots`,
			want:  `\dots \dots \dots \dots \dots`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lines := []staticInputLine{
				{LineNo: 1, PageNo: 1, OriginalLineType: "equation", Font: "F", FontSize: "12", Coordinate: "[0,0,1,1]", Content: tc.input},
			}
			result := applyStaticTruncateDots(lines, nil)
			if got := result[0].Content; got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStaticTOCDetector_SkipsPrimaryDuringProviderCooldown(t *testing.T) {
	t.Setenv("DETECT_TOC_PROVIDER_COOLDOWN_SEC", "3600")

	extractor := &fakeTOCStructuredExtractor{
		results: map[string]map[string]any{
			"fallback-model": {"toc_line_numbers": []any{4, 5}},
		},
		errors: map[string]error{
			"primary-model": &llmclients.StructuredOutputError{
				Kind: llmclients.ErrStructuredOutputProvider,
				Err:  errors.New(`(MID_26050141) openai request failed with status 402: {"error":{"message":"Insufficient Balance","code":"invalid_request_error"}}`),
			},
		},
	}
	logger := &fakeLogger{}
	detector := &staticTOCDetector{
		Client:            extractor,
		PromptText:        "detect toc",
		ModelName:         "primary-model",
		FallbackModelName: "fallback-model",
		Logger:            logger,
	}

	got, usedModel, err := detector.detectTOCLines(context.Background(), `[{"line_number":4}]`)
	if err != nil {
		t.Fatalf("first detectTOCLines error = %v", err)
	}
	if usedModel != "fallback-model" {
		t.Fatalf("first usedModel=%q, want fallback-model", usedModel)
	}
	if len(got) != 2 || got[0] != 4 || got[1] != 5 {
		t.Fatalf("first toc lines=%v, want [4 5]", got)
	}

	got, usedModel, err = detector.detectTOCLines(context.Background(), `[{"line_number":4}]`)
	if err != nil {
		t.Fatalf("second detectTOCLines error = %v", err)
	}
	if usedModel != "fallback-model" {
		t.Fatalf("second usedModel=%q, want fallback-model", usedModel)
	}
	if extractor.callByModel["primary-model"] != 1 {
		t.Fatalf("primary-model calls=%d, want 1", extractor.callByModel["primary-model"])
	}
	if extractor.callByModel["fallback-model"] != 2 {
		t.Fatalf("fallback-model calls=%d, want 2", extractor.callByModel["fallback-model"])
	}

	foundSkipLog := false
	for _, entry := range logger.infos {
		if entry.message == "skipping primary TOC LLM call during provider cooldown" {
			foundSkipLog = true
			break
		}
	}
	if !foundSkipLog {
		t.Fatalf("expected cooldown skip log, got infos=%+v", logger.infos)
	}
}
