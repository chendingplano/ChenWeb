package docprocessing

// extract_references (openspec change extract-references-processor, devdoc 2026101002): a Phase B
// processor at the lower layer. It reads only the document's chunks and uses no output of any
// other extractor. Per chunk the model labels every reference (reference model, devdoc §3); at
// finalize the whole document is assembled: identifiers are parsed in code, normativity is derived
// from the formal lists, mentions and the catalog are stored (R1), the document is attached to its
// own resource, holdings are resolved (R2), and the registered artifact families resolve their
// references (step 5 and the holding event of devdoc §5.4).

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

// refOwnIdentifierChunks is how many leading chunks may report the document's own identifier.
const refOwnIdentifierChunks = 3

type ReferencesProcessor struct {
	InputStore        DocMetadataStore
	DB                *sql.DB
	Extractor         LLMJSONExtractor
	Logger            ApiTypes.JimoLogger
	ModelRef          string
	ModelCfgPath      string
	ModelErr          error
	ModelName         string
	FallbackModelName string
	FallbackModelErr  error
	FallbackExtractor LLMJSONExtractor
	PromptText        string
	PromptRef         string
	PromptPath        string
	PromptErr         error

	batchRecordID int64
	batchChunks   []Chunk
	batchDocCtx   string
	batchStart    time.Time
	mu            sync.Mutex
	mentions      []RefMention
	ownIDs        map[int]string // chunk index -> document_identifier reported in that chunk
}

func NewReferencesProcessor(inputStore DocMetadataStore, db *sql.DB, e LLMJSONExtractor, logger ApiTypes.JimoLogger) *ReferencesProcessor {
	if logger == nil {
		logger = loggerutil.CreateDefaultLogger("MID_26101120")
	}
	promptText, promptRef, promptPath, promptErr := loadProductPromptFromEnvKeys(
		[]string{"EXTRACT_REFERENCES_PROMPT"}, "prompt-extract-references-v2.md")
	modelRef, modelCfgPath, modelCfg, modelErr := loadModelConfigFromEnvKeys(
		[]string{"EXTRACT_REFERENCES_MODEL_NAME"}, "MODEL_DEF_FILE")
	fallbackRef := strings.TrimSpace(os.Getenv("EXTRACT_REFERENCES_MODEL_FALLBACK"))
	settings, configErr := LoadExtractorConfig("references")
	if configErr != nil {
		promptErr = configErr
	} else {
		if ref, ok := settings["EXTRACT_REFERENCES_PROMPT"]; ok {
			promptText, promptRef, promptPath, promptErr = loadPromptByRef(ref)
		}
		if ref, ok := settings["EXTRACT_REFERENCES_MODEL_FALLBACK"]; ok {
			fallbackRef = strings.TrimSpace(ref)
		}
		if ref, ok := settings["EXTRACT_REFERENCES_MODEL_NAME"]; ok {
			modelRef, modelCfgPath, modelCfg, modelErr = loadModelConfigByRef(ref, "MODEL_DEF_FILE")
		}
	}
	var fallbackCfg structureModelConfig
	var fallbackErr error
	if fallbackRef != "" {
		_, _, fallbackCfg, fallbackErr = loadModelConfigByRef(fallbackRef, "MODEL_DEF_FILE")
	}
	// Only an explicit no-reasoning policy overrides the model's thinking config.
	if strings.EqualFold(strings.TrimSpace(os.Getenv("MODEL_DEFAULT_REASONING_POLICY")), "no-reasoning") {
		modelCfg = forceDisableThinking(modelCfg)
		fallbackCfg = forceDisableThinking(fallbackCfg)
	}
	fallbackExtractor := e
	if client, ok := e.(*llmclients.OpenAIJSONClient); ok && fallbackRef != "" && fallbackErr == nil {
		timeoutSec := fallbackCfg.TimeoutSec
		if timeoutSec <= 0 {
			timeoutSec = 60
		}
		fallbackExtractor, fallbackErr = llmclients.NewOpenAIJSONClientFromConfig(llmclients.OpenAIJSONClientConfig{
			Host: fallbackCfg.Host, BaseURL: fallbackCfg.BaseURL, APIKey: fallbackCfg.APIKey,
			ModelName: fallbackCfg.ModelName, ProfileName: fallbackCfg.ProfileName, ThinkingType: fallbackCfg.ThinkingType,
			TimeoutSec: timeoutSec, MaxOutputTokens: fallbackCfg.MaxOutputTokens,
			EmbeddingDimensions: client.EmbeddingDimensions,
		}, logger)
	}
	applyStructureModelConfigToExtractor(e, modelCfg)
	return &ReferencesProcessor{InputStore: inputStore, DB: db, Extractor: e, Logger: logger,
		ModelRef: modelRef, ModelCfgPath: modelCfgPath, ModelErr: modelErr, ModelName: modelCfg.ModelName,
		FallbackModelName: fallbackCfg.ModelName, FallbackModelErr: fallbackErr, FallbackExtractor: fallbackExtractor,
		PromptText: promptText, PromptRef: promptRef, PromptPath: promptPath, PromptErr: promptErr}
}

