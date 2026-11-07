package docprocessing

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

func (p *ProvisionsProcessor) reviewProvisionPayload(ctx context.Context, inputText, promptText string, payload map[string]any, model string, unit, total int) (map[string]any, error) {
	// Only v7 runs the second pass; v8 carries the audit-only rules in one pass.
	if filepath.Base(p.PromptRef) != "prompt-extract-provisions-v7.md" || !provisionNeedsReview(inputText, payload) {
		return payload, nil
	}

	if p.ReviewPromptErr != nil || strings.TrimSpace(p.ReviewPromptText) == "" {
		return nil, fmt.Errorf("(MID_20261107_846) provisions review prompt unavailable: %v", p.ReviewPromptErr)
	}
	body, err := json.Marshal(map[string]any{"source": inputText, "draft": payload, "candidate_units": provisionReviewUnits(inputText)})
	if err != nil {
		return nil, fmt.Errorf("(MID_20261107_847) encode provisions review: %w", err)
	}
	// A separate processor value supplies review provenance without mutating the
	// shared processor or copying its mutex while other chunks are running.
	reviewer := &ProvisionsProcessor{PromptRef: p.ReviewPromptRef, forceV2: true}
	start := time.Now()
	reviewed, err := reviewer.extractProvisionPayloadFromText(ctx, string(body), promptText+"\n\n"+p.ReviewPromptText,
		model, p.callExtractor(model == p.FallbackModelName && p.FallbackModelName != ""))
	count := 0
	if reviewed != nil {
		count = len(reviewed["provisions"].([]any))
	}
	if p.Logger != nil {
		p.logLLMCall(ctx, fmt.Sprintf("%s_provision_review_%d", eventIDFromContext(ctx), unit), "review_provisions",
			[]string{model}, p.ReviewPromptRef, reviewed, err, start, time.Now(), llmRecordIDFromContext(ctx), unit, total, 0, count)
	}
	if err != nil {
		return nil, fmt.Errorf("(MID_20261107_848) review provisions unit %d: %w", unit, err)
	}
	reviewed["review_calls"] = 1 // internal accounting, not a wire/storage field
	return reviewed, nil
}

func provisionNeedsReview(input string, payload map[string]any) bool {
	rows, _ := payload["provisions"].([]any)
	for _, value := range rows {
		row, _ := value.(map[string]any)
		text := strings.TrimRight(asString(row["provision"]), "。.!?！？ \n\t")
		if strings.ContainsAny(text, "。.!?！？,，;；") {
			return true
		}
	}
	if strings.Contains(input, `"line_type":"table"`) || strings.Contains(input, `"line_type":"list-item"`) {
		return true
	}
	// A suspiciously empty extraction still needs a source-first coverage check.
	if len(rows) == 0 {
		lower := strings.ToLower(input)
		for _, cue := range []string{"应", "必须", "不得", "shall", "must", "should", "may ", "称取", "调节", "收集"} {
			if strings.Contains(lower, cue) {
				return true
			}
		}
	}
	return false
}

// Clause boundaries are review hints, never provision boundaries. The reviewer
// must join fragments about the same subject and preserve source conditions.
func provisionReviewUnits(input string) []map[string]any {
	if _, after, ok := strings.Cut(input, "Input block lines (JSON array):"); ok {
		input = strings.TrimSpace(after)
	}
	var lines []lineForLLM
	if json.Unmarshal([]byte(input), &lines) != nil {
		var envelope struct {
			Lines []lineForLLM `json:"lines"`
		}
		if json.Unmarshal([]byte(input), &envelope) != nil {
			return nil
		}
		lines = envelope.Lines
	}
	var units []map[string]any
	for _, line := range lines {
		if line.Flag == "o" || strings.HasPrefix(line.LineType, "heading") {
			continue
		}
		var clauses []string
		var clause strings.Builder
		for _, char := range line.Content {
			clause.WriteRune(char)
			if strings.ContainsRune("。！？，,；;", char) {
				clauses = append(clauses, clause.String())
				clause.Reset()
			}
		}
		if clause.Len() > 0 {
			clauses = append(clauses, clause.String())
		}
		units = append(units, map[string]any{"line": line.LineNumber, "line_type": line.LineType, "clauses": clauses})
	}
	return units
}

func provisionPromptUsesV2(ref string) bool {
	switch filepath.Base(ref) {
	case "prompt-extract-provisions-v6.md", "prompt-extract-provisions-v7.md", "prompt-extract-provisions-v8.md", "prompt-audit-provision-extraction-v1.md":
		return true
	default:
		return false
	}
}

func (p *ProvisionsProcessor) normalizeChunkProvisions(items []any, chunk Chunk) []map[string]any {
	owned := make(map[int]bool, len(chunk.Lines))
	for _, line := range chunk.Lines {
		if line.Mark != "o" {
			owned[line.Line.LineNo] = true
		}
	}
	return p.filterOwnedProvisions(normalizeProvisionList(items, chunkLineToPage(chunk), chunkLineText(chunk)), owned, chunk.SeqNo)
}

func (p *ProvisionsProcessor) normalizeBlockProvisions(items []any, block Block) []map[string]any {
	owned := make(map[int]bool, len(block.Lines))
	for _, line := range block.Lines {
		if line.Flag != "o" {
			owned[line.LineNumber] = true
		}
	}
	return p.filterOwnedProvisions(normalizeProvisionList(items, blockLineToPage(block), blockLineText(block)), owned, block.Index)
}

func (p *ProvisionsProcessor) filterOwnedProvisions(rows []map[string]any, owned map[int]bool, unit int) []map[string]any {
	kept := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		ownsEvidence := false
		for _, span := range toStringSlice(row["source_line_spans"]) {
			for _, line := range expandLineSpan(span) {
				ownsEvidence = ownsEvidence || owned[line]
			}
		}
		if ownsEvidence {
			kept = append(kept, row)
		}
	}
	if p.Logger != nil && len(kept) != len(rows) {
		p.Logger.Info("discarded provisions without owned source evidence", "unit", unit, "discarded", len(rows)-len(kept))
	}
	return kept
}

func (p *ProvisionsProcessor) deduplicateProvisions(rows []map[string]any) []map[string]any {
	seen := make(map[string]bool, len(rows))
	kept := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		text := strings.TrimRight(strings.Join(strings.Fields(asString(row["provision"])), " "), "。.;；")
		spans := normalizeProvisionSourceLineSpans(row["source_line_spans"], nil)
		key := strings.Join(spans, ",") + "\x00" + text
		// Missing text/evidence is not a basis for declaring two rows duplicates.
		if text != "" && len(spans) > 0 {
			if seen[key] {
				continue
			}
			seen[key] = true
		}
		kept = append(kept, row)
	}
	if p.Logger != nil && len(kept) != len(rows) {
		p.Logger.Info("discarded duplicate provisions", "discarded", len(rows)-len(kept))
	}
	return kept
}
