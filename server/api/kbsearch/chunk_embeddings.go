package kbsearch

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

const ChunkEmbeddingMaxRunes = 6000
const chunkEmbeddingOverlapRunes = 300

type EmbeddingWindow struct {
	Text      string
	Embedding []float64
}

// ChunkEmbeddingTexts covers the complete passage, including long table lines.
func ChunkEmbeddingTexts(text string) []string {
	runes := []rune(strings.TrimSpace(text))
	var out []string
	for start := 0; start < len(runes); start += ChunkEmbeddingMaxRunes - chunkEmbeddingOverlapRunes {
		end := min(start+ChunkEmbeddingMaxRunes, len(runes))
		out = append(out, string(runes[start:end]))
		if end == len(runes) {
			break
		}
	}
	return out
}

// ChunkSearchGeneration snapshots the source version before files are read or models called.
func ChunkSearchGeneration(ctx context.Context, db *sql.DB, recordID int64) (int64, error) {
	if _, err := db.ExecContext(ctx, `INSERT INTO kb.chunk_search_state (input_record_id) VALUES ($1) ON CONFLICT DO NOTHING`, recordID); err != nil {
		return 0, err
	}
	var generation int64
	err := db.QueryRowContext(ctx, `SELECT generation FROM kb.chunk_search_state WHERE input_record_id = $1`, recordID).Scan(&generation)
	return generation, err
}

func lockChunkGeneration(ctx context.Context, tx *sql.Tx, recordID, expected int64) error {
	var generation int64
	if err := tx.QueryRowContext(ctx, `SELECT generation FROM kb.chunk_search_state WHERE input_record_id = $1 FOR UPDATE`, recordID).Scan(&generation); err != nil {
		return err
	}
	if generation != expected {
		return fmt.Errorf("chunk source changed for record %d; retry indexing", recordID)
	}
	return nil
}

// ReplaceChunkSearchRows publishes one complete generation; failed inserts retain the old index.
func ReplaceChunkSearchRows(ctx context.Context, db *sql.DB, recordID int64, rows []RegistryRow, generation int64) error {
	for _, r := range rows {
		if r.ArtifactType != "chunk" || r.InputRecordID != recordID {
			return fmt.Errorf("invalid chunk row for record %d", recordID)
		}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := lockChunkGeneration(ctx, tx, recordID, generation); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM kb.search_artifacts WHERE artifact_type = $1 AND input_record_id = $2`, "chunk", recordID); err != nil {
		return err
	}
	if _, err := insertSearchRegistryRows(ctx, tx, rows); err != nil {
		return err
	}
	for _, r := range rows {
		if err := insertChunkWindows(ctx, tx, r); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE kb.chunk_search_state SET generation = generation + 1 WHERE input_record_id = $1`, recordID); err != nil {
		return err
	}
	return tx.Commit()
}

func insertChunkWindows(ctx context.Context, tx *sql.Tx, row RegistryRow) error {
	for i, w := range row.EmbeddingWindows {
		if _, err := tx.ExecContext(ctx, `INSERT INTO kb.chunk_search_windows (artifact_id, window_index, embedding_text, embedding) VALUES ($1, $2, $3, $4::vector)`, row.ArtifactID, i+1, w.Text, formatEmbeddingArg(w.Embedding)); err != nil {
			return err
		}
	}
	return nil
}

// InvalidateChunkSearch advances the source version and cascades removal of window vectors.
func InvalidateChunkSearch(ctx context.Context, db *sql.DB, recordID int64) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO kb.chunk_search_state (input_record_id, generation) VALUES ($1, 1) ON CONFLICT (input_record_id) DO UPDATE SET generation = kb.chunk_search_state.generation + 1`, recordID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM kb.search_artifacts WHERE artifact_type = 'chunk' AND input_record_id = $1`, recordID); err != nil {
		return err
	}
	return tx.Commit()
}

// UpdateChunkEmbeddings replaces all vectors of an unchanged chunk in one transaction.
func UpdateChunkEmbeddings(ctx context.Context, db *sql.DB, row RegistryRow) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := lockChunkGeneration(ctx, tx, row.InputRecordID, row.SourceGeneration); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `UPDATE kb.search_artifacts SET embedding = $1::vector, embedding_text = $2, updated_at = NOW() WHERE artifact_type = 'chunk' AND artifact_id = $3`, formatEmbeddingArg(row.Embedding), row.EmbeddingText, row.ArtifactID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("chunk %s no longer exists", row.ArtifactID)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM kb.chunk_search_windows WHERE artifact_id = $1`, row.ArtifactID); err != nil {
		return err
	}
	if err := insertChunkWindows(ctx, tx, row); err != nil {
		return err
	}
	return tx.Commit()
}
