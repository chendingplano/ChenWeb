package complianceeval

// Pool augmentation (openspec compliance-reviewer-phase2, design D12): a gold run built from a pool
// never judged the passages its pool missed. When a reviewer run reports such a passage, AugmentPool
// writes it into a pool file for the agent to label, and MergeAugmentation turns the labels into a
// new run that holds the base run's judgements plus the new ones.

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/searchengine"
	"github.com/lib/pq"
)

// StoredRun is a saved agent gold run of one document, read back for augmentation.
type StoredRun struct {
	ID, SnapshotID, RecordID      int64
	Key, RulesVersion, Annotator1 string
	MaxCandidates                 int
	Activities                    []GoldActivity // in saved order, with their judgements
}

// LoadStoredRun reads an agent gold run with its activities and judgements. Agent runs hold one
// document; a model-panel run is refused.
func LoadStoredRun(ctx context.Context, db *sql.DB, key string) (*StoredRun, error) {
	r := &StoredRun{Key: key}
	var method string
	if err := db.QueryRowContext(ctx, `SELECT id, method, snapshot_id, rules_version, annotator_1, max_candidates
		FROM testbed.compliance_gold_runs WHERE run_key = $1`, key).
		Scan(&r.ID, &method, &r.SnapshotID, &r.RulesVersion, &r.Annotator1, &r.MaxCandidates); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("gold run %q does not exist", key)
		}
		return nil, err
	}
	if method != MethodAgent {
		return nil, fmt.Errorf("gold run %s was labelled by %s; only agent runs are augmented", key, method)
	}
	rows, err := db.QueryContext(ctx, `SELECT id, input_record_id, activity, object, line_start, line_end, document_level,
		agreement, rationale, queries, cited_docs FROM testbed.compliance_gold_activities WHERE run_id = $1 ORDER BY id`, r.ID)
	if err != nil {
		return nil, err
	}
	var ids []int64
	byID := map[int64]int{}
	for rows.Next() {
		var id, rec int64
		var a Activity
		if err := rows.Scan(&id, &rec, &a.Name, &a.Object, &a.LineStart, &a.LineEnd, &a.DocumentLevel, &a.Agreement, &a.Rationale,
			pq.Array(&a.Queries), pq.Array(&a.CitedDocs)); err != nil {
			rows.Close()
			return nil, err
		}
		if r.RecordID != 0 && rec != r.RecordID {
			rows.Close()
			return nil, fmt.Errorf("gold run %s holds records %d and %d; augmentation needs a run of one document", key, r.RecordID, rec)
		}
		r.RecordID = rec
		byID[id] = len(r.Activities)
		ids = append(ids, id)
		r.Activities = append(r.Activities, GoldActivity{Activity: a})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("gold run %s has no activities", key)
	}
	js, err := db.QueryContext(ctx, `SELECT activity_id, artifact_type, artifact_id, candidate_record, cosine, label_1, reason_1,
		final_label, found_by FROM testbed.compliance_gold_judgements WHERE activity_id = ANY($1) ORDER BY id`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer js.Close()
	for js.Next() {
		var actID int64
		var j Judgement
		var cos sql.NullFloat64
		if err := js.Scan(&actID, &j.Candidate.ArtifactType, &j.Candidate.ArtifactID, &j.Candidate.RecordID, &cos,
			&j.Label1, &j.Reason1, &j.Final, &j.Candidate.FoundBy); err != nil {
			return nil, err
		}
		if cos.Valid {
			v := cos.Float64
			j.Candidate.Cosine = &v
		}
		i := byID[actID]
		r.Activities[i].Judgements = append(r.Activities[i].Judgements, j)
	}
	return r, js.Err()
}

func activityKey(a Activity) string {
	return fmt.Sprintf("%s|%d|%d|%t", a.Name, a.LineStart, a.LineEnd, a.DocumentLevel)
}

