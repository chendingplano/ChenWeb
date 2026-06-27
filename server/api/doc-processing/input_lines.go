package docprocessing

import "encoding/json"

// lineForLLM is the JSON representation of a document line sent to LLMs.
type lineForLLM struct {
	Flag       string `json:"flag"`
	LineNumber int    `json:"line_number"`
	PageNumber int    `json:"page_number"`
	LineType   string `json:"line_type"`
	Content    string `json:"content"`
}

// blockLinesToJSON serialises []BlockLine as a JSON array for LLM input.
func blockLinesToJSON(lines []BlockLine) string {
	out := make([]lineForLLM, len(lines))
	for i, l := range lines {
		out[i] = lineForLLM{
			Flag:       l.Flag,
			LineNumber: l.LineNumber,
			PageNumber: l.PageNumber,
			LineType:   l.LineType,
			Content:    l.Content,
		}
	}
	bs, _ := json.Marshal(out)
	return string(bs)
}

// markedLinesToJSON serialises []MarkedLine as a JSON array for LLM input.
// Image lines are skipped (same behaviour as the previous tab-format path).
func markedLinesToJSON(lines []MarkedLine) string {
	out := make([]lineForLLM, 0, len(lines))
	for _, l := range lines {
		if l.Line.LineType == "image" {
			continue
		}
		out = append(out, lineForLLM{
			Flag:       normalizeChunkFlag(l.Mark),
			LineNumber: l.Line.LineNo,
			PageNumber: l.Line.PageNo,
			LineType:   l.Line.LineType,
			Content:    l.Line.Content,
		})
	}
	bs, _ := json.Marshal(out)
	return string(bs)
}

// canonicalChunkInputText is the single, canonical serialization of a chunk for LLM input,
// shared by all chunk-based doc processors so the same chunk produces a byte-identical
// prefix across processors (enabling DeepSeek cross-processor prompt-cache reuse).
//
// It is the document/window text only — no per-processor schema or task text. Processors put
// their schema/task instructions in the prompt (the <TASK> section under document-first
// layout), never prepended here. docCtx is buildDocContextLine(rec); pass it consistently so
// every processor's envelope matches. See ADR 2026062701 §Phase 2.3 and +CAPSULE §6.2.
func canonicalChunkInputText(lines []MarkedLine, docCtx string) string {
	return wrapLinesWithDocContext(markedLinesToJSON(lines), docCtx)
}

// rawLinesToJSON serialises []Line as a JSON array for LLM input, using flag "n".
// Image lines are skipped.
func rawLinesToJSON(lines []Line) string {
	out := make([]lineForLLM, 0, len(lines))
	for _, l := range lines {
		if l.LineType == "image" {
			continue
		}
		out = append(out, lineForLLM{
			Flag:       "n",
			LineNumber: l.LineNo,
			PageNumber: l.PageNo,
			LineType:   l.LineType,
			Content:    l.Content,
		})
	}
	bs, _ := json.Marshal(out)
	return string(bs)
}
