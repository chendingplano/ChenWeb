// Package complianceeval holds the evaluation side of the Compliance Review reviewer
// (openspec compliance-reviewer-phase2): frozen evaluation snapshots of the searchable corpus.
//
// A snapshot does not copy the corpus, which stays in kb.search_artifacts_*. It records, for each
// member record, the file hash, the chunk-index generation and the index counts, plus the
// embedding model, so a gold run can refuse to start when any of them changed since the freeze.
package complianceeval

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// Member is the state of one record in the searchable corpus.
type Member struct {
	RecordID       int64
	MD5            string
	Generation     int64
	ChunkCount     int
	ProvisionCount int
}

// Difference names one member that no longer matches the manifest.
type Difference struct {
	RecordID int64
	Reason   string
}

func (d Difference) String() string { return fmt.Sprintf("record %d: %s", d.RecordID, d.Reason) }

// Diff compares the manifest with the current state of the same records. A record missing from
// current (deleted or not indexed) is a difference. Order follows the manifest's record ids.
func Diff(manifest []Member, current map[int64]Member) []Difference {
	ms := append([]Member(nil), manifest...)
	sort.Slice(ms, func(i, j int) bool { return ms[i].RecordID < ms[j].RecordID })
	var out []Difference
	for _, m := range ms {
		c, ok := current[m.RecordID]
		if !ok {
			out = append(out, Difference{m.RecordID, "no longer present"})
			continue
		}
		var why []string
		if c.MD5 != m.MD5 {
			why = append(why, fmt.Sprintf("md5 %q -> %q", m.MD5, c.MD5))
		}
		if c.Generation != m.Generation {
			why = append(why, fmt.Sprintf("index generation %d -> %d", m.Generation, c.Generation))
		}
		if c.ChunkCount != m.ChunkCount {
			why = append(why, fmt.Sprintf("chunk artifacts %d -> %d", m.ChunkCount, c.ChunkCount))
		}
		if c.ProvisionCount != m.ProvisionCount {
			why = append(why, fmt.Sprintf("provision artifacts %d -> %d", m.ProvisionCount, c.ProvisionCount))
		}
		if len(why) > 0 {
			out = append(out, Difference{m.RecordID, strings.Join(why, "; ")})
		}
	}
	return out
}

// CurrentMembers reads the present state of the given records. A record with no kb.inputs row is
// absent from the result.
func CurrentMembers(ctx context.Context, db *sql.DB, recordIDs []int64) (map[int64]Member, error) {
	out := make(map[int64]Member, len(recordIDs))
	for _, id := range recordIDs {
		m := Member{RecordID: id}
		err := db.QueryRowContext(ctx, `
			SELECT COALESCE(i.md5, ''),
			       COALESCE((SELECT generation FROM kb.chunk_search_state s WHERE s.input_record_id = i.id), 0),
			       (SELECT count(*) FROM kb.search_artifacts_chunk c WHERE c.input_record_id = i.id),
			       (SELECT count(*) FROM kb.search_artifacts_provision p WHERE p.input_record_id = i.id)
			FROM kb.inputs i WHERE i.id = $1`, id).
			Scan(&m.MD5, &m.Generation, &m.ChunkCount, &m.ProvisionCount)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read record %d state: %w", id, err)
		}
		out[id] = m
	}
	return out, nil
}

// Record freezes the current state of recordIDs as a new snapshot and returns its id. Every
// record must exist and have at least one indexed chunk, otherwise nothing is written.
func Record(ctx context.Context, db *sql.DB, name, embeddingModel, notes string, recordIDs []int64) (int64, error) {
	name, embeddingModel = strings.TrimSpace(name), strings.TrimSpace(embeddingModel)
	if name == "" {
		return 0, fmt.Errorf("snapshot name is required")
	}
	if embeddingModel == "" {
		return 0, fmt.Errorf("embedding model is required (set EMBEDDING_MODEL_NAME)")
	}
	if len(recordIDs) == 0 {
		return 0, fmt.Errorf("a snapshot needs at least one record")
	}
	cur, err := CurrentMembers(ctx, db, recordIDs)
	if err != nil {
		return 0, err
	}
	seen := map[int64]bool{}
	for _, id := range recordIDs {
		if seen[id] {
			return 0, fmt.Errorf("record %d listed twice", id)
		}
		seen[id] = true
		m, ok := cur[id]
		if !ok {
			return 0, fmt.Errorf("record %d does not exist", id)
		}
		if m.ChunkCount == 0 {
			return 0, fmt.Errorf("record %d has no indexed chunks; index it before freezing", id)
		}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var snapID int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO testbed.compliance_eval_snapshots (name, embedding_model, notes)
		VALUES ($1, $2, $3) RETURNING id`, name, embeddingModel, notes).Scan(&snapID); err != nil {
		return 0, fmt.Errorf("insert snapshot %q: %w", name, err)
	}
	for _, id := range recordIDs {
		m := cur[id]
		if _, err := tx.ExecContext(ctx, `INSERT INTO testbed.compliance_eval_snapshot_records
			(snapshot_id, input_record_id, md5, generation, chunk_count, provision_count)
			VALUES ($1, $2, $3, $4, $5, $6)`, snapID, id, m.MD5, m.Generation, m.ChunkCount, m.ProvisionCount); err != nil {
			return 0, fmt.Errorf("insert snapshot member %d: %w", id, err)
		}
	}
	return snapID, tx.Commit()
}

// Verify compares snapshot snapshotID with the corpus now and returns every difference, including
// a changed embedding model (currentEmbeddingModel). An empty result means the snapshot still holds.
func Verify(ctx context.Context, db *sql.DB, snapshotID int64, currentEmbeddingModel string) ([]Difference, error) {
	var model string
	if err := db.QueryRowContext(ctx, `SELECT embedding_model FROM testbed.compliance_eval_snapshots WHERE id = $1`, snapshotID).Scan(&model); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("snapshot %d does not exist", snapshotID)
		}
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT input_record_id, md5, generation, chunk_count, provision_count
		FROM testbed.compliance_eval_snapshot_records WHERE snapshot_id = $1`, snapshotID)
	if err != nil {
		return nil, err
	}
	var manifest []Member
	var ids []int64
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.RecordID, &m.MD5, &m.Generation, &m.ChunkCount, &m.ProvisionCount); err != nil {
			rows.Close()
			return nil, err
		}
		manifest = append(manifest, m)
		ids = append(ids, m.RecordID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	cur, err := CurrentMembers(ctx, db, ids)
	if err != nil {
		return nil, err
	}
	out := Diff(manifest, cur)
	if strings.TrimSpace(currentEmbeddingModel) != model {
		out = append([]Difference{{0, fmt.Sprintf("embedding model %q -> %q", model, currentEmbeddingModel)}}, out...)
	}
	return out, nil
}
