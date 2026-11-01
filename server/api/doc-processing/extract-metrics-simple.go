package docprocessing

// extract_metrics_simple (openspec change extract-metrics-simple): the metrics of a chunk in one
// LLM call, with the instructions as a cacheable system message and a compact output. Everything
// after the calls (finishMetricRows, save, merge, indexing) is extract_metrics' own code.

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

const (
	metricsSimpleProcessorName = "extract_metrics_simple"
	// A new DeepSeek prompt prefix becomes reusable about 60 s after the call that planted it
	// (measured 2026-08-16, KnowledgeStore bug 2026081601).
	metricSimpleWarmupWait   = 60 * time.Second
	metricSimpleTextMaxRunes = 300
	metricSimpleConfidence   = 0.8
)

// metricSimpleResult holds the rows, drop-tagged rows and set-aside items of one or more chunks.
type metricSimpleResult struct {
	rows     []map[string]any
	tagged   []map[string]any
	dropped  []droppedMetricRow
	language string
}

func (r metricSimpleResult) empty() bool {
	return len(r.rows) == 0 && len(r.tagged) == 0 && len(r.dropped) == 0
}

func (r *metricSimpleResult) add(o metricSimpleResult) {
	r.rows = append(r.rows, o.rows...)
	r.tagged = append(r.tagged, o.tagged...)
	r.dropped = append(r.dropped, o.dropped...)
	if r.language == "" || r.language == "unknown" {
		r.language = o.language
	}
}

// NewMetricsSimpleProcessor builds extract_metrics' processor in simple mode. Its one prompt and
// model take the place of both passes' prompts and models, so the shared checks, logs and saves
// (which read the candidate and relation fields) use them.
func NewMetricsSimpleProcessor(inputStore DocMetadataStore, store MetricsStore, extractor LLMJSONExtractor, logger ApiTypes.JimoLogger) *MetricsProcessor {
	p := NewMetricsProcessor(inputStore, store, extractor, logger)
	p.name = metricsSimpleProcessorName
	p.simple = true
	p.simpleWarmupWait = metricSimpleWarmupWait
	p.Logger = loggerutil.CreateDefaultLogger("20261101-317")
	p.setSimplePrompt(loadProductPromptFromEnvKeys(
		[]string{"EXTRACT_METRICS_SIMPLE_PROMPT"},
		"prompt-extract-metric-simple-v1.md",
	))
	p.setSimpleModel(loadModelConfigFromEnvKeys(
		[]string{"EXTRACT_METRICS_SIMPLE_MODEL_NAME", "ENRICH_METRICS_MODEL_NAME", "EXTRACT_METRICS_MODEL_NAME"},
		"MODEL_DEF_FILE",
	))
	return p
}

func (p *MetricsProcessor) setSimplePrompt(text, ref, path string, err error) {
	p.MentionPromptText, p.MentionPromptRef, p.MentionPromptPath, p.MentionPromptErr = text, ref, path, err
	p.RelationPromptText, p.RelationPromptRef, p.RelationPromptPath, p.RelationPromptErr = text, ref, path, err
	p.PromptText, p.PromptRef, p.PromptPath, p.PromptErr = text, ref, path, err
}

func (p *MetricsProcessor) setSimpleModel(ref, cfgPath string, cfg structureModelConfig, err error) {
	cfg = forceDisableThinking(cfg)
	p.MentionModelRef, p.MentionModelCfgPath, p.MentionModelCfg, p.MentionModelErr = ref, cfgPath, cfg, err
	p.MentionModelName = cfg.ModelName
	p.RelationModelRef, p.RelationModelCfgPath, p.RelationModelCfg, p.RelationModelErr = ref, cfgPath, cfg, err
	p.RelationModelName = cfg.ModelName
	p.ModelRef, p.ModelCfgPath, p.ModelErr, p.ModelName = ref, cfgPath, err, cfg.ModelName
	// The halves retry (extractSimpleChunk) replaces the candidate pass's fallback model.
	p.FallbackMentionModelRef, p.FallbackMentionModelName, p.FallbackMentionModelErr = "", "", nil
	applyStructureModelConfigToExtractor(p.Extractor, cfg)
}

