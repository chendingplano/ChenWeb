package docprocessing

import (
	"context"
	"database/sql"
	"errors"
	"sort"

	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
)

type ClassificationFactLoader struct {
	DB *sql.DB
}

type DeploymentFactContext struct {
	Workspace      string
	Tenant         string
	KnowledgeStore string
	User           string
	Corpus         string
}

func (l ClassificationFactLoader) LoadObjectClassFacts(ctx context.Context, recordID, vocabularyReleaseID int64) (semrules.FactSet, error) {
	if l.DB == nil {
		return nil, errors.New("db is nil")
	}
	if recordID <= 0 {
		return nil, errors.New("record_id is required")
	}
	const stmt = `
SELECT a.id, COALESCE(a.subject_object_id, ''), a.object_ref_id, COALESCE(a.confidence, e.confidence)
FROM kb.semantic_assertions a
JOIN kb.assertion_evidence e ON e.assertion_id = a.id
WHERE e.input_record_id = $1
  AND a.predicate_term_id = 'core:instance_of'
  AND a.object_ref_kind = 'ontology_term'
  AND a.status = 'accepted'
  AND e.deleted = false
ORDER BY a.object_ref_id, a.id`
	rows, err := l.DB.QueryContext(ctx, stmt, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	classes := map[string]struct{}{}
	var minConfidence *float64
	for rows.Next() {
		var assertionID int64
		var objectID, classTermID string
		var confidence sql.NullFloat64
		if err := rows.Scan(&assertionID, &objectID, &classTermID, &confidence); err != nil {
			return nil, err
		}
		_ = assertionID
		_ = objectID
		classes[classTermID] = struct{}{}
		if confidence.Valid {
			minConfidence = minFloat64Ptr(minConfidence, &confidence.Float64)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(classes) == 0 {
		return semrules.FactSet{"object.class": {Path: "object.class", State: semrules.FactMissing}}, nil
	}
	values := make([]string, 0, len(classes))
	for classTermID := range classes {
		values = append(values, classTermID)
	}
	sort.Strings(values)
	return semrules.FactSet{"object.class": {
		Path:       "object.class",
		State:      semrules.FactKnown,
		Value:      values,
		Confidence: minConfidence,
		Method:     "classification",
		ReleaseID:  int64String(vocabularyReleaseID),
	}}, nil
}

func BuildDeploymentFacts(ctx DeploymentFactContext) (semrules.FactSet, error) {
	builder := semrules.NewFactSetBuilder()
	for _, fact := range []semrules.Fact{
		stringFact("deployment.workspace", ctx.Workspace, ""),
		stringFact("deployment.tenant", ctx.Tenant, ""),
		stringFact("deployment.knowledge_store", ctx.KnowledgeStore, ""),
		stringFact("deployment.user", ctx.User, ""),
		stringFact("deployment.corpus", ctx.Corpus, ""),
	} {
		if err := builder.Add(fact); err != nil {
			return nil, err
		}
	}
	return builder.Build(), nil
}

func MergeApplicabilityFactSets(sets ...semrules.FactSet) (semrules.FactSet, error) {
	builder := semrules.NewFactSetBuilder()
	for _, set := range sets {
		if err := builder.AddSet(set); err != nil {
			return nil, err
		}
	}
	return builder.Build(), nil
}

func stringFact(path, value, releaseID string) semrules.Fact {
	fact := semrules.Fact{Path: path, State: semrules.FactMissing}
	if value == "" {
		return fact
	}
	fact.State = semrules.FactKnown
	fact.Value = value
	fact.ReleaseID = releaseID
	return fact
}
