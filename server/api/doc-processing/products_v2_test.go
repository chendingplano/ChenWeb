package docprocessing

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

const productV2Example = `{"v":2,"p":[` +
	`{"n":"垃圾桶","t":"eq","r":"use","d":"规定垃圾桶的清洁要求","s":[143,"150-151"],"o":"m","x":"垃圾桶应保持清洁。","cnd":["使用期间"],"exc":["临时容器"],"p":[{"n":"容量","v":"240","u":"L"},{"n":"数量","v":2}],"a":"村保洁员","q":0.95,"cr":"正文明确规定"},` +
	`{"n":"收集箱","c":"分类收集箱","t":"pc","r":"has","d":"收集箱包含内胆","s":["116#r2"],"o":"d","rp":[{"n":"内胆","r":"has"}],"q":0}]}`

// productV2Lines is the input those example spans resolve against, in the form
// a merged call sends it: table line 116 rendered as labeled rows.
var productV2Lines = productSourceLines{
	116: "116#h0: 名称 | 说明\n116#r1: 垃圾桶 | 240L\n116#r2: 收集箱 | 含内胆",
	143: "垃圾桶应保持清洁。",
	150: "第一行",
	151: "第二行",
}

func decodeProductV2Test(t *testing.T, wire string, lines productSourceLines) []map[string]any {
	t.Helper()
	var raw map[string]any
	if err := json.Unmarshal([]byte(wire), &raw); err != nil {
		t.Fatal(err)
	}
	got, err := decodeProductMergedV2(raw, lines)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	items := got["products"].([]any)
	rows := make([]map[string]any, 0, len(items))
	for _, item := range items {
		rows = append(rows, item.(map[string]any))
	}
	return rows
}

func TestProductsV2AcceptsEmptyExtraction(t *testing.T) {
	if rows := decodeProductV2Test(t, `{"v":2,"p":[]}`, productV2Lines); len(rows) != 0 {
		t.Fatalf("expected successful empty extraction, got %#v", rows)
	}
}

func TestProductsV2DecodesFieldsAndDefaults(t *testing.T) {
	rows := decodeProductV2Test(t, productV2Example, productV2Lines)
	if len(rows) != 2 {
		t.Fatalf("rows=%d, want 2", len(rows))
	}
	want := []map[string]any{
		{
			"product_name": "垃圾桶", "canonical_name": "垃圾桶", "product_type": "equipment",
			"relation_type": "usage_condition", "relation_code": "use", "product_summary": "规定垃圾桶的清洁要求",
			"evidence_lines": []string{"143", "150-151"}, "evidence_quote": "垃圾桶应保持清洁。\n第一行\n第二行",
			"confidence": 0.95, "confidence_reason": "正文明确规定",
			"relation_details": map[string]any{
				"obligation_level": "mandatory", "requirement_text": "垃圾桶应保持清洁。",
				"conditions": []string{"使用期间"}, "exceptions": []string{"临时容器"},
				"thresholds_or_parameters": []any{
					map[string]any{"name": "容量", "value": "240", "unit": "L"},
					map[string]any{"name": "数量", "value": "2", "unit": ""},
				},
				"related_products":  []any{},
				"responsible_actor": "村保洁员",
			},
		},
		{
			"product_name": "收集箱", "canonical_name": "分类收集箱", "product_type": "product_class",
			"relation_type": "contains_product", "relation_code": "has", "product_summary": "收集箱包含内胆",
			"evidence_lines": []string{"116#r2"}, "evidence_quote": "收集箱 | 含内胆",
			"confidence": 0.0, "confidence_reason": "",
			"relation_details": map[string]any{
				"obligation_level": "descriptive", "requirement_text": "",
				"conditions": []string{}, "exceptions": []string{},
				"thresholds_or_parameters": []any{},
				"related_products":         []any{map[string]any{"product_name": "内胆", "relationship": "contains_product"}},
				"responsible_actor":        "",
			},
		},
	}
	for i := range want {
		if !reflect.DeepEqual(rows[i], want[i]) {
			t.Errorf("row %d\n got %#v\nwant %#v", i, rows[i], want[i])
		}
	}

	// The expanded rows feed the unchanged legacy normalizer.
	normalized := normalizeProductList([]any{rows[0], rows[1]})
	if normalized[0]["relation_summary"] != "规定垃圾桶的清洁要求" || normalized[0]["requirement_text"] != "垃圾桶应保持清洁。" ||
		normalized[0]["obligation_level"] != "mandatory" || normalized[1]["canonical_name"] != "分类收集箱" {
		t.Fatalf("normalizeProductList lost decoded fields: %#v", normalized)
	}
}