// metricProcessorConflict refuses a run that selects both metric processors: they write the same
// record's kb.metrics rows. It is checked per run, not in the processor plan, because the runtime
// plan holds every configured processor.
func metricProcessorConflict(processors []Processor) error {
	found := 0
	for _, p := range processors {
		if p != nil && (p.Name() == "extract_metrics" || p.Name() == metricsSimpleProcessorName) {
			found++
		}
	}
	if found > 1 {
		return fmt.Errorf("(MID-20261101-07) extract_metrics and extract_metrics_simple cannot run together: both write the record's metrics")
	}
	return nil
}

// extractMetricsSimple is the sequential path's extraction: one call per chunk, the warm-up gate
// first, then the steps shared with extract_metrics.
func (p *MetricsProcessor) extractMetricsSimple(ctx context.Context, recordID int64, chunks []Chunk, docCtx string) (metricExtractionResult, error) {
	maxTasks := max(p.MaxTasks, 1)
	gate := newMetricWarmupGate(p.Now, p.simpleWarmupWait)
	p.Logger.Info("extract metrics simple", "record_id", recordID, "chunks", len(chunks),
		"model name", p.RelationModelName, "prompt", p.RelationPromptRef)
	results, err := runConcurrent(ctx, maxTasks, len(chunks), func(concCtx context.Context, i int) (metricSimpleResult, error) {
		var res metricSimpleResult
		var callErr error
		if !gate.do(concCtx, func() bool {
			res, callErr = p.extractSimpleChunk(concCtx, recordID, chunks[i], docCtx, len(chunks))
			return callErr == nil && p.simpleCacheWarm()
		}) || isCtxStopped(concCtx) {
			return metricSimpleResult{}, ErrPipelineStopped
		}
		if callErr != nil {
			return metricSimpleResult{}, fmt.Errorf("(MID-20261101-05) extract metrics simple via llm: %w", callErr)
		}
		return res, nil
	})
	if err != nil {
		if isCtxStopped(ctx) {
			return metricExtractionResult{}, ErrPipelineStopped
		}
		return metricExtractionResult{}, err
	}
	var all metricSimpleResult
	for _, r := range results {
		all.add(r)
	}
	found := len(all.rows) + len(all.tagged) + len(all.dropped)
	metrics, dropped := p.finishMetricRows(ctx, recordID, all.rows, all.tagged, all.dropped, chunks)
	return metricExtractionResult{
		Language:     firstNonEmptyTrimmed(all.language, "unknown"),
		Candidates:   found,
		Metrics:      metrics,
		Dropped:      dropped,
		ModelName:    p.RelationModelName,
		LLMCallCount: len(chunks),
	}, nil
}

// processSimpleChunk is ProcessChunk in simple mode. Like a failed candidate-pass chunk, a chunk
// whose call and halves retry both fail is logged and skipped.
func (p *MetricsProcessor) processSimpleChunk(ctx context.Context, chunkIdx int) error {
	chunk := p.batchChunks[chunkIdx]
	var res metricSimpleResult
	var err error
	if !p.simpleWarmup.do(ctx, func() bool {
		res, err = p.extractSimpleChunk(ctx, p.batchRecordID, chunk, p.batchDocCtx, len(p.batchChunks))
		return err == nil && p.simpleCacheWarm()
	}) || isCtxStopped(ctx) {
		return ErrPipelineStopped
	}
	if err != nil {
		p.Logger.Warn("extract metrics simple: chunk failed", "record_id", p.batchRecordID, "chunk", chunkIdx, "error", err)
		return nil
	}
	p.batchMu.Lock()
	p.batchSimple.add(res)
	p.batchMu.Unlock()
	return nil
}

// simpleCacheWarm reports whether the last call got at least half of its input from the cache.
// It is read right after the warm-up call, the only call of this processor's client in flight.
func (p *MetricsProcessor) simpleCacheWarm() bool {
	hit, miss := cacheTokenCounts(p.Extractor)
	return hit+miss > 0 && 2*hit >= hit+miss
}

