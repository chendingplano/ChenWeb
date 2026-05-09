package docprocessing

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
	store := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              recordID,
		ParserName:      "opendata",
		ResultFilename:  filepath.Join(tmp, "ocr_rslt_9001.json"),
		StagingFilename: filepath.Join(tmp, "ocr_rslt_9001.pdf"),
		StatusRaw:       "[]",
	}}
	p := NewStaticAnalyzerProcessor(store, nil)

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
	if got := strings.TrimSpace(asString(row["operation"])); got != "static_analyzer" {
		t.Fatalf("operation=%q, want static_analyzer", got)
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
	if gotByLine["2"] != "num-list-item" {
		t.Fatalf("line2 corrected=%q, want num-list-item", gotByLine["2"])
	}
	if gotByLine["3"] != "toc" || gotByLine["4"] != "toc" || gotByLine["5"] != "toc" {
		t.Fatalf("toc lines got: 3=%q 4=%q 5=%q, want all toc", gotByLine["3"], gotByLine["4"], gotByLine["5"])
	}
	if gotByLine["6"] != "s-sym-list-item" {
		t.Fatalf("line6 corrected=%q, want s-sym-list-item", gotByLine["6"])
	}
	if gotByLine["7"] != "m-sym-list-item" {
		t.Fatalf("line7 corrected=%q, want m-sym-list-item", gotByLine["7"])
	}
}

func TestStaticAnalyzer_MissingArtifactDirFailsFast(t *testing.T) {
	t.Setenv("ARTIFACT_DIR", "")
	store := &fakeDocMetadataStore{}
	p := NewStaticAnalyzerProcessor(store, nil)
	err := p.HandleEvent(context.Background(), []byte(`{"record_id":"1"}`))
	if err == nil || !strings.Contains(err.Error(), "missing ARTIFACT_DIR") {
		t.Fatalf("err=%v, want missing ARTIFACT_DIR", err)
	}
}

func TestStaticAnalyzer_StatusReplacesExistingEntry(t *testing.T) {
	start := time.Date(2026, 4, 23, 10, 0, 0, 0, time.UTC)
	raw := `[{"operation":"static-analyzer","proc_status":"failed"},{"operation":"chunking","proc_status":"success"}]`
	got, err := appendStaticAnalyzerStatus(raw, staticStatusParams{
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
	if strings.TrimSpace(asString(arr[0]["operation"])) != "static_analyzer" {
		t.Fatalf("first operation=%q, want static_analyzer", asString(arr[0]["operation"]))
	}
	if strings.TrimSpace(asString(arr[0]["proc_status"])) != "success" {
		t.Fatalf("first proc_status=%q, want success", asString(arr[0]["proc_status"]))
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
	if got := out.CorrectedType[84]; got != "unchanged" {
		t.Fatalf("line84=%q, want unchanged", got)
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