func (p *ReferencesProcessor) Name() string { return "extract_references" }

func (p *ReferencesProcessor) WarmPromptBeforeRemainingChunks() bool { return true }

func (p *ReferencesProcessor) HandleEvent(context.Context, []byte) error {
	return fmt.Errorf("%s requires chunk batching", p.Name())
}

func (p *ReferencesProcessor) InitChunkBatch(_ context.Context, recordID int64, chunks []Chunk, docCtx string) error {
	switch {
	case p.Extractor == nil || p.DB == nil || p.InputStore == nil:
		return fmt.Errorf("%s is not configured", p.Name())
	case p.PromptErr != nil:
		return fmt.Errorf("%s prompt %s: %w", p.Name(), p.PromptRef, p.PromptErr)
	case p.ModelErr != nil || p.ModelName == "":
		return fmt.Errorf("%s model (EXTRACT_REFERENCES_MODEL_NAME=%q): %v", p.Name(), p.ModelRef, p.ModelErr)
	}
	p.batchRecordID, p.batchChunks, p.batchDocCtx, p.batchStart = recordID, chunks, docCtx, time.Now()
	p.mentions, p.ownIDs = nil, map[int]string{}
	return nil
}

func (p *ReferencesProcessor) referenceChunkInputText(lines []MarkedLine, docCtx string) string {
	if filepath.Base(p.PromptRef) == "prompt-extract-references-v2.md" {
		return wrapLinesWithDocContext(markedLinesToCompactJSON(lines), docCtx)
	}
	return canonicalChunkInputText(lines, docCtx)
}

func (p *ReferencesProcessor) ProcessChunk(ctx context.Context, idx int) error {
	if idx < 0 || idx >= len(p.batchChunks) {
		return fmt.Errorf("%s chunk index out of range", p.Name())
	}
	start := time.Now()
	ctx, recovery := startLLMFallback(withLLMRecordID(ctx, p.batchRecordID))
	defer recovery.flush()
	in := newLLMJSONInput(ctx, p.Name(), p.PromptText, p.ModelName,
		p.referenceChunkInputText(p.batchChunks[idx].Lines, p.batchDocCtx), p.Name(), "MID_26101121")
	// The extraction instructions are large and identical across chunks, while each
	// chunk's document input changes. Keep the prompt first so its stable prefix can
	// be reused by the provider's prompt cache.
	in.DocumentFirst = false
	in.Metadata = map[string]any{"chunk_index": idx, "chunk_total": len(p.batchChunks)}
	raw, err := retryLLMTimeout(ctx, func() (map[string]any, error) {
		return extractReferenceChunk(ctx, p.Extractor, in)
	})
	extractor := p.Extractor
	if err != nil && ctx.Err() == nil {
		if p.FallbackModelErr != nil {
			err = fmt.Errorf("reference fallback model configuration failed: %w; primary failed: %w", p.FallbackModelErr, err)
		} else if p.FallbackModelName != "" && p.FallbackExtractor != nil {
			primaryErr := err
			in.ModelName = p.FallbackModelName
			extractor = p.FallbackExtractor
			raw, err = extractReferenceChunk(ctx, extractor, in)
			if err == nil {
				recovery.recordChunk(p.DB, p.Logger, p.Name(), p.PromptRef, p.ModelName, in.ModelName, primaryErr, &idx)
			} else {
				err = fmt.Errorf("reference primary failed: %w; fallback failed: %w", primaryErr, err)
			}
		}
	}
	if err != nil {
		p.Logger.Warn("extract references chunk failed", "record_id", p.batchRecordID, "chunk", idx,
			"ms_used", time.Since(start).Milliseconds(), "error", err)
		return err
	}
	ms := parseRefMentions(raw)
	hit, miss := cacheTokenCounts(extractor)
	p.Logger.Info("extract references chunk done", "record_id", p.batchRecordID, "chunk", idx,
		"total", len(p.batchChunks), "references", len(ms), "cache_hit", hit, "cache_miss", miss,
		"model_name", in.ModelName, "prompt_name", p.PromptRef, "ms_used", time.Since(start).Milliseconds())
	p.mu.Lock()
	p.mentions = append(p.mentions, ms...)
	if idx < refOwnIdentifierChunks {
		if own := strings.TrimSpace(asString(raw["document_identifier"])); own != "" {
			p.ownIDs[idx] = own
		}
	}
	p.mu.Unlock()
	return nil
}

