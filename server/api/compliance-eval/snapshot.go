// Package complianceeval holds the evaluation side of the Compliance Review reviewer
// (openspec compliance-reviewer-phase2): evaluation snapshots, gold labelling and scoring.
//
// A snapshot is a version stamp of the whole indexed knowledge base, not a chosen subset: the
// knowledge base has no parts. It does not copy the corpus, which stays in kb.search_artifacts_*.
// It records, for every indexed record, the file hash, the chunk-index generation and the index
// counts, plus the embedding model. Records added later are allowed; a snapshot record that
// changed or disappeared makes the snapshot fail verification.
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
	RecordID          int64
	MD5               string
	Generation        int64
	ChunkCount        int
	ProvisionCount    int
	ManifestVersion   int
	MetricCount       int
	MetricFingerprint string
}

// Difference names one member that no longer matches the manifest.
type Difference struct {
	RecordID int64
	Reason   string
}

func (d Difference) String() string { return fmt.Sprintf("record %d: %s", d.RecordID, d.Reason) }

// Diff compares the manifest with the current state of its records. A manifest record missing from
// current (deleted or no longer indexed) is a difference; records only in current are not (see
// Added). Order follows the manifest's record ids.
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
		if m.ManifestVersion >= 2 {
			if c.MetricCount != m.MetricCount {
				why = append(why, fmt.Sprintf("metric artifacts %d -> %d", m.MetricCount, c.MetricCount))
			}
			if c.MetricFingerprint != m.MetricFingerprint {
				why = append(why, "metric fingerprint changed")
			}
		}
		if len(why) > 0 {
			out = append(out, Difference{m.RecordID, strings.Join(why, "; ")})
		}
	}
	return out
}

// CurrentIndexed reads the present state of every indexed record: each kb.inputs row with at least
// one chunk, provision or metric search artifact.
func CurrentIndexed(ctx context.Context, db *sql.DB) (map[int64]Member, error) {
	return currentIndexed(ctx, db, 2)
}

func currentIndexed(ctx context.Context, db *sql.DB, version int) (map[int64]Member, error) {
	query := `
		SELECT i.id, COALESCE(i.md5, ''), COALESCE(s.generation, 0), COALESCE(c.n, 0), COALESCE(p.n, 0)
		FROM kb.inputs i
		LEFT JOIN kb.chunk_search_state s ON s.input_record_id = i.id
		LEFT JOIN (SELECT input_record_id, count(*) AS n FROM kb.search_artifacts_chunk GROUP BY 1) c ON c.input_record_id = i.id
		LEFT JOIN (SELECT input_record_id, count(*) AS n FROM kb.search_artifacts_provision GROUP BY 1) p ON p.input_record_id = i.id
		WHERE COALESCE(c.n, 0) > 0 OR COALESCE(p.n, 0) > 0`
	if version >= 2 {
		query = metricIndexedSQL
	}
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("read indexed records: %w", err)
	}
	defer rows.Close()
	out := map[int64]Member{}
	for rows.Next() {
		var m Member
		m.ManifestVersion = version
		args := []any{&m.RecordID, &m.MD5, &m.Generation, &m.ChunkCount, &m.ProvisionCount}
		if version >= 2 {
			args = append(args, &m.MetricCount, &m.MetricFingerprint)
		}
		if err := rows.Scan(args...); err != nil {
			return nil, err
		}
		out[m.RecordID] = m
	}
	return out, rows.Err()
}