// AugmentPool builds the augmentation pool of run for one review run: per gold activity, in the run's
// order, the passages the review rows covering it report on usable records (inSnapshot, not in
// exclude) that the run never judged for it. acts are the run's activities checked against the
// document (they carry the source text). load returns the stored passages by key ("<type>/<id>");
// a reported passage it cannot find is an error. It returns the pool and the number of passages.
func AugmentPool(run *StoredRun, acts []Activity, reviewRun int64, rows []ReviewRow, inSnapshot, exclude map[int64]bool,
	load func([]searchengine.ArtifactRef) (map[string]Candidate, error)) (PoolFile, int, error) {
	if len(acts) != len(run.Activities) {
		return PoolFile{}, 0, fmt.Errorf("run %s has %d activities, %d were checked", run.Key, len(run.Activities), len(acts))
	}
	type pending struct {
		key    string
		record int64
		cosine *float64
	}
	perAct := make([][]pending, len(acts))
	var refs []searchengine.ArtifactRef
	asked := map[string]bool{}
	total := 0
	for i, ga := range run.Activities {
		judged := map[string]bool{}
		for _, j := range ga.Judgements {
			judged[j.Candidate.key()] = true
		}
		for _, r := range rows {
			if r.Record != run.RecordID || !spansMatch(ga.LineStart, ga.LineEnd, r.LineStart, r.LineEnd, ga.DocumentLevel, r.DocumentLevel) {
				continue
			}
			for _, c := range r.Artifacts {
				if judged[c.Key] || exclude[c.Record] || (inSnapshot != nil && !inSnapshot[c.Record]) {
					continue
				}
				judged[c.Key] = true // once per activity
				perAct[i] = append(perAct[i], pending{c.Key, c.Record, c.Cosine})
				total++
				if !asked[c.Key] {
					asked[c.Key] = true
					t, id, _ := strings.Cut(c.Key, "/")
					refs = append(refs, searchengine.ArtifactRef{Type: t, ID: id})
				}
			}
		}
	}
	pf := PoolFile{SnapshotID: run.SnapshotID, RecordID: run.RecordID, MaxCandidates: run.MaxCandidates,
		BaseRun: run.Key, ReviewRun: reviewRun}
	var loaded map[string]Candidate
	if len(refs) > 0 {
		var err error
		if loaded, err = load(refs); err != nil {
			return PoolFile{}, 0, err
		}
	}
	for i, a := range acts {
		pa := PoolActivity{ID: fmt.Sprintf("a%d", i+1), AgentActivity: agentActivityOf(a), SourceText: a.SourceText,
			Candidates: []PoolCandidate{}}
		for _, p := range perAct[i] {
			c, ok := loaded[p.key]
			if !ok {
				return PoolFile{}, 0, fmt.Errorf("review run %d reports passage %s, which is no longer in the search index", reviewRun, p.key)
			}
			c.RecordID, c.Cosine, c.FoundBy = p.record, p.cosine, fmt.Sprintf("review:%d", reviewRun)
			pa.Candidates = append(pa.Candidates, poolCandidate(c))
		}
		pf.Activities = append(pf.Activities, pa)
	}
	return pf, total, nil
}

// MergeAugmentation returns the document of the augmented run: the base run's activities, each with
// its judgements followed by the judgements of the augmentation labels (aug, built from the
// augmentation pool). The pool must be for the base run's snapshot, document and activities.
func MergeAugmentation(base *StoredRun, pf PoolFile, aug *GoldDocument) (*GoldDocument, error) {
	switch {
	case pf.BaseRun != base.Key:
		return nil, fmt.Errorf("the pool augments run %s, not %s", pf.BaseRun, base.Key)
	case pf.SnapshotID != base.SnapshotID:
		return nil, fmt.Errorf("the pool uses snapshot %d, run %s snapshot %d", pf.SnapshotID, base.Key, base.SnapshotID)
	case pf.RecordID != base.RecordID:
		return nil, fmt.Errorf("the pool is for record %d, run %s for record %d", pf.RecordID, base.Key, base.RecordID)
	case len(aug.Activities) != len(base.Activities):
		return nil, fmt.Errorf("the pool has %d activities, run %s has %d", len(aug.Activities), base.Key, len(base.Activities))
	}
	out := &GoldDocument{GoldDoc: aug.GoldDoc}
	for i, b := range base.Activities {
		a := aug.Activities[i]
		if activityKey(a.Activity) != activityKey(b.Activity) {
			return nil, fmt.Errorf("activity %d of the pool (%q, lines %d-%d) is not activity %d of run %s (%q, lines %d-%d)",
				i+1, a.Name, a.LineStart, a.LineEnd, i+1, base.Key, b.Name, b.LineStart, b.LineEnd)
		}
		merged := GoldActivity{Activity: b.Activity}
		merged.Agreement = agreeSingle
		seen := map[string]bool{}
		for _, j := range b.Judgements {
			seen[j.Candidate.key()] = true
			merged.Judgements = append(merged.Judgements, j)
		}
		for _, j := range a.Judgements {
			if seen[j.Candidate.key()] {
				return nil, fmt.Errorf("activity %q: run %s already judged %s", b.Name, base.Key, j.Candidate.key())
			}
			merged.Judgements = append(merged.Judgements, j)
		}
		out.Activities = append(out.Activities, merged)
	}
	return out, nil
}
