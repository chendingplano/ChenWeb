package docreviews

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
)

// GoldChunk is one per-chunk review input exactly as the Compliance Review reviewer receives it
// (same line source, chunk size and compact encoding), so gold annotators see what the reviewer sees.
type GoldChunk struct {
	Index     int
	StartLine int
	EndLine   int
	InputJSON string
}

// LoadGoldChunks loads a record's review lines and splits them into the reviewer's chunk inputs.
func LoadGoldChunks(ctx context.Context, db *sql.DB, recordID int64) ([]GoldChunk, error) {
	rec, err := docprocessing.DocMetadataSQLStore{DB: db}.GetInputRecord(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("load record %d: %w", recordID, err)
	}
	lines, err := loadPromptCacheReviewLines(recordID, rec)
	if err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("record %d: review input contains no lines", recordID)
	}
	var out []GoldChunk
	for i, c := range buildChunkInputs(lines, buildDocContextLine(rec), DefaultChunkInputSize) {
		out = append(out, GoldChunk{Index: i, StartLine: c.startLine, EndLine: c.endLine, InputJSON: c.inputJSON})
	}
	return out, nil
}

// GoldLinesText returns the content of lines start..end of a chunk input (the quote the reviewer
// reports as evidence), capped like the reviewer's own evidence.
func GoldLinesText(inputJSON string, start, end int) string {
	return complianceLinesText(inputJSON, start, end)
}

// GoldLine is one line of a chunk input: its document line number and content.
type GoldLine struct {
	Number  int
	Content string
}

// GoldChunkLines returns the lines of a chunk input in order, so a gold annotator can read exactly
// what the reviewer reads. Input that cannot be read yields no lines.
func GoldChunkLines(inputJSON string) []GoldLine {
	lines, _ := parseComplianceInput(inputJSON)
	out := make([]GoldLine, len(lines))
	for i, l := range lines {
		out[i] = GoldLine{Number: l.LineNumber, Content: l.Content}
	}
	return out
}

// GoldDocContext returns the doc_context line of a chunk input, or "" when there is none.
func GoldDocContext(inputJSON string) string {
	var env struct {
		DocContext any `json:"doc_context"`
	}
	if json.Unmarshal([]byte(inputJSON), &env) != nil || env.DocContext == nil {
		return ""
	}
	if s, ok := env.DocContext.(string); ok {
		return s
	}
	b, _ := json.Marshal(env.DocContext)
	return string(b)
}

// SameMD5RecordIDs is the reviewer's duplicate lookup: the record plus every record with the same
// non-empty md5. Gold pooling excludes the same set the reviewer does.
func SameMD5RecordIDs(ctx context.Context, db *sql.DB, recordID int64) ([]int64, error) {
	return sameMD5RecordIDs(ctx, db, recordID)
}
