package docprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	llmclients "github.com/chendingplano/shared/go/api/llm"
)

func preflightInputLines(t *testing.T, input string) []lineForLLM {
	t.Helper()
	var envelope struct {
		Lines json.RawMessage `json:"lines"`
	}
	if strings.HasPrefix(input, "{") {
		if err := json.Unmarshal([]byte(input), &envelope); err != nil {
			t.Fatal(err)
		}
		input = string(envelope.Lines)
	}
	var lines []lineForLLM
	if err := json.Unmarshal([]byte(input), &lines); err == nil {
		return lines
	}
	var compact [][]any
	if err := json.Unmarshal([]byte(input), &compact); err != nil {
		t.Fatal(err)
	}
	for _, row := range compact[1:] {
		lines = append(lines, lineForLLM{Flag: row[0].(string), LineNumber: int(row[1].(float64)), Content: row[4].(string)})
	}
	return lines
}

func preflightTable(rows int) string {
	var b strings.Builder
	b.WriteString(`<table><tr><td>材料</td><td>长度 mm</td><td>宽度 mm</td><td>高度 mm</td></tr>`)
	for i := 1; i <= rows; i++ {
		fmt.Fprintf(&b, `<tr><td>材料%d</td><td>%d</td><td>%d</td><td>%d</td></tr>`, i, i, i+10, i+20)
	}
	b.WriteString(`</table>`)
	return b.String()
}

func TestSimplePreflightSplitsBeforeFirstCall(t *testing.T) {
	for _, version := range []string{"v1", "v2"} {
		t.Run(version, func(t *testing.T) {
			seen := map[string]int{}
			ext := &simpleMetricsExtractor{respond: func(_ int, in llmclients.JSONExtractionInput) (map[string]any, error) {
				var rows []any
				count := 0
				for _, line := range preflightInputLines(t, in.InputText) {
					if line.Flag != "n" || line.LineNumber != 20 {
						continue
					}
					if !strings.Contains(line.Content, "20#h0:") {
						t.Error("table header lost")
					}
					for _, row := range strings.Split(line.Content, "\n") {
						id, _, _ := strings.Cut(row, ":")
						if !strings.HasPrefix(id, "20#r") {
							continue
						}
						seen[id]++
						count++
						rows = append(rows, map[string]any{"n": id, "s": "材料", "v": "1", "l": []any{"20"}, "t": []any{id}})
					}
				}
				if count == 6 {
					t.Error("the full dense table was sent before splitting")
				}
				if !strings.Contains(in.InputText, "表1") || !strings.Contains(in.InputText, "ambient condition") {
					t.Error("caption/prose context lost")
				}
				return map[string]any{"lang": "zh", "m": rows}, nil
			}}
			p := makeTestSimpleMetricsProcessor(ext, &fakeMetricsStore{}, nil)
			p.RelationPromptRef = "prompt-extract-metric-simple-" + version + ".md"
			p.RelationModelCfg.MaxOutputTokens = 1500
			chunk := Chunk{SeqNo: 2, Lines: []MarkedLine{simpleLine("o", 18, "paragraph", "ambient condition"), simpleLine("n", 19, "table-caption", "表1"), simpleLine("n", 20, "table", preflightTable(6))}}
			res, err := p.extractSimpleChunk(context.Background(), 813, chunk, "document", 1)
			if err != nil {
				t.Fatal(err)
			}
			if len(ext.inputs) <= 1 || len(res.rows) != 6 {
				t.Fatalf("calls=%d rows=%d", len(ext.inputs), len(res.rows))
			}
			for i := 1; i <= 6; i++ {
				if seen[fmt.Sprintf("20#r%d", i)] != 1 {
					t.Fatalf("row ownership=%v", seen)
				}
			}
			for i, row := range res.rows {
				if row["candidate_id"] != fmt.Sprintf("2_%d", i+1) {
					t.Errorf("candidate_id=%v", row["candidate_id"])
				}
				if !strings.Contains(asString(row["threshold_or_target"]), "材料") {
					t.Error("source expansion lost original table")
				}
			}
		})
	}
}