func extractReferenceChunk(ctx context.Context, extractor LLMJSONExtractor, in llmclients.JSONExtractionInput) (map[string]any, error) {
	if structured, ok := extractor.(LLMStructuredJSONExtractor); ok {
		contract := newDocProcessingContract("chenweb_reference_extraction", map[string]any{
			"type": "object", "required": []string{"references"},
			"properties": map[string]any{"references": schemaArray()},
		})
		var bestPartial map[string]any
		bestCount := -1
		contract.CheckParsed = func(payload map[string]any) error {
			var problems []error
			items, _ := payload["references"].([]any)
			validCount := 0
			for i, item := range items {
				row, ok := item.(map[string]any)
				if !ok {
					problems = append(problems, fmt.Errorf("Reference %d must be an object with citation text (cited_raw) and source lines (source_line_spans)", i+1))
					continue
				}
				missingText := strings.TrimSpace(asString(row["cited_raw"])) == ""
				missingLines := len(refSpans(row["source_line_spans"])) == 0
				if missingText {
					problems = append(problems, fmt.Errorf("Reference %d has no citation text. Set cited_raw to the citation as written in the document", i+1))
				}
				if missingLines {
					problems = append(problems, fmt.Errorf("Reference %d has no source lines. Set source_line_spans to an array of the supplied line numbers containing the citation", i+1))
				}
				if !missingText && !missingLines {
					validCount++
				}
			}
			if len(problems) > 0 && validCount > bestCount {
				bestPartial, bestCount = payload, validCount
			}
			return errors.Join(problems...)
		}
		result, err := structured.ExtractStructuredJSON(ctx, in, contract)
		if err != nil {
			if bestPartial != nil && ctx.Err() == nil && errors.Is(err, llmclients.ErrStructuredOutputRetriesExhausted) {
				// Preserve usable references when the model runs out of correction attempts.
				return bestPartial, nil
			}
			return nil, err
		}
		return result.Parsed, nil
	}
	payload, err := extractor.ExtractJSON(ctx, in)
	if err != nil {
		return nil, err
	}
	if _, ok := payload["references"].([]any); !ok {
		return nil, fmt.Errorf("reference extraction response must contain a references array")
	}
	return payload, nil
}