// Added returns, in id order, the records indexed now that the manifest does not hold.
func Added(manifest []Member, current map[int64]Member) []int64 {
	in := make(map[int64]bool, len(manifest))
	for _, m := range manifest {
		in[m.RecordID] = true
	}
	var out []int64
	for id := range current {
		if !in[id] {
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Record stamps the current state of the whole indexed knowledge base as a new snapshot and
// returns its id and record count. An empty index is an error: there would be nothing to judge.
func Record(ctx context.Context, db *sql.DB, name, embeddingModel, notes string) (int64, int, error) {
	name, embeddingModel = strings.TrimSpace(name), strings.TrimSpace(embeddingModel)
	if name == "" {
		return 0, 0, fmt.Errorf("snapshot name is required")
	}
	if embeddingModel == "" {
		return 0, 0, fmt.Errorf("embedding model is required (set EMBEDDING_MODEL_NAME)")
	}
	cur, err := CurrentIndexed(ctx, db)
	if err != nil {
		return 0, 0, err
	}
	if len(cur) == 0 {
		return 0, 0, fmt.Errorf("no record is indexed for search; index the knowledge base first")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	var snapID int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO testbed.compliance_eval_snapshots (name, embedding_model, notes, manifest_version)
		VALUES ($1, $2, $3, 2) RETURNING id`, name, embeddingModel, notes).Scan(&snapID); err != nil {
		return 0, 0, fmt.Errorf("insert snapshot %q: %w", name, err)
	}
	for _, m := range cur {
		if _, err := tx.ExecContext(ctx, `INSERT INTO testbed.compliance_eval_snapshot_records
			(snapshot_id, input_record_id, md5, generation, chunk_count, provision_count, metric_count, metric_fingerprint)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, snapID, m.RecordID, m.MD5, m.Generation, m.ChunkCount, m.ProvisionCount, m.MetricCount, m.MetricFingerprint); err != nil {
			return 0, 0, fmt.Errorf("insert snapshot record %d: %w", m.RecordID, err)
		}
	}
	return snapID, len(cur), tx.Commit()
}

// Verify compares snapshot snapshotID with the knowledge base now. diffs names every snapshot record
// that changed or disappeared, and a changed embedding model (currentEmbeddingModel); empty diffs
// means the snapshot still holds. added lists records indexed since the snapshot, which are allowed:
// gold pooling leaves them out and the scorer reports reviewer hits on them as unjudged.
func Verify(ctx context.Context, db *sql.DB, snapshotID int64, currentEmbeddingModel string) (diffs []Difference, added []int64, err error) {
	var model string
	var version int
	if err := db.QueryRowContext(ctx, `SELECT embedding_model, manifest_version FROM testbed.compliance_eval_snapshots WHERE id = $1`, snapshotID).Scan(&model, &version); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, fmt.Errorf("snapshot %d does not exist", snapshotID)
		}
		return nil, nil, err
	}
	manifest, err := SnapshotMembers(ctx, db, snapshotID)
	if err != nil {
		return nil, nil, err
	}
	cur, err := currentIndexed(ctx, db, version)
	if err != nil {
		return nil, nil, err
	}
	diffs = Diff(manifest, cur)
	if strings.TrimSpace(currentEmbeddingModel) != model {
		diffs = append([]Difference{{0, fmt.Sprintf("embedding model %q -> %q", model, currentEmbeddingModel)}}, diffs...)
	}
	return diffs, Added(manifest, cur), nil
}

// SnapshotMembers returns the manifest rows of a snapshot.
func SnapshotMembers(ctx context.Context, db *sql.DB, snapshotID int64) ([]Member, error) {
	rows, err := db.QueryContext(ctx, `SELECT r.input_record_id, r.md5, r.generation, r.chunk_count, r.provision_count, r.metric_count, r.metric_fingerprint, s.manifest_version
		FROM testbed.compliance_eval_snapshot_records r JOIN testbed.compliance_eval_snapshots s ON s.id = r.snapshot_id WHERE r.snapshot_id = $1 ORDER BY r.input_record_id`, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.RecordID, &m.MD5, &m.Generation, &m.ChunkCount, &m.ProvisionCount, &m.MetricCount, &m.MetricFingerprint, &m.ManifestVersion); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Hash complete source and index rows: same-count changes to values, semantic payload,
// embeddings, keywords and provenance all invalidate v2. Ignore maintenance timestamps.
const metricIndexedSQL = `
 SELECT i.id, COALESCE(i.md5, ''), COALESCE(s.generation, 0),
        COALESCE(c.n, 0), COALESCE(p.n, 0), COALESCE(m.n, 0), COALESCE(m.fingerprint, '')
 FROM kb.inputs i
 LEFT JOIN kb.chunk_search_state s ON s.input_record_id = i.id
 LEFT JOIN (SELECT input_record_id, count(*) n FROM kb.search_artifacts_chunk GROUP BY 1) c ON c.input_record_id = i.id
 LEFT JOIN (SELECT input_record_id, count(*) n FROM kb.search_artifacts_provision GROUP BY 1) p ON p.input_record_id = i.id
 LEFT JOIN (
   SELECT a.input_record_id, count(*) n,
          md5(string_agg(md5(jsonb_build_array(to_jsonb(a) - 'updated_at', to_jsonb(src) - 'updated_at')::text), E'\n' ORDER BY a.artifact_id)) fingerprint
   FROM kb.search_artifacts_metric a LEFT JOIN kb.metrics src ON src.id = a.source_row_id
   GROUP BY a.input_record_id
 ) m ON m.input_record_id = i.id
 WHERE COALESCE(c.n, 0) > 0 OR COALESCE(p.n, 0) > 0 OR COALESCE(m.n, 0) > 0`

// RequireMetricSnapshot prevents new metric-inclusive pools from using a legacy manifest.
func RequireMetricSnapshot(ctx context.Context, db *sql.DB, snapshotID int64) error {
	var version int
	if err := db.QueryRowContext(ctx, `SELECT manifest_version FROM testbed.compliance_eval_snapshots WHERE id = $1`, snapshotID).Scan(&version); err != nil {
		return err
	}
	if version != 2 {
		return fmt.Errorf("snapshot %d uses manifest version %d; record a new metric-inclusive snapshot (version 2)", snapshotID, version)
	}
	return nil
}
