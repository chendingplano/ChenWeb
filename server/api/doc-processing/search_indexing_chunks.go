package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/kbsearch"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

func buildChunkRegistryRows(rec InputRecord, chunks []Chunk) ([]kbsearch.RegistryRow, error) {
	rows := make([]kbsearch.RegistryRow, 0, len(chunks))
	for _, chunk := range chunks {
		text := RenderLinesCompact(chunk.Lines, "")
		if strings.TrimSpace(text) == "" {
			continue
		}
		regular := []string{}
		overlap := []string{}
		regularRefs := []map[string]int{}
		overlapRefs := []map[string]int{}
		for _, ml := range chunk.Lines {
			span := strconv.Itoa(ml.Line.LineNo)
			ref := map[string]int{"page_number": ml.Line.PageNo, "line_number": ml.Line.LineNo}
			if strings.EqualFold(ml.Mark, "o") {
				overlap = append(overlap, span)
				overlapRefs = append(overlapRefs, ref)
			} else {
				regular = append(regular, span)
				regularRefs = append(regularRefs, ref)
			}
		}
		if len(regular) == 0 {
			return nil, fmt.Errorf("chunk %d has no regular source lines", chunk.SeqNo)
		}
		spans, _ := json.Marshal(regular)
		payload, _ := json.Marshal(map[string]any{"chunk_seqno": chunk.SeqNo, "overlap_line_spans": overlap, "source_line_refs": regularRefs, "overlap_line_refs": overlapRefs})
		rows = append(rows, kbsearch.RegistryRow{
			ArtifactType: "chunk", ArtifactID: kbsearch.BuildArtifactID(rec.ID, "chunk", strconv.Itoa(chunk.SeqNo)), InputRecordID: rec.ID,
			PrimaryLabel: fmt.Sprintf("%s · %d", rec.FileName, chunk.SeqNo), SecondaryLabel: rec.FileName,
			SourceTitle: rec.FileName, SourceFilename: rec.FileName, SearchDocument: text, SnippetBasis: text,
			SourceLineSpans: spans, SemanticPayload: payload,
		})
	}
	return rows, nil
}

// loadSearchChunks uses saved boundaries and refuses the review loader's rebuild fallback.
func loadSearchChunks(rec InputRecord, lines []Line) ([]Chunk, error) {
	artDir := strings.TrimSpace(os.Getenv("ARTIFACT_DIR"))
	if artDir == "" {
		return nil, fmt.Errorf("ARTIFACT_DIR is required")
	}
	name := buildChunkArtifactBaseName(rec.StagingFilename, rec.ParserName) + ".chunks"
	// The shared loader intentionally skips absent lines for reviewers. Search indexing
	// must reject those stale boundaries rather than publish a partial source passage.
	body, err := os.ReadFile(filepath.Join(artDir, strconv.FormatInt(rec.ID/1000, 10), strconv.FormatInt(rec.ID, 10), name))
	if err != nil {
		return nil, err
	}
	available := make(map[int]bool, len(lines))
	for _, line := range lines {
		available[line.LineNo] = true
	}
	for _, row := range strings.Split(string(body), "\n") {
		row = strings.TrimSpace(row)
		if !strings.HasPrefix(row, "lines:") && !strings.HasPrefix(row, "overlap:") {
			continue
		}
		raw := strings.TrimSpace(strings.SplitN(row, ":", 2)[1])
		numbers, err := parseLineNumberRanges(raw)
		if err != nil {
			return nil, err
		}
		for _, n := range numbers {
			if !available[n] {
				return nil, fmt.Errorf("stale chunk artifact: source line %d is missing", n)
			}
		}
	}
	chunks, err := loadChunksFromArtifactFile(artDir, rec.ID, name, lines)
	if err != nil {
		return nil, err
	}
	if len(chunks) == 0 {
		return nil, fmt.Errorf("chunk artifact for record %d is empty", rec.ID)
	}
	return chunks, nil
}