func (p *ReferencesProcessor) FinalizeChunkBatch(ctx context.Context) error {
	if isCtxStopped(ctx) {
		return ErrPipelineStopped
	}
	recordID := p.batchRecordID
	rec, err := p.InputStore.GetInputRecord(ctx, recordID)
	if err != nil {
		return fmt.Errorf("(MID_26101122) %s load record %d: %w", p.Name(), recordID, err)
	}
	var lines []Line
	for _, chunk := range p.batchChunks {
		for _, marked := range chunk.Lines {
			lines = append(lines, marked.Line)
		}
	}
	applyReferenceTableRows(lines, p.mentions, p.Logger, recordID)
	ms := assembleRefMentions(recordID, p.mentions)
	store := ReferenceSQLStore{DB: p.DB}
	if err := store.ReplaceRecordReferences(ctx, recordID, eventIDFromContext(ctx), ms); err != nil {
		return fmt.Errorf("(MID_26101123) %s store references: %w", p.Name(), err)
	}

	ownCands := []string{rec.DocumentNumber}
	for i := 0; i < refOwnIdentifierChunks; i++ {
		ownCands = append(ownCands, p.ownIDs[i])
	}
	knownIDs := len(ownCands)
	ownCands = append(ownCands, rec.Title, rec.FileName, rec.StagingFilename)
	var ownResource int64
	if own, ok := refOwnIdentifier(knownIDs, ownCands...); ok {
		if ownResource, err = store.AttachOwnDocument(ctx, recordID, own, rec.Title, refOwnTargetType(own.Kind)); err != nil {
			return fmt.Errorf("(MID_26101124) %s attach own document: %w", p.Name(), err)
		}
	}
	if err := store.ResolveHoldings(ctx, []int64{recordID}, 0); err != nil {
		return fmt.Errorf("(MID_26101125) %s resolve holdings: %w", p.Name(), err)
	}

	// Step 5. Resolution failures never fail the extraction: the mentions are stored and the next
	// event on either side resolves them again.
	outgoing, err := ResolveOutgoing(ctx, p.DB, recordID)
	if err != nil {
		p.Logger.Warn("step 5 resolution failed", "record_id", recordID, "error", err)
	}
	incoming := ResolveSummary{}
	if ownResource > 0 {
		incoming, err = p.resolveHoldingEvent(ctx, store, recordID, ownResource)
		if err != nil {
			p.Logger.Warn("holding-event resolution failed", "record_id", recordID, "error", err)
		}
	}
	p.Logger.Info("extract references saved", "record_id", recordID, "references", len(ms),
		"by_normativity", countRefBy(ms, func(m RefMention) string { return m.Normativity }),
		"by_target_type", countRefBy(ms, func(m RefMention) string { return m.TargetType }),
		"by_content", countRefBy(ms, func(m RefMention) string { return m.TargetContent }),
		"own_resource_id", ownResource, "outgoing", outgoing, "incoming", incoming,
		"ms_used", time.Since(p.batchStart).Milliseconds())
	return nil
}

// resolveHoldingEvent: this record is now attached to its resource, so other records' mentions of
// that resource may become held (devdoc §5.4, second event).
func (p *ReferencesProcessor) resolveHoldingEvent(ctx context.Context, store ReferenceSQLStore, recordID, resourceID int64) (ResolveSummary, error) {
	sum := ResolveSummary{}
	citing, err := store.CitingRecordsOfResource(ctx, resourceID, recordID)
	if err != nil || len(citing) == 0 {
		return sum, err
	}
	if err := store.ResolveHoldings(ctx, citing, resourceID); err != nil {
		return sum, err
	}
	for _, citingID := range citing {
		outcome, err := resolveOutgoing(ctx, p.DB, citingID, resourceID)
		sum.merge(outcome)
		if err != nil {
			return sum, err
		}
	}
	return sum, nil
}