func TestProductsV2RebuildsEvidenceInSourceOrder(t *testing.T) {
	for _, tc := range []struct {
		spans string
		want  string
	}{
		{`["116#r2",143,"116#r1"]`, "垃圾桶 | 240L\n收集箱 | 含内胆\n垃圾桶应保持清洁。"},
		{`[116,"116#r1"]`, "名称 | 说明\n垃圾桶 | 240L\n收集箱 | 含内胆"},
		{`[143,143,"143-143"]`, "垃圾桶应保持清洁。"},
	} {
		wire := `{"v":2,"p":[{"n":"垃圾桶","t":"eq","r":"sc","d":"摘要","s":` + tc.spans + `,"o":"d","q":0.9}]}`
		row := decodeProductV2Test(t, wire, productV2Lines)[0]
		if row["evidence_quote"] != tc.want {
			t.Errorf("spans %s: evidence_quote=%q, want %q", tc.spans, row["evidence_quote"], tc.want)
		}
	}
}

func TestProductsV2RangeMayCrossImageLines(t *testing.T) {
	lines := productSourceLinesFromChunk([]MarkedLine{
		{Mark: "r", Line: Line{LineNo: 10, LineType: "paragraph", Content: "第一段"}},
		{Mark: "r", Line: Line{LineNo: 11, LineType: "image", Content: "img.png"}},
		{Mark: "o", Line: Line{LineNo: 12, LineType: "paragraph", Content: "第三段"}},
	})
	row := decodeProductV2Test(t, `{"v":2,"p":[{"n":"垃圾桶","t":"eq","r":"sc","d":"摘要","s":["10-12"],"o":"d","q":0.9}]}`, lines)[0]
	if row["evidence_quote"] != "第一段\n第三段" {
		t.Fatalf("evidence_quote=%q", row["evidence_quote"])
	}
	var raw map[string]any
	_ = json.Unmarshal([]byte(`{"v":2,"p":[{"n":"垃圾桶","t":"eq","r":"sc","d":"摘要","s":[11],"o":"d","q":0.9}]}`), &raw)
	if _, err := decodeProductMergedV2(raw, lines); err == nil {
		t.Fatal("a span on an image line, which is never sent, must be rejected")
	}
}

func TestProductsV2ResolvesRowsOfHTMLTables(t *testing.T) {
	lines := productSourceLinesFromChunk([]MarkedLine{{Mark: "r", Line: Line{
		LineNo: 116, LineType: "table",
		Content: "<table><tr><th>名称</th><th>容量</th></tr><tr><td>垃圾桶</td><td>240L</td></tr></table>",
	}}})
	row := decodeProductV2Test(t, `{"v":2,"p":[{"n":"垃圾桶","t":"eq","r":"sc","d":"摘要","s":["116#r1"],"o":"d","q":0.9}]}`, lines)[0]
	if row["evidence_quote"] != "垃圾桶 | 240L" {
		t.Fatalf("evidence_quote=%q, want the row's rendered cells", row["evidence_quote"])
	}
}

func TestProductsV2RejectsInvalidWireRecords(t *testing.T) {
	rowMutations := []func(map[string]any){
		func(r map[string]any) { delete(r, "s") },
		func(r map[string]any) { r["s"] = []any{} },
		func(r map[string]any) { r["s"] = nil },
		func(r map[string]any) { r["s"] = []any{143, "invalid"} },
		func(r map[string]any) { r["s"] = []any{"151-150"} },
		func(r map[string]any) { r["s"] = []any{142.5} },
		func(r map[string]any) { r["s"] = []any{0} },
		func(r map[string]any) { r["s"] = []any{999} },
		func(r map[string]any) { r["s"] = []any{"150-999"} },
		func(r map[string]any) { r["s"] = []any{"143#r1"} },
		func(r map[string]any) { r["s"] = []any{"116#r9"} },
		func(r map[string]any) { delete(r, "t") },
		func(r map[string]any) { r["t"] = "unk" },
		func(r map[string]any) { r["t"] = "equipment" },
		func(r map[string]any) { r["r"] = " " },
		func(r map[string]any) { delete(r, "o") },
		func(r map[string]any) { r["o"] = "x" },
		func(r map[string]any) { delete(r, "d") },
		func(r map[string]any) { r["d"] = "" },
		func(r map[string]any) { r["d"] = "  " },
		func(r map[string]any) { r["n"] = " " },
		func(r map[string]any) { delete(r, "q") },
		func(r map[string]any) { r["q"] = -0.1 },
		func(r map[string]any) { r["q"] = 1.1 },
		func(r map[string]any) { r["q"] = "0.9" },
		func(r map[string]any) { r["e"] = "quote" },
		func(r map[string]any) { r["evidence_quote"] = "quote" },
		func(r map[string]any) { r["product_summary_en"] = "translation" },
		func(r map[string]any) { r["relation_details"] = map[string]any{} },
		func(r map[string]any) { r["cnd"] = "使用期间" },
		func(r map[string]any) { r["p"] = []any{map[string]any{"n": "容量"}} },
		func(r map[string]any) { r["p"] = []any{map[string]any{"n": "容量", "v": "240", "unit": "L"}} },
		func(r map[string]any) { r["rp"] = []any{map[string]any{"n": "内胆", "r": "cmp"}} },
		func(r map[string]any) { r["rp"] = []any{map[string]any{"r": "has"}} },
	}
	topMutations := []func(map[string]any){
		func(p map[string]any) { delete(p, "v") },
		func(p map[string]any) { p["v"] = 1 },
		func(p map[string]any) { delete(p, "p") },
		func(p map[string]any) { p["p"] = nil },
		func(p map[string]any) { p["products"] = []any{} },
	}
	check := func(i int, raw map[string]any) {
		t.Helper()
		got, err := decodeProductMergedV2(raw, productV2Lines)
		if err == nil || got != nil {
			t.Errorf("mutation %d accepted: %#v", i, raw)
		}
	}
	for i, mutate := range rowMutations {
		var raw map[string]any
		_ = json.Unmarshal([]byte(productV2Example), &raw)
		mutate(raw["p"].([]any)[0].(map[string]any))
		check(i, raw)
	}
	for i, mutate := range topMutations {
		var raw map[string]any
		_ = json.Unmarshal([]byte(productV2Example), &raw)
		mutate(raw)
		check(100+i, raw)
	}
}