// extractSimpleChunk makes the chunk's call and turns the response into rows and set-aside
// items. A failed call is retried once as two calls, each extracting from one half of the normal
// lines with the other half shown as overlap (design D5).
func (p *MetricsProcessor) extractSimpleChunk(ctx context.Context, recordID int64, chunk Chunk, docCtx string, total int) (metricSimpleResult, error) {
	res, err := p.callSimpleChunk(ctx, recordID, chunk, chunk.Lines, docCtx, total, "")
	if err != nil && !isCtxStopped(ctx) {
		first, second, ok := splitChunkHalves(chunk.Lines)
		if ok {
			p.Logger.Warn("extract metrics simple: chunk call failed; retrying as two halves",
				"record_id", recordID, "chunk", chunk.SeqNo, "error", err)
			a, errA := p.callSimpleChunk(ctx, recordID, chunk, first, docCtx, total, "_h1")
			b, errB := p.callSimpleChunk(ctx, recordID, chunk, second, docCtx, total, "_h2")
			if errA == nil && errB == nil {
				a.add(b)
				res, err = a, nil
			} else {
				err = fmt.Errorf("(MID-20261101-06) %w; halves retry: %v %v", err, errA, errB)
			}
		}
	}
	if err != nil {
		return metricSimpleResult{}, err
	}
	n := 0
	for _, row := range res.rows {
		n++
		row["candidate_id"] = fmt.Sprintf("%d_%d", chunk.SeqNo, n)
	}
	for _, d := range res.dropped {
		n++
		d.Row["candidate_id"] = fmt.Sprintf("%d_%d", chunk.SeqNo, n)
	}
	res.rows, res.tagged = dropRowsTaggedWithDropReason(res.rows)
	return res, nil
}

// callSimpleChunk makes one call over lines (the chunk's, or a halves-retry copy of them) and
// parses its response; callSuffix tells the halves' log entries apart.
func (p *MetricsProcessor) callSimpleChunk(ctx context.Context, recordID int64, chunk Chunk, lines []MarkedLine, docCtx string, total int, callSuffix string) (metricSimpleResult, error) {
	start := p.Now()
	payload, err := p.extractMetricSimplePayload(ctx, canonicalChunkInputText(lines, docCtx))
	p.logExtractMetricsChunk(ctx, recordID, fmt.Sprintf("%d_s_c%d%s", recordID, chunk.SeqNo, callSuffix),
		chunk.SeqNo, total, []string{p.RelationModelName}, p.RelationPromptRef, payload, err, start, p.Now(), "")
	if err != nil {
		return metricSimpleResult{}, err
	}
	idx := newSimpleChunkLines(lines)
	res := metricSimpleResult{language: normalizeMetricSimpleLang(asString(payload["lang"]))}
	discarded := 0
	rows, _ := payload["m"].([]any)
	for _, item := range rows {
		raw, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := expandSimpleMetricRow(raw, idx)
		spans, _ := row["source_line_spans"].([]string)
		switch {
		case asString(row["metric_name"]) == "" || len(spans) == 0:
			res.dropped = append(res.dropped, newDroppedMetricRow(row, metricDropStageEnrich, "invalid_row"))
		case !idx.hasNormalLine(spans):
			discarded++
		default:
			res.rows = append(res.rows, row)
		}
	}
	items, _ := payload["x"].([]any)
	for _, item := range items {
		raw, ok := item.(map[string]any)
		if !ok {
			continue
		}
		spans := normalizeSourceLineSpans(raw["l"])
		if len(spans) > 0 && !idx.hasNormalLine(spans) {
			discarded++
			continue
		}
		res.dropped = append(res.dropped, newDroppedMetricRow(map[string]any{
			"metric_name":         strings.TrimSpace(asString(raw["n"])),
			"source_line_spans":   spans,
			"threshold_or_target": idx.sourceText(spans, nil),
		}, metricDropStageEnrich, firstNonEmptyTrimmed(asString(raw["r"]), "unspecified")))
	}
	if discarded > 0 {
		p.Logger.Info("extract metrics simple: discarded rows on overlap lines only",
			"record_id", recordID, "chunk", chunk.SeqNo, "discarded", discarded)
	}
	return res, nil
}