// parseRefMentions reads one chunk's model output.
func parseRefMentions(payload map[string]any) []RefMention {
	raw, _ := payload["references"].([]any)
	out := make([]RefMention, 0, len(raw))
	for _, item := range raw {
		r, ok := item.(map[string]any)
		if !ok {
			continue
		}
		m := RefMention{
			CitedRaw:        strings.TrimSpace(asString(r["cited_raw"])),
			TargetTitle:     strings.TrimSpace(asString(r["target_title"])),
			TargetVersion:   strings.TrimSpace(asString(r["target_version"])),
			TargetType:      asString(r["target_type"]),
			TargetSubtype:   asString(r["target_subtype"]),
			Mechanism:       asString(r["mechanism"]),
			Relationships:   asStringSlice(r["relationships"]),
			TargetScope:     asString(r["target_scope"]),
			TargetLocator:   strings.TrimSpace(asString(r["target_locator"])),
			TargetContent:   asString(r["target_content"]),
			InRequirement:   refBool(r["in_requirement"]),
			RefForm:         strings.ToLower(strings.TrimSpace(asString(r["ref_form"]))),
			ListKind:        strings.ToLower(strings.TrimSpace(asString(r["list_kind"]))),
			ListLabel:       strings.TrimSpace(asString(r["list_label"])),
			MetricName:      strings.TrimSpace(asString(r["metric_name"])),
			MetricNameEn:    strings.TrimSpace(asString(r["metric_name_en"])),
			MetricSubject:   strings.TrimSpace(asString(r["metric_subject"])),
			MetricSubjectEn: strings.TrimSpace(asString(r["metric_subject_en"])),
			Condition:       strings.TrimSpace(asString(r["condition"])),
			Section:         strings.TrimSpace(asString(r["section"])),
			SourceLineSpans: refSpans(r["source_line_spans"]),
			SourceTableRows: tableRowRefsFromValue(r["source_table_rows"]),
			Evidence:        strings.TrimSpace(asString(r["evidence"])),
			Confidence:      refFloat(r["confidence"]),
		}
		if m.CitedRaw == "" || len(m.SourceLineSpans) == 0 {
			continue
		}
		out = append(out, m)
	}
	return out
}

// assembleRefMentions turns the chunks' raw mentions into one document's references: duplicates
// from overlapping chunks are removed, labels normalized, identifiers parsed, normativity derived,
// and ids and stable keys assigned in reading order.
func assembleRefMentions(recordID int64, raw []RefMention) []RefMention {
	seen := map[string]bool{}
	var ms []RefMention
	for _, m := range raw {
		normalizeRefMention(&m)
		m.InputRecordID = recordID
		m.Identifiers = parseRefIdentifiersForMention(m.Mechanism, m.CitedRaw)
		// Overlapping chunks report the same occurrence again, sometimes with different raw text
		// ("GB/T755" vs "GB/T755 旋转电机 定额和性能"): a list line holds one entry, and an in-text
		// citation is identified by what it cites.
		what := refNormKey(m.CitedRaw)
		if key, _, _, ok := refIdentityKey(m.Identifiers, ""); ok {
			what = key
		}
		location := m.SourceLineSpans[0] + refTableRowLocation(m.SourceTableRows)
		k := strings.Join([]string{m.RefForm, location, what, m.TargetContent, refNormKey(m.MetricName)}, "|")
		if m.RefForm == refFormListed {
			k = m.RefForm + "|" + location
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		if m.TargetContent != "criterion" {
			m.MetricName, m.MetricNameEn, m.MetricSubject, m.MetricSubjectEn, m.Condition = "", "", "", "", ""
		}
		ms = append(ms, m)
	}
	sort.SliceStable(ms, func(i, j int) bool { return refFirstLine(ms[i]) < refFirstLine(ms[j]) })
	deriveRefNormativity(ms)
	for i := range ms {
		ms[i].RefID = fmt.Sprintf("%d_ref_%d", recordID, i+1)
		ms[i].MentionKey = refMentionKey(ms[i])
	}
	return ms
}

func refFirstLine(m RefMention) int {
	if len(m.SourceLineSpans) == 0 {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(strings.SplitN(m.SourceLineSpans[0], "-", 2)[0]))
	return n
}

func refOwnTargetType(kind string) string {
	switch kind {
	case "standard", "rfc":
		return "standard"
	case "legal":
		return "legal"
	case "doi", "arxiv":
		return "publication"
	}
	return refDefaultTargetType
}

func refSpans(v any) []string {
	items, _ := v.([]any)
	var out []string
	for _, it := range items {
		s := strings.TrimSpace(asString(it))
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func refBool(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return strings.EqualFold(strings.TrimSpace(x), "true")
	}
	return false
}

func refFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f
	}
	return 0
}

func countRefBy(ms []RefMention, key func(RefMention) string) map[string]int {
	out := map[string]int{}
	for _, m := range ms {
		out[key(m)]++
	}
	return out
}