// Prod record 382, chunk 14 (ChenWeb#28): line 264 is a figure legend paragraph, and the
// primary cited its numbered items as rows 264#r2 to 264#r10 in nine products. The check
// error is the primary's retry text, so it must name every invalid span, not only the first.
func TestProductsV2NamesEveryInvalidRowSpan(t *testing.T) {
	response, lines := loadProductsRecord382Chunk14(t)
	var payload map[string]any
	if err := json.Unmarshal([]byte(response), &payload); err != nil {
		t.Fatal(err)
	}
	_, err := decodeProductMergedV2(payload, lines)
	if err == nil {
		t.Fatal("decode accepted rows cited on a line without table rows")
	}
	for r := 2; r <= 10; r++ {
		if span := fmt.Sprintf(`"264#r%d"`, r); !strings.Contains(err.Error(), span) {
			t.Errorf("error does not name %s: %v", span, err)
		}
	}
	if !strings.Contains(err.Error(), "line 264 has no table rows, cite it as 264") {
		t.Errorf("error does not say how to cite line 264: %v", err)
	}
}

func TestProductsV2RetryNamesContinuationTableLabels(t *testing.T) {
	// Runshen alerts 21662 and 21663 cited the first continuation row as r0.
	lines := productSourceLines{
		386: "386#h0: 10 |  | 卷筒 | 运转正常,无异常声响\n386#r1: 11 | 回转机构 | 开式齿轮 | 啮合良好;润滑良好",
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(`{"v":2,"p":[{"n":"卷筒","t":"cmp","r":"perf","d":"要求卷筒运转正常、无异常声响。","s":["386#r0"],"o":"m","q":0.9}]}`), &payload); err != nil {
		t.Fatal(err)
	}
	_, err := decodeProductMergedV2(payload, lines)
	if err == nil {
		t.Fatal("decode accepted a row label absent from the input")
	}
	for _, label := range []string{"386#r0", "386#h0", "386#r1", "ChenWeb#51"} {
		if !strings.Contains(err.Error(), label) {
			t.Errorf("retry text does not name %s: %v", label, err)
		}
	}
	payload["p"].([]any)[0].(map[string]any)["s"] = []any{"386#h0"}
	corrected, err := decodeProductMergedV2(payload, lines)
	if err != nil {
		t.Fatal(err)
	}
	if got := corrected["products"].([]any)[0].(map[string]any)["evidence_quote"]; got != "10 |  | 卷筒 | 运转正常,无异常声响" {
		t.Errorf("decoded evidence = %q", got)
	}
}

func TestProductsV2TreatsPackagingRequirementsAsHandling(t *testing.T) {
	lines := productSourceLines{123: "机组应外套塑料罩或防潮纸并应固定在包装箱内，其包装应符合GB/T13384的规定。"}
	rows := decodeProductV2Test(t, `{"v":2,"p":[
		{"n":"二氧化碳空气源热泵机组","t":"eq","r":"pkg","d":"机组应按规定包装。","s":[123],"o":"m","q":0.9},
		{"n":"包装箱","t":"pkg","r":"use","d":"包装箱用于固定机组。","s":[123],"o":"m","q":0.9}
	]}`, lines)
	if rows[0]["relation_code"] != "stor" || rows[0]["relation_type"] != "storage_requirement" {
		t.Fatalf("packaging requirement did not use the existing handling relation: %+v", rows[0])
	}
	if rows[1]["product_type"] != "packaging" || rows[1]["relation_code"] != "use" {
		t.Fatalf("packaging product type changed: %+v", rows[1])
	}
}

// loadProductsRecord382Chunk14 returns the primary's captured response and the lines
// the call sent, keyed by line number as productSourceLinesFromChunk builds them.
func loadProductsRecord382Chunk14(t *testing.T) (string, productSourceLines) {
	t.Helper()
	body, err := os.ReadFile("testdata/products-merged-record382-chunk14.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Response string            `json:"response"`
		Lines    map[string]string `json:"lines"`
	}
	if err := json.Unmarshal(body, &fixture); err != nil {
		t.Fatal(err)
	}
	lines := productSourceLines{}
	for k, v := range fixture.Lines {
		n, err := strconv.Atoi(k)
		if err != nil {
			t.Fatal(err)
		}
		lines[n] = v
	}
	return fixture.Response, lines
}