// extractMetricSimplePayload sends the prompt as the system message and the chunk as the user
// message (DocumentFirst false), so the instructions are one cached prefix for every call.
func (p *MetricsProcessor) extractMetricSimplePayload(ctx context.Context, inputText string) (map[string]any, error) {
	applyStructureModelConfigToExtractor(p.Extractor, p.RelationModelCfg)
	in := newLLMJSONInput(ctx, p.RelationPromptRef, p.RelationPromptText, p.RelationModelName, inputText, p.Name(), "MID-20261101-01")
	in.DocumentFirst = false
	var payload map[string]any
	if structured, ok := p.Extractor.(LLMStructuredJSONExtractor); ok {
		result, err := structured.ExtractStructuredJSON(ctx, in, metricsSimpleExtractionContract())
		if err != nil {
			return nil, fmt.Errorf("(MID-20261101-02) extract metrics simple: %w", err)
		}
		if result == nil {
			return nil, fmt.Errorf("(MID-20261101-03) extract metrics simple: no result")
		}
		payload = result.Parsed
	} else {
		var err error
		if payload, err = p.Extractor.ExtractJSON(ctx, in); err != nil {
			return nil, fmt.Errorf("(MID-20261101-02) extract metrics simple: %w", err)
		}
	}
	if _, ok := payload["m"].([]any); !ok {
		return nil, fmt.Errorf("(MID-20261101-04) llm output must contain an 'm' array")
	}
	return payload, nil
}

func normalizeMetricSimpleLang(raw string) string {
	lang := ApiUtils.NormalizeLang(raw)
	if lang == "chinese" || lang == "中文" || lang == "zh-cn" {
		return "zh"
	}
	return lang
}

// splitChunkHalves returns two copies of lines; each keeps one half of the normal lines and
// marks the other half overlap. ok is false when there are fewer than two normal lines.
func splitChunkHalves(lines []MarkedLine) (first, second []MarkedLine, ok bool) {
	var normal []int
	for i, l := range lines {
		if normalizeChunkFlag(l.Mark) == "n" {
			normal = append(normal, i)
		}
	}
	if len(normal) < 2 {
		return nil, nil, false
	}
	first = append([]MarkedLine(nil), lines...)
	second = append([]MarkedLine(nil), lines...)
	for k, i := range normal {
		if k < len(normal)/2 {
			second[i].Mark = "o"
		} else {
			first[i].Mark = "o"
		}
	}
	return first, second, true
}

// expandSimpleMetricRow turns a compact row (spec metric-simple-extraction) into the long-named
// row the shared steps and the store use, and fills the fields the model does not return.
func expandSimpleMetricRow(raw map[string]any, idx simpleChunkLines) map[string]any {
	name, subject := strings.TrimSpace(asString(raw["n"])), strings.TrimSpace(asString(raw["s"]))
	nameEn, subjectEn := strings.TrimSpace(asString(raw["ne"])), strings.TrimSpace(asString(raw["se"]))
	rangeType := strings.TrimSpace(asString(raw["r"]))
	valueClass := strings.TrimSpace(asString(raw["k"]))
	row := normalizeMetricList([]any{map[string]any{
		"metric_name":           name,
		"metric_name_en":        nameEn,
		"subject":               subject,
		"subject_en":            subjectEn,
		"metric_value":          raw["v"],
		"unit":                  raw["u"],
		"value_range_type":      rangeType,
		"value_class":           valueClass,
		"value_class_en":        valueClass,
		"condition":             raw["c"],
		"condition_en":          raw["ce"],
		"source_line_spans":     raw["l"],
		"source_table_rows":     raw["t"],
		"value_structure":       raw["vs"],
		"value_min":             raw["lo"],
		"value_max":             raw["hi"],
		"reasoning_tags":        raw["g"],
		"formula_or_definition": raw["d"],
		"metric_categories":     raw["cat"],
		"keywords":              distinctNonEmpty(name, subject),
		"keywords_en":           distinctNonEmpty(nameEn, subjectEn),
		"confidence":            metricSimpleConfidence,
		"is_explicit_metric":    metricSimpleExplicitRangeTypes[rangeType],
	}})[0]
	if cats, _ := row["metric_categories"].([]string); len(cats) == 0 {
		row["metric_categories"] = []string{firstNonEmptyTrimmed(valueClass, "metric")}
	}
	spans, _ := row["source_line_spans"].([]string)
	refs, _ := row["source_table_rows"].([]TableRowRef)
	text := idx.sourceText(spans, refs)
	row["context"] = text
	row["threshold_or_target"] = text
	row["table_name_or_section"] = idx.section(spans)
	row["location_type"] = idx.locationType(spans)
	row["value_data_type"] = metricSimpleValueDataType(asString(row["metric_value"]), row["value_structure"])
	row["unit_en"] = metricSimpleUnitEn(asString(row["unit"]))
	return row
}

