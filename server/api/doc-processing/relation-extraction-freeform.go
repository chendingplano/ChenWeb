package docprocessing

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Free-form, entity-agnostic relation extraction (ADR 2026061702 DR2). Relations
// are extracted per CHUNK — the same units entity extraction uses — so the two
// passes are symmetric and each call is small and focused. Windows carry NO
// entity roster: just the chunk's numbered source lines. The model returns
// relations with their own subject_lines/predicate_lines/object_lines; endpoint
// linking happens later in Phase C (linkRecordEndpoints). This lets relation
// extraction run in a processor independent of (and parallel to) entity extraction.
//
// Trade-off (same as entity extraction): a relation whose endpoints fall in two
// different chunks is not captured here. Chunk overlap (the "o"-marked lines)
// catches boundary cases; truly long-range relations are out of scope and are
// recovered, at the entity level, by scheduled reconciliation (ADR 2026061701).

// buildChunkRelationInputJSON renders a chunk's lines as the v2 relation prompt
// input: the JSON line array the prompt documents. The chunk's own overlap
// marker (MarkedLine.Mark == "o") maps to the v2 "o" flag (context only); regular
// lines are "n".
func buildChunkRelationInputJSON(lines []MarkedLine) string {
	type lineObj struct {
		Flag       string `json:"flag"`
		LineNumber int    `json:"line_number"`
		PageNumber int    `json:"page_number"`
		LineType   string `json:"line_type"`
		Content    string `json:"content"`
	}
	objs := make([]lineObj, 0, len(lines))
	for _, ml := range lines {
		flag := "n"
		if strings.EqualFold(strings.TrimSpace(ml.Mark), "o") {
			flag = "o"
		}
		lt := strings.TrimSpace(ml.Line.LineType)
		if lt == "" {
			lt = "text"
		}
		objs = append(objs, lineObj{
			Flag:       flag,
			LineNumber: ml.Line.LineNo,
			PageNumber: ml.Line.PageNo,
			LineType:   lt,
			Content:    ml.Line.Content,
		})
	}
	bs, err := json.Marshal(objs)
	if err != nil {
		return "[]"
	}
	return string(bs)
}

// extractFreeformRelations runs one entity-agnostic relation call per chunk,
// concurrently, and returns the normalized relations. Failed chunks are skipped
// (not fatal), matching the entity-extraction path.
func (p *EntityRelationProcessor) extractFreeformRelations(
	ctx context.Context,
	recordID int64,
	chunks []Chunk,
	docCtx string,
) ([]map[string]any, string, error) {
	results, runErr := runConcurrent(ctx, p.ExtractEntityRelationMaxTasks, len(chunks),
		func(workerCtx context.Context, i int) (relationWindowResult, error) {
			if isCtxStopped(workerCtx) {
				return relationWindowResult{}, ErrPipelineStopped
			}
			return p.processFreeformRelationChunk(workerCtx, recordID, i, len(chunks), chunks[i], docCtx), nil
		},
	)
	if runErr != nil {
		if isCtxStopped(ctx) {
			return nil, "", ErrPipelineStopped
		}
		return nil, "", runErr
	}

	relations := make([]map[string]any, 0)
	language := ""
	for _, r := range results {
		if r.Failed {
			continue
		}
		if language == "" && r.Language != "" {
			language = r.Language
		}
		relations = append(relations, r.Relations...)
	}
	return relations, language, nil
}

func (p *EntityRelationProcessor) processFreeformRelationChunk(
	ctx context.Context,
	recordID int64,
	idx, totalChunks int,
	chunk Chunk,
	docCtx string,
) relationWindowResult {
	// Canonical chunk serialization (markedLinesToJSON + doc-context envelope) so this chunk's
	// InputText is byte-identical to the entities pass (and other chunk processors), enabling
	// DeepSeek cross-processor cache reuse. See ADR 2026062701 §Phase 2.3.
	inputText := canonicalChunkInputText(chunk.Lines, docCtx)
	callStart := p.Now()
	p.Logger.Info("extract relation start",
		"record_id", recordID, "chunk_idx", idx, "seq_no", chunk.SeqNo,
		"lines", len(chunk.Lines),
		"relation_prompt", p.RelationPromptRef,
		"relation_prompt_len", len(p.RelationPromptText),
		"input_len", len(inputText),
	)
	callID := fmt.Sprintf("%s_relchunk%d", eventIDFromContext(ctx), idx)
	payload, modelName, err := p.extractRelationsWithFallback(ctx, inputText)

	var relations []map[string]any
	var detectedLanguage string
	if err == nil && payload != nil {
		detectedLanguage = strings.TrimSpace(asString(payload["language"]))
		relations = normalizeRelationRows(payload["relations"], 0)
		cacheHit, cacheMiss := cacheTokenCounts(p.Extractor)
		p.Logger.Info("extract relation end  ",
			"record_id", recordID, "chunk_idx", idx,
			"relations", len(relations),
			"ms_used", time.Since(callStart).Milliseconds(),
			"cache_hit", cacheHit,
			"cache_miss", cacheMiss,
		)
	}

	p.logLLMCall(ctx, callID, "extract_relations", 2,
		[]string{strings.TrimSpace(modelName)}, p.RelationPromptRef,
		payload, err, callStart, p.Now(),
		recordID, idx, totalChunks,
		0, 0, len(relations), 0)

	if err != nil {
		p.Logger.Warn("free-form relation extraction failed for chunk; skipping",
			"record_id", recordID, "chunk_idx", idx, "error", err)
		return relationWindowResult{Failed: true}
	}
	return relationWindowResult{
		Relations: relations,
		Language:  detectedLanguage,
		ModelName: strings.TrimSpace(modelName),
	}
}