func TestProductsV2DoesNotMutateCapturedResponse(t *testing.T) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(productV2Example), &raw); err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(raw)
	if _, err := decodeProductMergedV2(raw, productV2Lines); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(raw)
	if string(before) != string(after) {
		t.Fatal("captured wire response was expanded in place")
	}
}

func TestProductsMergedPromptUsesV2(t *testing.T) {
	for ref, want := range map[string]bool{
		"prompt-extract-products-merged-v2.md":              true,
		"/srv/prompts/prompt-extract-products-merged-v2.md": true,
		"prompt-extract-products-merged-v1.md":              false,
		"":                                                  false,
	} {
		if got := productMergedPromptUsesV2(ref); got != want {
			t.Errorf("productMergedPromptUsesV2(%q)=%v, want %v", ref, got, want)
		}
	}
}

func TestBuildCompactMergedProductsTaskPrompt_CarriesNoSchema(t *testing.T) {
	got := buildCompactMergedProductsTaskPrompt(9)
	for _, legacy := range []string{`"products"`, `"relation_type"`, "evidence_quote", "schema"} {
		if strings.Contains(got, legacy) {
			t.Fatalf("compact task prompt repeats the legacy schema (%s):\n%s", legacy, got)
		}
	}
	if !strings.Contains(got, "Block index: 9") {
		t.Fatalf("compact task prompt lost the block index:\n%s", got)
	}
}

func newCompactMergedTestProcessor(extractor LLMJSONExtractor) *ProductsProcessor {
	return &ProductsProcessor{
		Extractor:        extractor,
		Logger:           loggerutil.CreateDefaultLogger("MID_20261107_870"),
		Now:              time.Now,
		MergedPass:       true,
		MergedPromptText: "merged extract",
		MergedPromptRef:  "prompt-extract-products-merged-v2.md",
		MergedModelName:  "test",
		MergedModelCfg:   structureModelConfig{ModelName: "test"},
	}
}

var compactMergedTestChunk = Chunk{SeqNo: 3, Lines: []MarkedLine{
	{Mark: "r", Line: Line{LineNo: 143, PageNo: 1, LineType: "paragraph", Content: "垃圾桶应保持清洁。"}},
}}

const compactMergedTestResponse = `{"v":2,"p":[{"n":"垃圾桶","t":"eq","r":"use","d":"规定垃圾桶的清洁要求","s":[143],"o":"m","q":0.9}]}`