var metricSimpleExplicitRangeTypes = map[string]bool{"lower_bound": true, "upper_bound": true, "exact": true, "range": true}

func distinctNonEmpty(values ...string) []string {
	out := []string{}
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" && !containsString(out, v) {
			out = append(out, v)
		}
	}
	return out
}

// metricSimpleNumberRe matches a number, an interval (a~b, a-b) and a tolerance (±x, a±b).
var metricSimpleNumberRe = regexp.MustCompile(`^[±+\-]?\d+(\.\d+)?(\s*[~～\-±]\s*[+\-]?\d+(\.\d+)?)?$`)

func metricSimpleValueDataType(value string, structure any) string {
	if s, ok := structure.(map[string]any); ok && len(s) > 0 {
		switch asString(s["kind"]) {
		case "alternatives", "required_points", "composite":
			return "number"
		}
		return "string"
	}
	if metricSimpleNumberRe.MatchString(strings.ReplaceAll(strings.TrimSpace(value), ",", "")) {
		return "number"
	}
	return "string"
}

// metricSimpleUnitsEn translates the common Chinese units; a unit with no CJK characters is
// already its English form. Compound units translate per "/" part ("次/日" → "times/d").
var metricSimpleUnitsEn = map[string]string{
	"个": "pcs", "件": "pcs", "支": "pcs", "只": "pcs", "根": "pcs", "片": "pieces", "粒": "grains",
	"张": "sheets", "台": "units", "套": "sets", "组": "sets", "份": "portions", "种": "kinds",
	"项": "items", "处": "places", "层": "layers", "人": "persons", "户": "households",
	"次": "times", "倍": "times", "位": "digits", "圈": "turns", "步": "steps",
	"秒": "s", "分钟": "min", "小时": "h", "天": "d", "日": "d", "周": "weeks", "月": "months",
	"年": "years", "岁": "years", "周岁": "years",
	"米": "m", "厘米": "cm", "毫米": "mm", "千米": "km", "克": "g", "千克": "kg", "毫克": "mg",
	"吨": "t", "升": "L", "毫升": "mL", "立方米": "m³", "平方米": "m²", "元": "CNY", "万元": "10k CNY",
}

var metricSimpleCJKRe = regexp.MustCompile(`[\p{Han}]`)

func metricSimpleUnitEn(unit string) string {
	unit = strings.TrimSpace(unit)
	if !metricSimpleCJKRe.MatchString(unit) {
		return unit
	}
	parts := strings.Split(unit, "/")
	for i, part := range parts {
		if en, ok := metricSimpleUnitsEn[strings.TrimSpace(part)]; ok {
			parts[i] = en
		}
	}
	return strings.Join(parts, "/")
}

// simpleChunkLines indexes the lines of one call, for the overlap check and the fields built
// from source text.
type simpleChunkLines struct {
	ordered []MarkedLine
	byNo    map[int]MarkedLine
}

func newSimpleChunkLines(lines []MarkedLine) simpleChunkLines {
	idx := simpleChunkLines{ordered: lines, byNo: make(map[int]MarkedLine, len(lines))}
	for _, l := range lines {
		idx.byNo[l.Line.LineNo] = l
	}
	return idx
}