func indexChunkSearch(ctx context.Context, db *sql.DB, rec InputRecord, chunks []Chunk, generation int64, logger ApiTypes.JimoLogger) error {
	rows, err := buildChunkRegistryRows(rec, chunks)
	if err != nil {
		return err
	}
	if kbsearch.SemanticSearchEnabled() {
		embedChunkRegistryRows(withLLMRecordID(ctx, rec.ID), rows, logger, "embed_chunk", "20261111-731")
	}
	if err := kbsearch.ReplaceChunkSearchRows(ctx, db, rec.ID, rows, generation); err != nil {
		return err
	}
	if logger != nil {
		logger.Info("chunk search indexed", "record_id", rec.ID, "chunks", len(rows))
	}
	return nil
}

func invalidateChunkSearchForRecord(ctx context.Context, recordID int64) error {
	return kbsearch.InvalidateChunkSearch(ctx, ApiTypes.ProjectDBHandle, recordID)
}

type chunkSearchLockKey struct{}
type chunkSearchSnapshot struct{ recordID, generation int64 }

func lockChunkSearchSource(ctx context.Context, db *sql.DB, recordID int64) (context.Context, func(), error) {
	snapshot, held := ctx.Value(chunkSearchLockKey{}).(chunkSearchSnapshot)
	if db == nil || (held && snapshot.recordID == recordID) {
		return ctx, func() {}, nil
	}
	unlock, err := kbsearch.LockChunkSearchSource(ctx, db, recordID)
	if err != nil {
		return ctx, nil, err
	}
	generation, err := kbsearch.ChunkSearchGeneration(ctx, db, recordID)
	if err != nil {
		unlock()
		return ctx, nil, err
	}
	return context.WithValue(ctx, chunkSearchLockKey{}, chunkSearchSnapshot{recordID, generation}), unlock, nil
}

// An already-built buffer may predate a source rewrite even after the caller
// obtains the lock. Verify its search-relevant lines against the current file.
func verifyChunkSearchSource(rec InputRecord, lines []Line) error {
	path, err := ResolveInputFilePath(LineFileGeneratedEvent{RecordID: rec.ID}, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	if err != nil {
		return err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	current, err := ParseInputLines(body)
	if err != nil {
		return err
	}
	provided := make([]Line, 0, len(lines))
	for _, l := range lines {
		if !strings.EqualFold(l.LineType, "TOC") {
			provided = append(provided, l)
		}
	}
	if len(current) != len(provided) {
		return fmt.Errorf("chunk source changed for record %d; rerun the pipeline", rec.ID)
	}
	for i, l := range current {
		p := provided[i]
		if l.LineNo != p.LineNo || l.PageNo != p.PageNo || l.LineType != p.LineType || strings.TrimSpace(l.Content) != strings.TrimSpace(p.Content) {
			return fmt.Errorf("chunk source changed for record %d at line %d; rerun the pipeline", rec.ID, l.LineNo)
		}
	}
	return nil
}

// embedChunkRegistryRows reuses batching/retries and preserves every window on failure.
func embedChunkRegistryRows(ctx context.Context, rows []kbsearch.RegistryRow, logger ApiTypes.JimoLogger, reason, loc string) {
	var work []kbsearch.RegistryRow
	ranges := make([][2]int, len(rows))
	for i, r := range rows {
		start := len(work)
		text := r.SearchDocument
		if text == "" {
			text = r.EmbeddingText
		}
		for _, window := range kbsearch.ChunkEmbeddingTexts(text) {
			work = append(work, kbsearch.RegistryRow{ArtifactType: "chunk", ArtifactID: r.ArtifactID, InputRecordID: r.InputRecordID, EmbeddingText: window})
		}
		ranges[i] = [2]int{start, len(work)}
	}
	embedRegistryRows(ctx, work, logger, reason, loc)
	for i, span := range ranges {
		if span[0] == span[1] {
			continue
		}
		complete := true
		for _, w := range work[span[0]:span[1]] {
			if len(w.Embedding) == 0 {
				complete = false
			}
		}
		rows[i].EmbeddingText = work[span[0]].EmbeddingText
		rows[i].Embedding = nil
		rows[i].EmbeddingWindows = nil
		if complete {
			rows[i].Embedding = work[span[0]].Embedding
		}
		for _, w := range work[span[0]+1 : span[1]] {
			vec := w.Embedding
			if !complete {
				vec = nil
			}
			rows[i].EmbeddingWindows = append(rows[i].EmbeddingWindows, kbsearch.EmbeddingWindow{Text: w.EmbeddingText, Embedding: vec})
		}
	}
}
