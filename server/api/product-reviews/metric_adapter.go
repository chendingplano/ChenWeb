package productreviews

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/lib/pq"
)

// metricAdapter is the `metric` artifact adapter. It keys results on the
// existing kb.search_artifacts_metric rows (artifact_id, source_row_id, line
// spans) and joins kb.metrics for the subject and its concept id.
type metricAdapter struct{}

func (metricAdapter) Type() string      { return "metric" }
func (metricAdapter) Partition() string { return "metric" }

const metricSelect = `
	SELECT sa.artifact_id,
	       COALESCE(sa.source_row_id, 0),
	       sa.input_record_id,
	       COALESCE(NULLIF(sa.primary_label, ''), m.metric_name, ''),
	       COALESCE(m.metric_subject, ''),
	       COALESCE(m.subject_concept_id, ''),
	       COALESCE(sa.source_line_spans, '[]'::jsonb)
	FROM kb.search_artifacts_metric sa
	JOIN kb.metrics m ON m.id = sa.source_row_id`

func scanArtifacts(rows *sql.Rows, artifactType string) ([]Artifact, error) {
	defer func() { _ = rows.Close() }()
	var out []Artifact
	for rows.Next() {
		var (
			a        Artifact
			spansRaw []byte
		)
		if err := rows.Scan(&a.ArtifactID, &a.SourceRowID, &a.InputRecordID,
			&a.PrimaryLabel, &a.SubjectText, &a.SubjectConcept, &spansRaw); err != nil {
			return nil, err
		}
		a.ArtifactType = artifactType
		a.LineSpans = json.RawMessage(spansRaw)
		out = append(out, a)
	}
	return out, rows.Err()
}

func (metricAdapter) ListByDocuments(ctx context.Context, db *sql.DB, recordIDs []int64) ([]Artifact, error) {
	if len(recordIDs) == 0 {
		return nil, nil
	}
	rows, err := db.QueryContext(ctx, metricSelect+`
		WHERE sa.input_record_id = ANY($1)
		ORDER BY sa.artifact_id`, pq.Array(recordIDs))
	if err != nil {
		return nil, err
	}
	return scanArtifacts(rows, "metric")
}

func (metricAdapter) MatchBySubjectConcept(ctx context.Context, db *sql.DB, conceptIDs []string) ([]Artifact, error) {
	conceptIDs = dedupeStrings(conceptIDs)
	if len(conceptIDs) == 0 {
		return nil, nil
	}
	rows, err := db.QueryContext(ctx, metricSelect+`
		WHERE m.subject_concept_id = ANY($1)
		ORDER BY sa.artifact_id`, pq.Array(conceptIDs))
	if err != nil {
		return nil, err
	}
	return scanArtifacts(rows, "metric")
}

func (metricAdapter) Project(ctx context.Context, db *sql.DB, artifactIDs []string) ([]Artifact, error) {
	artifactIDs = dedupeStrings(artifactIDs)
	if len(artifactIDs) == 0 {
		return nil, nil
	}
	rows, err := db.QueryContext(ctx, metricSelect+`
		WHERE sa.artifact_id = ANY($1)
		ORDER BY sa.artifact_id`, pq.Array(artifactIDs))
	if err != nil {
		return nil, err
	}
	return scanArtifacts(rows, "metric")
}
