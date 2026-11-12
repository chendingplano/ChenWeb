package docprocessing

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

// lineForLLM is the JSON representation of a document line sent to LLMs.
type lineForLLM struct {
	Flag       string `json:"flag"`
	LineNumber int    `json:"line_number"`
	PageNumber int    `json:"page_number"`
	LineType   string `json:"line_type"`
	Content    string `json:"content"`
}

// llmLineContent is the content sent to LLMs for one line. Table lines (one whole
// HTML table per line, as MinerU emits them) are rendered as numbered rows
// ("116#r1: cell | cell"), so models can cite individual rows; all other lines pass
// through unchanged. Line numbers are never altered.
func llmLineContent(lineType string, lineNo int, content string) string {
	if lineType != "table" {
		return content
	}
	return renderTableLineContent(lineNo, content)
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
			Content:    llmLineContent(l.LineType, l.LineNumber, l.Content),
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
			Content:    llmLineContent(l.Line.LineType, l.Line.LineNo, l.Line.Content),
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

// RenderLinesCompact is the compact serialization of document lines for LLM input
// (openspec change artifact-review-source-chunk): an optional "Document: <docCtx>"
// line, then one text line per document line, "[<line_number>] <content>", with a
// type tag only where it matters ("[36 h1] …", "[121 table] …"). Lines that hold
// only an image path are left out. Table lines keep llmLineContent's numbered rows
// (one per text line); newlines inside any other line become spaces. Unlike
// canonicalChunkInputText it carries no overlap flags and no page numbers.
func RenderLinesCompact(lines []MarkedLine, docCtx string) string {
	var b strings.Builder
	if docCtx = strings.TrimSpace(docCtx); docCtx != "" {
		b.WriteString("Document: ")
		b.WriteString(docCtx)
		b.WriteByte('\n')
	}
	for _, ml := range lines {
		l := ml.Line
		if strings.HasSuffix(l.LineType, "image") {
			continue
		}
		content := llmLineContent(l.LineType, l.LineNo, l.Content)
		if l.LineType != "table" {
			content = compactNewlines.Replace(content)
		}
		b.WriteByte('[')
		b.WriteString(strconv.Itoa(l.LineNo))
		if tag := compactLineTag(l.LineType); tag != "" {
			b.WriteByte(' ')
			b.WriteString(tag)
		}
		b.WriteString("] ")
		b.WriteString(content)
		b.WriteByte('\n')
	}
	return strings.TrimSuffix(b.String(), "\n")
}

var compactNewlines = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ")

// compactLineTag is the RenderLinesCompact type tag: none for running text,
// hN for headings, short names for captions and equations, the raw type otherwise.
func compactLineTag(lineType string) string {
	switch {
	case lineType == "", lineType == "paragraph", lineType == "list-item", lineType == "text":
		return ""
	case lineType == "heading":
		return "h"
	case strings.HasPrefix(lineType, "heading-"):
		return "h" + strings.TrimPrefix(lineType, "heading-")
	case lineType == "table-caption":
		return "caption"
	case lineType == "equation":
		return "eq"
	default:
		return lineType
	}
}

// rawLinesToJSON serialises []Line as a JSON array for LLM input, using flag "n".
// Image lines are skipped.
// compactLinesHeader is the first row of compact line input; it names the columns of every
// following row, so prompts can keep referring to the fields by name.
var compactLinesHeader = []string{"flag", "line_number", "page_number", "line_type", "content"}

// rawLinesToCompactJSON is rawLinesToJSON for reviewers: the same lines and fields, but as a
// JSON array whose first row names the columns and whose other rows are bare value arrays,
// which drops the per-line keys (about half the input tokens). Topic extraction and the other
// processors keep rawLinesToJSON.
func rawLinesToCompactJSON(lines []Line) string {
	rows := make([][]any, 0, len(lines)+1)
	header := make([]any, len(compactLinesHeader))
	for i, h := range compactLinesHeader {
		header[i] = h
	}
	rows = append(rows, header)
	for _, l := range lines {
		if l.LineType == "image" {
			continue
		}
		rows = append(rows, []any{"n", l.LineNo, l.PageNo, l.LineType, llmLineContent(l.LineType, l.LineNo, l.Content)})
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(rows)
	return strings.TrimSpace(buf.String())
}

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
			Content:    llmLineContent(l.LineType, l.LineNo, l.Content),
		})
	}
	bs, _ := json.Marshal(out)
	return string(bs)
}