func TestSimplePreflightSmallChunkUnchanged(t *testing.T) {
	chunk := Chunk{SeqNo: 1, Lines: []MarkedLine{simpleLine("n", 20, "table", preflightTable(2))}}
	ext := &simpleMetricsExtractor{respond: func(_ int, in llmclients.JSONExtractionInput) (map[string]any, error) {
		if in.InputText != canonicalChunkInputText(chunk.Lines, "") {
			t.Error("small chunk changed")
		}
		return map[string]any{"m": []any{}}, nil
	}}
	p := makeTestSimpleMetricsProcessor(ext, &fakeMetricsStore{}, nil)
	p.RelationModelCfg.MaxOutputTokens = 100000
	if _, err := p.extractSimpleChunk(context.Background(), 813, chunk, "", 1); err != nil {
		t.Fatal(err)
	}
	if len(ext.inputs) != 1 {
		t.Fatalf("calls=%d", len(ext.inputs))
	}
}

func TestSimplePreflightTruncationSplitsSingleTableLine(t *testing.T) {
	ext := &simpleMetricsExtractor{respond: func(n int, _ llmclients.JSONExtractionInput) (map[string]any, error) {
		if n == 1 {
			return nil, fmt.Errorf("wrapped: %w", llmclients.ErrStructuredOutputTruncated)
		}
		return map[string]any{"m": []any{}}, nil
	}}
	p := makeTestSimpleMetricsProcessor(ext, &fakeMetricsStore{}, nil)
	p.RelationModelCfg.MaxOutputTokens = 100000
	_, err := p.extractSimpleChunk(context.Background(), 813, Chunk{SeqNo: 1, Lines: []MarkedLine{simpleLine("n", 20, "table", preflightTable(4))}}, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(ext.inputs) != 3 {
		t.Fatalf("calls=%d, want initial call and two row groups", len(ext.inputs))
	}
}

func TestSimplePreflightCancellationDiscardsSiblings(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ext := &simpleMetricsExtractor{respond: func(_ int, _ llmclients.JSONExtractionInput) (map[string]any, error) {
		cancel()
		return map[string]any{"m": []any{map[string]any{"n": "length", "v": "1", "l": []any{"20"}, "t": []any{"20#r1"}}}}, nil
	}}
	p := makeTestSimpleMetricsProcessor(ext, &fakeMetricsStore{}, nil)
	p.RelationModelCfg.MaxOutputTokens = 1500
	res, err := p.extractSimpleChunk(ctx, 813, Chunk{SeqNo: 1, Lines: []MarkedLine{simpleLine("n", 20, "table", preflightTable(6))}}, "", 1)
	if !errors.Is(err, context.Canceled) || !res.empty() {
		t.Fatalf("result=%+v err=%v", res, err)
	}
	if len(ext.inputs) != 1 {
		t.Fatalf("calls=%d after cancellation", len(ext.inputs))
	}
}

func TestSimplePreflightPreservesMergedCellsNotesAndRowOwnership(t *testing.T) {
	table := `<table><tr><th>材料</th><th>长度 mm</th><th>宽度 mm</th></tr>` +
		`<tr><td rowspan="2">共用材料</td><td>1</td><td>11</td></tr>` +
		`<tr><td>2</td><td>12</td></tr><tr><td>另一材料</td><td>3</td><td>13</td></tr>` +
		`<tr><td colspan="3">注：最大温度为100 ℃。</td></tr></table>`
	chunk := Chunk{SeqNo: 3, Lines: []MarkedLine{simpleLine("n", 20, "table", table)}}
	normal := map[string]int{}
	contextNotes := 0
	ext := &simpleMetricsExtractor{respond: func(_ int, in llmclients.JSONExtractionInput) (map[string]any, error) {
		for _, line := range preflightInputLines(t, in.InputText) {
			if line.Flag == "o" && strings.Contains(line.Content, "20#r4:") {
				contextNotes++
			}
			if line.Flag != "n" {
				continue
			}
			for _, row := range strings.Split(line.Content, "\n") {
				id, _, _ := strings.Cut(row, ":")
				if strings.HasPrefix(id, "20#r") {
					normal[id]++
				}
				if id == "20#r2" && !strings.Contains(row, "共用材料") {
					t.Error("rowspan label lost")
				}
			}
		}
		// Deliberately return all rows, including context-only rows, every time.
		var metrics, excluded []any
		for i := 1; i <= 4; i++ {
			ref := fmt.Sprintf("20#r%d", i)
			metrics = append(metrics, map[string]any{"n": ref, "s": "材料", "v": "1", "l": []any{"20"}, "t": []any{ref}})
			excluded = append(excluded, map[string]any{"n": ref, "r": "term_definition", "l": []any{"20"}, "t": []any{ref}})
		}
		return map[string]any{"m": metrics, "x": excluded}, nil
	}}
	p := makeTestSimpleMetricsProcessor(ext, &fakeMetricsStore{}, nil)
	p.RelationModelCfg.MaxOutputTokens = 1400
	res, err := p.extractSimpleChunk(context.Background(), 813, chunk, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.rows) != 4 || len(res.dropped) != 4 || contextNotes == 0 {
		t.Fatalf("metrics=%d excluded=%d contextNotes=%d", len(res.rows), len(res.dropped), contextNotes)
	}
	for i := 1; i <= 4; i++ {
		if normal[fmt.Sprintf("20#r%d", i)] != 1 {
			t.Fatalf("normal rows=%v", normal)
		}
	}
	if !strings.Contains(asString(res.rows[1]["threshold_or_target"]), "20#r2: 共用材料") {
		t.Errorf("source=%v", res.rows[1]["threshold_or_target"])
	}
	if chunk.Lines[0].Line.Content != table || chunk.Lines[0].Mark != "n" {
		t.Fatal("original source mutated")
	}
}

func TestSimplePreflightMissingOrInvalidCitationFailsAtomically(t *testing.T) {
	for _, kind := range []string{"missing", "invalid", "set-aside"} {
		t.Run(kind, func(t *testing.T) {
			ext := &simpleMetricsExtractor{respond: func(n int, _ llmclients.JSONExtractionInput) (map[string]any, error) {
				row := map[string]any{"n": "length", "v": "1", "l": []any{"20"}, "t": []any{"20#r1"}}
				key := "m"
				if n == 2 {
					if kind == "invalid" {
						row["t"] = []any{"20#r999"}
					} else {
						delete(row, "t")
					}
					if kind == "set-aside" {
						key = "x"
						row["r"] = "no_value"
					}
				}
				payload := map[string]any{"m": []any{}}
				payload[key] = []any{row}
				return payload, nil
			}}
			p := makeTestSimpleMetricsProcessor(ext, &fakeMetricsStore{}, nil)
			p.RelationModelCfg.MaxOutputTokens = 1500
			res, err := p.extractSimpleChunk(context.Background(), 813, Chunk{SeqNo: 1, Lines: []MarkedLine{simpleLine("n", 20, "table", preflightTable(3))}}, "", 1)
			if err == nil || !res.empty() || len(ext.inputs) != 2 {
				t.Fatalf("result=%+v error=%v calls=%d", res, err, len(ext.inputs))
			}
		})
	}
}

func TestSimplePreflightBoundedTruncationAndIndivisibleLine(t *testing.T) {
	for _, tc := range []struct{ rows, calls int }{{1, 1}, {32, 4}} {
		ext := &simpleMetricsExtractor{respond: func(_ int, _ llmclients.JSONExtractionInput) (map[string]any, error) {
			return nil, llmclients.ErrLLMResponseTruncated
		}}
		p := makeTestSimpleMetricsProcessor(ext, &fakeMetricsStore{}, nil)
		p.RelationModelCfg.MaxOutputTokens = 1000000
		res, err := p.extractSimpleChunk(context.Background(), 813, Chunk{SeqNo: 1, Lines: []MarkedLine{simpleLine("n", 20, "table", preflightTable(tc.rows))}}, "", 1)
		if !errors.Is(err, llmclients.ErrLLMResponseTruncated) || !res.empty() || len(ext.inputs) != tc.calls {
			t.Fatalf("rows=%d result=%+v err=%v calls=%d", tc.rows, res, err, len(ext.inputs))
		}
	}
}

func TestSimplePreflightBudgetsAndCountsActualCalls(t *testing.T) {
	work := newMetricSimpleWork([]MarkedLine{simpleLine("n", 20, "table", preflightTable(30))})
	for _, limit := range []int{0, 2000, 100000} {
		parts, budget := work.plan(limit)
		if limit > 0 && budget != limit/2 {
			t.Errorf("budget=%d limit=%d", budget, limit)
		}
		if limit == 0 && budget != 2048 {
			t.Errorf("unconfigured cap budget=%d", budget)
		}
		for _, part := range parts {
			if len(part.units) > 1 && part.estimate() > budget {
				t.Errorf("estimate=%d budget=%d", part.estimate(), budget)
			}
		}
	}
	if estimateMetricSimpleOutput("1 2 3 4", "long column header") <= estimateMetricSimpleOutput("1", "x") {
		t.Error("width/density and labels ignored")
	}
	ext := &simpleMetricsExtractor{respond: func(n int, _ llmclients.JSONExtractionInput) (map[string]any, error) {
		if n == 1 {
			return nil, llmclients.ErrLLMResponseTruncated
		}
		return map[string]any{"m": []any{}}, nil
	}}
	p := makeTestSimpleMetricsProcessor(ext, &fakeMetricsStore{}, nil)
	p.RelationModelCfg.MaxOutputTokens = 100000
	p.simpleWarmupWait = 0
	res, err := p.extractMetricsSimple(context.Background(), 813, []Chunk{{SeqNo: 1, Lines: work.source}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.LLMCallCount != 3 {
		t.Errorf("calls=%d, want failed parent plus successful halves", res.LLMCallCount)
	}
}

func TestSimplePreflightKeepsSectionOrderAndQualifyingCitations(t *testing.T) {
	table := `<table><tr><th>项目</th><th>值</th></tr>` +
		`<tr><td colspan="2">分类 A；试验温度20 ℃</td></tr><tr><td>长度</td><td>1</td></tr>` +
		`<tr><td colspan="2">分类 B；试验温度30 ℃</td></tr><tr><td>长度</td><td>2</td></tr></table>`
	work := newMetricSimpleWork([]MarkedLine{simpleLine("o", 10, "table", preflightTable(1)), simpleLine("n", 20, "table", table)})
	var selected metricSimpleUnit
	for _, unit := range work.units {
		if unit.rowID == "r2" {
			selected = unit
		}
	}
	s := metricSimpleSelection{work, []metricSimpleUnit{selected}}
	input := markedLinesToJSON(s.lines())
	if !(strings.Index(input, "20#r1:") < strings.Index(input, "20#r2:") && strings.Index(input, "20#r2:") < strings.Index(input, "20#r3:")) {
		t.Errorf("section order lost: %s", input)
	}
	for _, refs := range [][]any{{"20#r1", "20#r2"}, {"10#r1", "20#r2"}} {
		accept, err := s.accepts(map[string]any{"t": refs}, []string{"20"})
		if !accept || err != nil {
			t.Errorf("refs=%v accept=%t err=%v", refs, accept, err)
		}
	}
	if !newSimpleChunkLines(s.sourceLines(s.lines())).hasNormalLine([]string{"20"}) {
		t.Fatal("trailing overlap note hid the normal segment")
	}
}

// Record 383, chunk 229 (ChenWeb#29): in a split table segment the model cited only a
// header row for a set-aside item. The row-citation check ran after the structured call
// returned, so the whole segment failed without a retry. It must run as CheckParsed: the
// primary is retried with text naming the item, and the segment's rows are kept.
func TestSimplePreflightRetriesPrimaryOnHeaderOnlyCitation(t *testing.T) {
	var (
		mu     sync.Mutex
		bodies []string
		bad    = true
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(body, &req); err != nil || len(req.Messages) == 0 {
			t.Errorf("request: %v", err)
			return
		}
		input, _, retry := strings.Cut(req.Messages[len(req.Messages)-1].Content, "\n\nRetry instructions:")
		mu.Lock()
		bodies = append(bodies, string(body))
		addBad := bad && !retry
		bad = bad && retry // only the first segment's first answer cites the header alone
		mu.Unlock()
		var metrics []any
		for _, line := range preflightInputLines(t, input) {
			if line.Flag != "n" || line.LineNumber != 20 {
				continue
			}
			for _, row := range strings.Split(line.Content, "\n") {
				if id, _, _ := strings.Cut(row, ":"); strings.HasPrefix(id, "20#r") {
					metrics = append(metrics, map[string]any{"n": id, "s": "材料", "v": "1", "l": []any{"20"}, "t": []any{id}})
				}
			}
		}
		payload := map[string]any{"lang": "zh", "m": metrics}
		if addBad {
			payload["x"] = []any{map[string]any{"n": "尺寸档位", "r": "term_definition", "l": []any{"20"}, "t": []any{"20#h0"}}}
		}
		content, _ := json.Marshal(payload)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": string(content)}}}})
	}))
	defer srv.Close()

	client := &llmclients.OpenAIJSONClient{BaseURL: srv.URL, APIKey: "test-key", HTTPClient: srv.Client()}
	p := makeTestSimpleMetricsProcessor(client, &fakeMetricsStore{}, nil)
	p.RelationModelCfg.MaxOutputTokens = 1500
	chunk := Chunk{SeqNo: 229, Lines: []MarkedLine{simpleLine("n", 20, "table", preflightTable(6))}}
	res, err := p.extractSimpleChunk(context.Background(), 383, chunk, "", 1)
	if err != nil {
		t.Fatalf("extractSimpleChunk: %v", err)
	}
	if len(res.rows) != 6 {
		t.Fatalf("rows=%d, want all 6 rows kept after the retry", len(res.rows))
	}
	retries := 0
	for _, b := range bodies {
		if strings.Contains(b, "Retry instructions:") {
			retries++
			if !strings.Contains(b, "尺寸档位") || !strings.Contains(b, "20#h0") {
				t.Errorf("retry request does not name the item and its citation: %s", b)
			}
		}
	}
	if retries != 1 {
		t.Fatalf("retries=%d of %d requests, want exactly one", retries, len(bodies))
	}
}

func TestSimplePreflightRetainsExplicitRowCitationsInLineField(t *testing.T) {
	// Runshen record 468 put precise citations in l and omitted t.
	row := map[string]any{
		"n": "教育培训总分", "s": "饮料制造企业", "v": "48", "u": "分",
		"k": "requirement", "r": "exact", "l": []any{"916#r1", "916#r2"},
	}
	work := newMetricSimpleWork([]MarkedLine{simpleLine("n", 916, "table", preflightTable(4))})
	first := metricSimpleSelection{work, work.units[:2]}
	second := metricSimpleSelection{work, work.units[2:]}
	spans := normalizeSourceLineSpans(row["l"])
	if owned, err := first.accepts(row, spans); err != nil || !owned {
		t.Fatalf("first segment owns the cited rows: owned=%t error=%v", owned, err)
	}
	if owned, err := second.accepts(row, spans); err != nil || owned {
		t.Fatalf("second segment must omit the same rows: owned=%t error=%v", owned, err)
	}

	expanded := expandSimpleMetricRow(row, newSimpleChunkLines(work.source))
	refs, _ := expanded["source_table_rows"].([]TableRowRef)
	if len(refs) != 1 || refs[0].Line != 916 || strings.Join(refs[0].Rows, ",") != "r1,r2" {
		t.Fatalf("expected both explicit source rows in the normalized metric, got %+v", refs)
	}

	row["t"] = []any{"916#r999"}
	if _, err := first.accepts(row, spans); err == nil {
		t.Fatal("explicit invalid t must fail even when l has usable row citations")
	}
}