func wireMap(t *testing.T, wire string) map[string]any {
	t.Helper()
	var raw map[string]any
	if err := json.Unmarshal([]byte(wire), &raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestExtractProductsMergedForChunk_DecodesCompactV2(t *testing.T) {
	extractor := &fakeJSONExtractor{out: wireMap(t, compactMergedTestResponse)}
	p := newCompactMergedTestProcessor(extractor)
	block := chunksToBlocks([]Chunk{compactMergedTestChunk})[0]
	rows, _, _, err := p.extractProductsMergedForChunk(context.Background(), "evt", 0, 1, block, compactMergedTestChunk, "doc")
	if err != nil {
		t.Fatal(err)
	}
	if len(extractor.contractNames) != 1 || extractor.contractNames[0] != "chenweb_product_extraction_merged_v2" {
		t.Fatalf("contracts=%v, want the merged v2 contract", extractor.contractNames)
	}
	if len(rows) != 1 || rows[0]["relation_type"] != "usage_condition" || rows[0]["relation_summary"] != "规定垃圾桶的清洁要求" ||
		rows[0]["evidence_quote"] != "垃圾桶应保持清洁。" || !reflect.DeepEqual(rows[0]["evidence_lines"], []string{"143"}) {
		t.Fatalf("rows=%#v", rows)
	}
}

func TestExtractProductsMergedForChunk_CompactRejectsLegacyAndEmptyShortcut(t *testing.T) {
	for name, extractor := range map[string]*fakeJSONExtractor{
		"legacy response": {out: map[string]any{"products": []any{map[string]any{"product_name": "垃圾桶", "evidence_lines": []any{"143"}}}}},
		// isEmptyProductExtractionError turns this message into an empty success
		// for legacy prompts; a compact call must keep it an error.
		"empty-json shortcut": {err: errors.New("unexpected end of JSON input, json:{[]}")},
	} {
		t.Run(name, func(t *testing.T) {
			p := newCompactMergedTestProcessor(extractor)
			block := chunksToBlocks([]Chunk{compactMergedTestChunk})[0]
			rows, _, _, err := p.extractProductsMergedForChunk(context.Background(), "evt", 0, 1, block, compactMergedTestChunk, "doc")
			if err == nil || rows != nil {
				t.Fatalf("rows=%#v err=%v; a compact call must fail", rows, err)
			}
		})
	}
}

func TestExtractProductsMergedForChunk_CompactSpanErrorUsesFallback(t *testing.T) {
	badSpan := wireMap(t, strings.Replace(compactMergedTestResponse, `"s":[143]`, `"s":[999]`, 1))
	extractor := &fakeJSONExtractor{outs: []map[string]any{badSpan, wireMap(t, compactMergedTestResponse)}}
	p := newCompactMergedTestProcessor(extractor)
	p.FallbackModelName = "fallback"
	block := chunksToBlocks([]Chunk{compactMergedTestChunk})[0]
	rows, modelName, didFallback, err := p.extractProductsMergedForChunk(context.Background(), "evt", 0, 1, block, compactMergedTestChunk, "doc")
	if err != nil || len(rows) != 1 || modelName != "fallback" || !didFallback {
		t.Fatalf("rows=%d model=%q fallback=%v err=%v", len(rows), modelName, didFallback, err)
	}

	extractor = &fakeJSONExtractor{outs: []map[string]any{badSpan, badSpan}}
	p = newCompactMergedTestProcessor(extractor)
	p.FallbackModelName = "fallback"
	rows, _, _, err = p.extractProductsMergedForChunk(context.Background(), "evt", 0, 1, block, compactMergedTestChunk, "doc")
	if err == nil || rows != nil {
		t.Fatalf("both models cited a missing line: rows=%#v err=%v, want an error", rows, err)
	}
}

func TestExtractProductsMergedForChunk_TimeoutFallbackAlert21578(t *testing.T) {
	for _, fallbackSucceeds := range []bool{true, false} {
		t.Run(fmt.Sprintf("fallback succeeds=%t", fallbackSucceeds), func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			originalDB := ApiTypes.ProjectDBHandle
			ApiTypes.ProjectDBHandle = db
			t.Cleanup(func() { ApiTypes.ProjectDBHandle = originalDB })

			var primaryCalls, fallbackCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct{ Model string }
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				if request.Model == "test" {
					primaryCalls.Add(1)
					w.WriteHeader(http.StatusOK)
					w.(http.Flusher).Flush()
					<-r.Context().Done()
					return
				}
				fallbackCalls.Add(1)
				if !fallbackSucceeds {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"choices": []any{map[string]any{"message": map[string]any{"content": compactMergedTestResponse}}},
				})
			}))
			defer server.Close()

			client := &llmclients.OpenAIJSONClient{
				BaseURL: server.URL, APIKey: "test-key",
				HTTPClient: &http.Client{Timeout: 200 * time.Millisecond},
			}
			p := newCompactMergedTestProcessor(client)
			p.MergedPromptRef = "prompt-extract-products-merged-v3.md"
			p.FallbackModelName = "fallback"
			p.FallbackModelCfg = structureModelConfig{ModelName: "fallback"}
			block := chunksToBlocks([]Chunk{compactMergedTestChunk})[0]
			const chunkIndex = 6
			ctx := withLLMUserID(withLLMRunID(withLLMRecordID(context.Background(), 441), 3939), "test-user")
			var alerts []loggerutil.AlertRecord
			loggerutil.SetAlertSink(func(a loggerutil.AlertRecord) { alerts = append(alerts, a) })
			t.Cleanup(func() { loggerutil.SetAlertSink(nil) })

			if fallbackSucceeds {
				mock.ExpectExec("INSERT INTO kb.failed_llm_calls").
					WithArgs(int64(441), int64(3939), "extract products merged", p.MergedPromptRef, "test", "fallback", sqlmock.AnyArg(), "", chunkIndex).
					WillReturnResult(sqlmock.NewResult(1, 1))
			}
			rows, model, didFallback, err := p.extractProductsMergedForChunk(ctx, "evt", chunkIndex, 49, block, compactMergedTestChunk, "doc")
			if primaryCalls.Load() != 2 || fallbackCalls.Load() != 1 {
				t.Fatalf("calls: primary=%d fallback=%d, want 2 and 1", primaryCalls.Load(), fallbackCalls.Load())
			}
			if fallbackSucceeds {
				if err != nil || !didFallback || model != "fallback" || len(rows) != 1 || len(alerts) != 0 {
					t.Fatalf("recovery: rows=%d model=%q fallback=%v err=%v alerts=%v", len(rows), model, didFallback, err, alerts)
				}
			} else {
				if err == nil || didFallback {
					t.Fatalf("unrecovered call: model=%q fallback=%v err=%v", model, didFallback, err)
				}
				hasProductAlert := false
				for _, alert := range alerts {
					hasProductAlert = hasProductAlert || strings.HasPrefix(alert.Message, "primary products; retrying fallback model")
				}
				if !hasProductAlert {
					t.Fatal("missing product alert for unrecovered call")
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestExtractProductsMergedForChunk_CompactStructuredRetry(t *testing.T) {
	for _, first := range []string{
		`{"products":[{"product_name":"垃圾桶","evidence_lines":["143"]}]}`,
		strings.Replace(compactMergedTestResponse, `"t":"eq"`, `"t":"equipment"`, 1),
		strings.Replace(compactMergedTestResponse, `"s":[143]`, `"s":[]`, 1),
	} {
		var requests atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			source := compactMergedTestResponse
			if requests.Add(1) == 1 {
				source = first
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": source}}}})
		}))
		client := &llmclients.OpenAIJSONClient{BaseURL: srv.URL, APIKey: "test-key", ModelName: "test", HTTPClient: srv.Client()}
		p := newCompactMergedTestProcessor(client)
		block := chunksToBlocks([]Chunk{compactMergedTestChunk})[0]
		rows, _, _, err := p.extractProductsMergedForChunk(context.Background(), "evt", 0, 1, block, compactMergedTestChunk, "doc")
		srv.Close()
		if err != nil || len(rows) != 1 || requests.Load() != 2 {
			t.Fatalf("first=%s: requests=%d rows=%d err=%v; nonconforming output must be retried", first, requests.Load(), len(rows), err)
		}
	}
}

type productsRun261Fixture struct {
	Calls []struct {
		Lines []struct {
			LineNumber int    `json:"line_number"`
			Content    string `json:"content"`
		} `json:"lines"`
		Response string `json:"response"`
	} `json:"calls"`
}

func invertProductCodes(codes map[string]string) map[string]string {
	out := make(map[string]string, len(codes))
	for code, value := range codes {
		out[value] = code
	}
	return out
}

// legacyMergedRowToV2 re-encodes one captured v1 merged row in the v2 wire
// format without dropping any value except the model quote.
func legacyMergedRowToV2(t *testing.T, row map[string]any) map[string]any {
	t.Helper()
	code := func(codes map[string]string, value any) string {
		c, ok := invertProductCodes(codes)[strings.TrimSpace(asString(value))]
		if !ok {
			t.Fatalf("captured value %q has no v2 code", asString(value))
		}
		return c
	}
	details, _ := row["relation_details"].(map[string]any)
	out := map[string]any{
		"n": strings.TrimSpace(asString(row["product_name"])),
		"t": code(productV2Types, row["product_type"]),
		"r": code(productV2Relations, row["relation_type"]),
		"d": strings.TrimSpace(asString(row["product_summary"])),
		"o": code(productV2Obligations, details["obligation_level"]),
		"q": row["confidence"],
	}
	if c := strings.TrimSpace(asString(row["canonical_name"])); c != "" && c != out["n"] {
		out["c"] = c
	}
	spans := []any{}
	for _, span := range toStringSlice(row["evidence_lines"]) {
		if n, ok := parsePositiveInt(span); ok {
			spans = append(spans, n)
		} else {
			spans = append(spans, span)
		}
	}
	out["s"] = spans
	for key, field := range map[string]string{"x": "requirement_text", "a": "responsible_actor"} {
		if v := strings.TrimSpace(asString(details[field])); v != "" {
			out[key] = v
		}
	}
	if reason := strings.TrimSpace(asString(row["confidence_reason"])); reason != "" {
		out["cr"] = reason
	}
	for key, field := range map[string]string{"cnd": "conditions", "exc": "exceptions"} {
		if v := toStringSlice(details[field]); len(v) > 0 {
			out[key] = v
		}
	}
	var params []any
	for _, item := range objectItemsFromValue(details["thresholds_or_parameters"]) {
		entry := map[string]any{"n": strings.TrimSpace(asString(item["name"])), "v": item["value"]}
		if unit := strings.TrimSpace(asString(item["unit"])); unit != "" {
			entry["u"] = unit
		}
		params = append(params, entry)
	}
	if len(params) > 0 {
		out["p"] = params
	}
	var related []any
	for _, item := range objectItemsFromValue(details["related_products"]) {
		related = append(related, map[string]any{"n": strings.TrimSpace(asString(item["product_name"])), "r": code(productV2RelatedRelationships, item["relationship"])})
	}
	if len(related) > 0 {
		out["rp"] = related
	}
	return out
}

func parsePositiveInt(s string) (int, bool) {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	return n, s != "" && n > 0
}

// comparableProductRow is a normalized row with the fields whose meaning the
// wire format may not change, rendered through JSON so null and "" compare equal
// where the legacy normalizer already treats them alike.
func comparableProductRow(row map[string]any) string {
	params := []map[string]string{}
	for _, item := range objectItemsFromValue(row["parameters"]) {
		params = append(params, map[string]string{"name": strings.TrimSpace(asString(item["name"])), "value": strings.TrimSpace(asString(item["value"])), "unit": strings.TrimSpace(asString(item["unit"]))})
	}
	related := []map[string]string{}
	for _, item := range objectItemsFromValue(row["related_products"]) {
		related = append(related, map[string]string{"product_name": asString(item["product_name"]), "relationship": asString(item["relationship"])})
	}
	strs := func(v any) []string { return append([]string{}, toStringSlice(v)...) }
	canonical := firstNonEmptyTrimmed(asString(row["canonical_name"]), asString(row["product_name"]))
	bs, _ := json.Marshal([]any{
		row["product_name"], canonical, row["product_type"], row["relation_type"], row["relation_summary"],
		strs(row["evidence_lines"]), row["obligation_level"], row["requirement_text"], strs(row["conditions"]), strs(row["exceptions"]),
		params, related, row["responsible_actor"], row["confidence"], row["confidence_reason"],
	})
	return string(bs)
}

func TestProductsMergedV2Run261Replay(t *testing.T) {
	f, err := os.Open("testdata/products-merged-run-261.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	var fixture productsRun261Fixture
	if err := json.NewDecoder(gz).Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Calls) != 10 {
		t.Fatalf("calls=%d, want 10", len(fixture.Calls))
	}

	var legacyRows, compactRows []map[string]any
	var legacyChars, legacyBytes, compactChars, compactBytes int
	for i, call := range fixture.Calls {
		lines := productSourceLines{}
		for _, line := range call.Lines {
			lines[line.LineNumber] = line.Content
		}
		var legacy map[string]any
		if err := json.Unmarshal([]byte(call.Response), &legacy); err != nil {
			t.Fatal(err)
		}
		legacyItems, _ := legacy["products"].([]any)
		wireRows := make([]any, 0, len(legacyItems))
		for _, item := range legacyItems {
			wireRows = append(wireRows, legacyMergedRowToV2(t, item.(map[string]any)))
		}
		wire, _ := json.Marshal(map[string]any{"v": 2, "p": wireRows})
		original, _ := json.Marshal(legacy)
		legacyChars += utf8.RuneCount(original)
		legacyBytes += len(original)
		compactChars += utf8.RuneCount(wire)
		compactBytes += len(wire)

		decoded, err := decodeProductMergedV2(wireMap(t, string(wire)), lines)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		legacyRows = append(legacyRows, normalizeProductList(legacyItems)...)
		compactRows = append(compactRows, normalizeProductList(decoded["products"].([]any))...)
	}
	if len(legacyRows) != 165 || len(compactRows) != len(legacyRows) {
		t.Fatalf("rows legacy=%d compact=%d, want 165 each", len(legacyRows), len(compactRows))
	}
	for i := range legacyRows {
		if got, want := comparableProductRow(compactRows[i]), comparableProductRow(legacyRows[i]); got != want {
			t.Errorf("row %d changed meaning\n got %s\nwant %s", i, got, want)
		}
		quote := asString(compactRows[i]["evidence_quote"])
		if quote == "" || strings.Contains(quote, "#r") || strings.Contains(quote, "#h") {
			t.Errorf("row %d: rebuilt evidence_quote %q must be the cited text without row labels", i, quote)
		}
	}
	if got, want := len(dedupeFinalProductRows(compactRows)), len(dedupeFinalProductRows(legacyRows)); got != want {
		t.Fatalf("Step B rows compact=%d legacy=%d", got, want)
	}
	t.Logf("run 261 merged output: %d -> %d characters (-%.1f%%), %d -> %d bytes (-%.1f%%)",
		legacyChars, compactChars, 100*(1-float64(compactChars)/float64(legacyChars)),
		legacyBytes, compactBytes, 100*(1-float64(compactBytes)/float64(legacyBytes)))
	if float64(compactChars) > 0.45*float64(legacyChars) {
		t.Fatalf("compact output %d characters is more than 45%% of legacy %d", compactChars, legacyChars)
	}
}

func TestProductsDefaultMergedPromptUsesCompactFormat(t *testing.T) {
	t.Setenv("EXTRACT_PRODUCT_MERGED_PROMPT", "")
	p := NewProductsProcessor(&fakeDocMetadataStore{}, &fakeProductsStore{}, &fakeJSONExtractor{}, nil)
	if !productMergedPromptUsesV2(p.MergedPromptRef) || p.MergedPromptErr != nil {
		t.Fatalf("default merged prompt %q (err %v) must be the compact v2 prompt", p.MergedPromptRef, p.MergedPromptErr)
	}
	if !strings.Contains(p.MergedPromptText, `{"v":2,"p":[`) {
		t.Fatal("default merged prompt text does not describe the v2 format")
	}
}

// TestProductsMergedV2PromptExampleDecodes keeps the prompt's own example in
// step with the decoder: a model copying its shape must not be rejected.
func TestProductsMergedV2PromptExampleDecodes(t *testing.T) {
	text, _, _, err := loadPromptByRef("../../../prompts/prompt-extract-products-merged-v2.md")
	if err != nil {
		t.Fatal(err)
	}
	_, after, ok := strings.Cut(text, "Example (line numbers must come from the actual input):\n\n```json\n")
	example, _, ok2 := strings.Cut(after, "\n```")
	if !ok || !ok2 {
		t.Fatal("prompt example not found")
	}
	rows := decodeProductV2Test(t, example, productSourceLines{143: "垃圾桶应每周清洗一次。", 116: "116#h0: 序号 | 名称\n116#r3: 3 | 易拉罐"})
	if len(rows) != 2 || rows[1]["evidence_quote"] != "3 | 易拉罐" {
		t.Fatalf("prompt example decoded to %#v", rows)
	}
}

func TestProductsV2GroundsAShiftedCitationOnTheNamingLine(t *testing.T) {
	lines := productSourceLinesFromChunk([]MarkedLine{
		{Mark: "r", Line: Line{LineNo: 40, LineType: "heading", Content: "B.1 Germination test"}},
		{Mark: "r", Line: Line{LineNo: 41, LineType: "paragraph", Content: "Shake the sample on a reciprocating shaker for 1 h."}},
		{Mark: "r", Line: Line{LineNo: 42, LineType: "paragraph", Content: "Hazardous waste"}},
		{Mark: "r", Line: Line{LineNo: 43, LineType: "paragraph", Content: "Household waste that harms health or the environment."}},
	})
	rows := decodeProductV2Test(t, `{"v":2,"p":[`+
		`{"n":"reciprocating shaker","t":"eq","r":"use","d":"s","s":[40],"o":"m","q":0.9},`+
		`{"n":"hazardous waste","t":"pc","r":"risk","d":"s","s":[43],"o":"d","q":0.9},`+
		`{"n":"shaker","t":"eq","r":"use","d":"s","s":[41],"o":"m","q":0.9},`+
		`{"n":"collection vehicles","t":"eq","r":"req","d":"s","s":[43],"o":"m","q":0.9}]}`, lines)
	cases := []struct {
		spans []string
		quote string
	}{
		// Cited one line early: the naming line is added, the citation kept.
		{[]string{"40", "41"}, "B.1 Germination test\nShake the sample on a reciprocating shaker for 1 h."},
		// A definition line under its term gains the term's line.
		{[]string{"43", "42"}, "Hazardous waste\nHousehold waste that harms health or the environment."},
		// Already grounded: unchanged.
		{[]string{"41"}, "Shake the sample on a reciprocating shaker for 1 h."},
		// Named nowhere nearby (an expanded conjunct, say): unchanged.
		{[]string{"43"}, "Household waste that harms health or the environment."},
	}
	for i, c := range cases {
		if got := toStringSlice(rows[i]["evidence_lines"]); !reflect.DeepEqual(got, c.spans) {
			t.Errorf("row %d evidence_lines=%v, want %v", i, got, c.spans)
		}
		if rows[i]["evidence_quote"] != c.quote {
			t.Errorf("row %d evidence_quote=%q, want %q", i, rows[i]["evidence_quote"], c.quote)
		}
	}
}

// Prod record 382, chunk 14: deepseek-v4-flash cited "264#r10", a row line 264
// does not have. The schema passed, so the span check ran only after the
// client returned, and the chunk went straight to the fallback model (dead,
// HTTP 403). A grounding failure must get the primary's own retries first.
func TestProductsMergedV2RetriesPrimaryOnUngroundedSpan(t *testing.T) {
	bad := strings.Replace(productV2Example, `"116#r2"`, `"116#r10"`, 1)
	var models []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		models = append(models, req.Model)
		content := productV2Example
		if len(models) == 1 {
			content = bad
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}}}})
	}))
	defer srv.Close()

	client := &llmclients.OpenAIJSONClient{BaseURL: srv.URL, APIKey: "test-key", HTTPClient: srv.Client()}
	p := NewProductsProcessor(nil, nil, client, loggerutil.CreateDefaultLogger("20261117-382"))
	p.FallbackModelName = "fallback-model"
	payload, model, _, err := p.extractProductPayloadWithFallback(context.Background(), "extract products merged", "input", "prompt",
		"prompt-extract-products-merged-v3.md", "primary-model", structureModelConfig{}, productV2Lines, nil)
	if err != nil {
		t.Fatalf("extractProductPayloadWithFallback: %v", err)
	}
	if model != "primary-model" || !reflect.DeepEqual(models, []string{"primary-model", "primary-model"}) {
		t.Fatalf("model=%q requests=%v; want the primary retried and no fallback call", model, models)
	}
	if rows := payload["products"].([]any); len(rows) != 2 {
		t.Fatalf("products=%d, want 2 from the retried response", len(rows))
	}
}