func (idx simpleChunkLines) lineNumbers(spans []string) []int {
	var out []int
	for _, span := range spans {
		start, end, ok := parseMetricLineSpan(span)
		if !ok {
			continue
		}
		for n := start; n <= end; n++ {
			out = append(out, n)
		}
	}
	return out
}

func (idx simpleChunkLines) hasNormalLine(spans []string) bool {
	for _, n := range idx.lineNumbers(spans) {
		if l, ok := idx.byNo[n]; ok && normalizeChunkFlag(l.Mark) == "n" {
			return true
		}
	}
	return false
}

// sourceText joins the text of the cited lines, cut to metricSimpleTextMaxRunes. A table line
// gives its header rows and the cited data rows (all rows when none is cited).
func (idx simpleChunkLines) sourceText(spans []string, refs []TableRowRef) string {
	cited := map[int][]string{}
	for _, ref := range refs {
		cited[ref.Line] = append(cited[ref.Line], ref.Rows...)
	}
	var parts []string
	for _, n := range idx.lineNumbers(spans) {
		l, ok := idx.byNo[n]
		if !ok {
			continue
		}
		content := strings.TrimSpace(l.Line.Content)
		if l.Line.LineType == "table" {
			content = tableRowsText(n, content, cited[n])
		}
		if content != "" {
			parts = append(parts, content)
		}
	}
	return truncateRunes(strings.Join(parts, " "), metricSimpleTextMaxRunes)
}

func tableRowsText(lineNo int, content string, rows []string) string {
	rendered := strings.Split(renderTableLineContent(lineNo, content), "\n")
	if len(rows) == 0 {
		return strings.Join(rendered, " / ")
	}
	var keep []string
	for _, r := range rendered {
		label, _, _ := strings.Cut(r, ":")
		_, rowID, _ := strings.Cut(label, "#")
		if strings.HasPrefix(rowID, "h") || containsString(rows, rowID) {
			keep = append(keep, strings.TrimSpace(r))
		}
	}
	return strings.Join(keep, " / ")
}

// section is the nearest heading or table caption at or before the row's first line.
func (idx simpleChunkLines) section(spans []string) string {
	nums := idx.lineNumbers(spans)
	if len(nums) == 0 {
		return ""
	}
	first := nums[0]
	for _, n := range nums {
		first = min(first, n)
	}
	section := ""
	for _, l := range idx.ordered {
		if l.Line.LineNo > first {
			break
		}
		if strings.HasPrefix(l.Line.LineType, "heading") || l.Line.LineType == "table-caption" {
			section = strings.TrimSpace(l.Line.Content)
		}
	}
	return truncateRunes(section, 200)
}

func (idx simpleChunkLines) locationType(spans []string) string {
	if nums := idx.lineNumbers(spans); len(nums) > 0 {
		if l, ok := idx.byNo[nums[0]]; ok && l.Line.LineType == "table" {
			return "table"
		}
	}
	return "text"
}

// metricWarmupGate lets the first call of a run go alone (design D4). The others wait until it
// returned and its input was mostly cached, or until wait after it started.
type metricWarmupGate struct {
	once     sync.Once
	released chan struct{}
	wait     time.Duration
	now      func() time.Time
	sleep    func(ctx context.Context, d time.Duration)
}

func newMetricWarmupGate(now func() time.Time, wait time.Duration) *metricWarmupGate {
	if now == nil {
		now = time.Now
	}
	return &metricWarmupGate{released: make(chan struct{}), wait: wait, now: now, sleep: sleepContext}
}

// do runs call, which reports whether its input was mostly cached. It returns false, without
// calling, when ctx ends while waiting for the first call.
func (g *metricWarmupGate) do(ctx context.Context, call func() (cacheWarm bool)) bool {
	leader := false
	g.once.Do(func() { leader = true })
	if !leader {
		select {
		case <-g.released:
		case <-ctx.Done():
			return false
		}
		call()
		return true
	}
	defer close(g.released)
	start := g.now()
	if !call() {
		g.sleep(ctx, start.Add(g.wait).Sub(g.now()))
	}
	return true
}

func sleepContext(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done():
	}
}
