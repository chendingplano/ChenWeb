package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/chendingplano/deepdoc/server/api/ontology/assertions"
)

type structuralCandidateSink interface {
	Propose(context.Context, assertions.DecisionCandidate) (assertions.DecisionCandidate, error)
}

// lineNumbersFromSpanStrings expands kb.relations.line_spans entries (each a
// single line or a "start-end"/"start:end" range, per parseLineSpanRange) into
// line numbers. The column is stored as a JSON array of strings, not numbers.
func lineNumbersFromSpanStrings(spans []string) []int {
	out := make([]int, 0, len(spans)*2)
	for _, s := range spans {
		start, end := parseLineSpanRange(s)
		if start == 0 {
			continue
		}
		out = append(out, start)
		if end != start {
			out = append(out, end)
		}
	}
	return out
}

// HarvestProductStructureFromRelations converts only explicit, already-extracted
// structural relations with reconciled endpoints into decision candidates.
func HarvestProductStructureFromRelations(ctx context.Context, db *sql.DB, recordID int64, sink structuralCandidateSink) error {
	if db == nil || sink == nil {
		return fmt.Errorf("product structure harvester is not configured")
	}
	const q = `
SELECT r.relation_id, LOWER(BTRIM(r.predicate)), COALESCE(r.line_spans, '[]'::jsonb),
       COALESCE(subject_ao.object_id, ''), COALESCE(object_ao.object_id, '')
FROM kb.relations r
JOIN kb.artifact_objects subject_ao ON subject_ao.input_record_id = r.input_record_id AND subject_ao.artifact_type = 'entity' AND subject_ao.artifact_id = r.subject_entity_id
JOIN kb.artifact_objects object_ao ON object_ao.input_record_id = r.input_record_id AND object_ao.artifact_type = 'entity' AND object_ao.artifact_id = r.object_entity_id
WHERE r.input_record_id = $1 AND LOWER(BTRIM(r.predicate)) IN ('part_of', 'component_of')`
	rows, err := db.QueryContext(ctx, q, recordID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, relation, subject, object string
		var spans json.RawMessage
		if err := rows.Scan(&id, &relation, &spans, &subject, &object); err != nil {
			return err
		}
		var spanStrings []string
		_ = json.Unmarshal(spans, &spanStrings)
		c, err := buildProductStructureCandidate(recordID, productStructureMention{SubjectObjectID: subject, ObjectObjectID: object, Relation: relation, LineNumbers: lineNumbersFromSpanStrings(spanStrings)})
		if err != nil {
			return fmt.Errorf("relation %s: %w", id, err)
		}
		if _, err = sink.Propose(ctx, c); err != nil {
			return err
		}
	}
	return rows.Err()
}