// Replays record 382's captured primary response (ChenWeb#28): the retry request
// must name all nine invented rows of line 264, so one corrected response passes.
func TestProductsMergedV2RetryNamesEveryInvalidSpan(t *testing.T) {
	response, lines := loadProductsRecord382Chunk14(t)
	fixed := regexp.MustCompile(`"264#r[0-9]+"`).ReplaceAllString(response, "264")
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(body))
		content := fixed
		if len(bodies) == 1 {
			content = response
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}}}})
	}))
	defer srv.Close()

	client := &llmclients.OpenAIJSONClient{BaseURL: srv.URL, APIKey: "test-key", HTTPClient: srv.Client()}
	p := NewProductsProcessor(nil, nil, client, loggerutil.CreateDefaultLogger("20261117-382"))
	p.FallbackModelName = "fallback-model"
	payload, model, _, err := p.extractProductPayloadWithFallback(context.Background(), "extract products merged", "input", "prompt",
		"prompt-extract-products-merged-v3.md", "primary-model", structureModelConfig{}, lines, nil)
	if err != nil {
		t.Fatalf("extractProductPayloadWithFallback: %v", err)
	}
	if model != "primary-model" || len(bodies) != 2 {
		t.Fatalf("model=%q requests=%d; want one primary retry and no fallback call", model, len(bodies))
	}
	for r := 2; r <= 10; r++ {
		if span := fmt.Sprintf(`\"264#r%d\"`, r); !strings.Contains(bodies[1], span) {
			t.Errorf("retry request does not name %s", span)
		}
	}
	if rows := payload["products"].([]any); len(rows) != 12 {
		t.Fatalf("products=%d, want the 12 of the corrected response", len(rows))
	}
}
