package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/ontology/assertions"
	"github.com/chendingplano/deepdoc/server/api/ontology/classfoundation"
	"github.com/chendingplano/deepdoc/server/api/ontology/semantic"
	"github.com/lib/pq"
)

// metricGraphNodeLimit caps every per-node row list. A metric relates to a
// handful of rows per node in practice; the cap is a defensive bound, not a
// paging boundary (openspec metric-scoped-explorer, design D7).
const metricGraphNodeLimit = 200

// errMetricNotFound is returned by metricGraphStore.Load when no kb.metrics row
// matches the (record, metric_id) pair. The handler maps it to HTTP 404.
var errMetricNotFound = errors.New("metric not found")

// MetricGraphMetric is the resolved metric a graph payload is centred on.
type MetricGraphMetric struct {
	MetricID      string `json:"metric_id"`
	MetricName    string `json:"metric_name"`
	MetricNameEn  string `json:"metric_name_en"`
	InputRecordID int64  `json:"input_record_id"`
}

// MetricGraphNode holds one chain node's metric-scoped rows. Rows are plain
// maps with documented field names; the frontend projects them to its column
// order (openspec metric-scoped-explorer, design D2).
type MetricGraphNode struct {
	Rows []map[string]any `json:"rows"`
}

// MetricGraph is the full per-metric explorer payload: the resolved metric plus
// one entry per model.ts chain-node id.
type MetricGraph struct {
	Metric MetricGraphMetric          `json:"metric"`
	Nodes  map[string]MetricGraphNode `json:"nodes"`
}

// metricGraphChainNodeIDs are the model.ts chain-node ids the payload keys on.
// Every id is always present in the response (possibly with an empty row list).
var metricGraphChainNodeIDs = []string{
	"object__mention", "object__node", "keyword__concept",
	"mdef__term", "mdef__contract",
	"proc__extract", "proc__normalize", "proc__associate", "proc__project",
	"ev__ae", "ev__dc", "ev__sa",
}

// metricGraphStore runs the read-only joins behind
// GET /api/v1/kb/metrics/:metric_id/graph. Every query is scoped to one metric
// and capped at metricGraphNodeLimit rows. It opens no transaction and issues
// only SELECTs.
type metricGraphStore struct {
	DB *sql.DB
}

// Load resolves the metric and every chain node's rows for it. recordID is the
// metric_id's numeric prefix; metricID is the canonical "<record>_mtc_<seq>".
func (s metricGraphStore) Load(ctx context.Context, recordID int64, metricID string) (MetricGraph, error) {
	g := MetricGraph{
		Metric: MetricGraphMetric{MetricID: metricID, InputRecordID: recordID},
		Nodes:  make(map[string]MetricGraphNode, len(metricGraphChainNodeIDs)),
	}
	for _, id := range metricGraphChainNodeIDs {
		g.Nodes[id] = MetricGraphNode{Rows: []map[string]any{}}
	}

	var (
		keywordConceptID       string
		metricDefinitionTermID string
		modelName              string
		isExplicit             bool
		confidence             sql.NullFloat64
		locationType           string
		createTime             sql.NullTime
	)
	err := s.DB.QueryRowContext(ctx, `
SELECT COALESCE(metric_name, ''), COALESCE(metric_name_en, ''),
       COALESCE(keyword_concept_id, ''), COALESCE(metric_definition_term_id, ''),
       COALESCE(model_name, ''), COALESCE(is_explicit_metric, false),
       confidence, COALESCE(location_type, ''), created_at
FROM kb.metrics
WHERE input_record_id = $1 AND metric_id = $2`, recordID, metricID).Scan(
		&g.Metric.MetricName, &g.Metric.MetricNameEn,
		&keywordConceptID, &metricDefinitionTermID,
		&modelName, &isExplicit, &confidence, &locationType, &createTime)
	if errors.Is(err, sql.ErrNoRows) {
		return MetricGraph{}, errMetricNotFound
	}
	if err != nil {
		return MetricGraph{}, err
	}

	g.Nodes["proc__extract"] = MetricGraphNode{Rows: []map[string]any{{
		"model_name":         emptyToNil(modelName),
		"is_explicit_metric": isExplicit,
		"confidence":         nullFloat(confidence),
		"location_type":      emptyToNil(locationType),
		"created_at":         nullTimeString(createTime),
	}}}

	// object__mention + the object ids its mentions resolve to.
	objectIDs, err := s.loadObjectMentions(ctx, metricID, &g)
	if err != nil {
		return MetricGraph{}, err
	}
	if err := s.loadObjectNodes(ctx, objectIDs, &g); err != nil {
		return MetricGraph{}, err
	}
	if err := s.loadKeywordConcept(ctx, keywordConceptID, &g); err != nil {
		return MetricGraph{}, err
	}

	// The metric's decision candidates -> resulting assertions carry the term
	// ids (unit / quantity-kind / assertion-kind / class) the term and contract
	// nodes need, and are themselves the ev__dc / ev__sa / proc__project rows.
	assertionIDs, err := s.loadDecisionCandidates(ctx, metricID, &g)
	if err != nil {
		return MetricGraph{}, err
	}
	instanceTermIDs, otherTermIDs, err := s.loadResultingAssertions(ctx, assertionIDs, &g)
	if err != nil {
		return MetricGraph{}, err
	}

	termIDs := dedupeNonEmpty(append([]string{metricDefinitionTermID}, otherTermIDs...))
	if err := s.loadOntologyTerms(ctx, termIDs, &g); err != nil {
		return MetricGraph{}, err
	}
	if err := s.loadClassContracts(ctx, dedupeNonEmpty(instanceTermIDs), &g); err != nil {
		return MetricGraph{}, err
	}
	if err := s.loadEvidence(ctx, metricID, &g); err != nil {
		return MetricGraph{}, err
	}
	if err := s.loadProcessingOutcomes(ctx, metricID, &g); err != nil {
		return MetricGraph{}, err
	}
	return g, nil
}

func (s metricGraphStore) loadObjectMentions(ctx context.Context, metricID string, g *MetricGraph) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `
SELECT id, COALESCE(object_name, ''), COALESCE(object_name_en, ''),
       COALESCE(object_id, ''), COALESCE(reconcile_status, '')
FROM kb.artifact_objects
WHERE artifact_type = 'metric' AND artifact_id = $1
ORDER BY id
LIMIT $2`, metricID, metricGraphNodeLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	var objectIDs []string
	for rows.Next() {
		var (
			id                             int64
			name, nameEn, objectID, status string
		)
		if err := rows.Scan(&id, &name, &nameEn, &objectID, &status); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"id":               id,
			"object_name":      emptyToNil(name),
			"object_name_en":   emptyToNil(nameEn),
			"object_id":        emptyToNil(objectID),
			"reconcile_status": emptyToNil(status),
		})
		if objectID != "" {
			objectIDs = append(objectIDs, objectID)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	g.Nodes["object__mention"] = MetricGraphNode{Rows: nonNilRows(out)}
	return dedupeNonEmpty(objectIDs), nil
}

func (s metricGraphStore) loadObjectNodes(ctx context.Context, objectIDs []string, g *MetricGraph) error {
	if len(objectIDs) == 0 {
		return nil
	}
	rows, err := s.DB.QueryContext(ctx, `
SELECT id, object_id, COALESCE(canonical_name, ''), COALESCE(canonical_name_en, ''),
       COALESCE(object_type, ''), COALESCE(reconcile_status, '')
FROM kb.object_nodes
WHERE object_id = ANY($1)
ORDER BY id
LIMIT $2`, pq.Array(objectIDs), metricGraphNodeLimit)
	if err != nil {
		return err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var (
			id                                     int64
			objectID, name, nameEn, otype, rstatus string
		)
		if err := rows.Scan(&id, &objectID, &name, &nameEn, &otype, &rstatus); err != nil {
			return err
		}
		out = append(out, map[string]any{
			"id":                id,
			"object_id":         objectID,
			"canonical_name":    emptyToNil(name),
			"canonical_name_en": emptyToNil(nameEn),
			"object_type":       emptyToNil(otype),
			"reconcile_status":  emptyToNil(rstatus),
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	g.Nodes["object__node"] = MetricGraphNode{Rows: nonNilRows(out)}
	return nil
}

func (s metricGraphStore) loadKeywordConcept(ctx context.Context, conceptID string, g *MetricGraph) error {
	if strings.TrimSpace(conceptID) == "" {
		return nil
	}
	var (
		id, label, status, scope string
		gloss                    sql.NullString
	)
	err := s.DB.QueryRowContext(ctx, `
SELECT concept_id, pref_label, status, scope, gloss
FROM kb.keyword_concepts
WHERE concept_id = $1`, conceptID).Scan(&id, &label, &status, &scope, &gloss)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	g.Nodes["keyword__concept"] = MetricGraphNode{Rows: []map[string]any{{
		"concept_id": id,
		"pref_label": emptyToNil(label),
		"status":     emptyToNil(status),
		"scope":      emptyToNil(scope),
		"gloss":      nullString(gloss),
	}}}
	return nil
}

func (s metricGraphStore) loadDecisionCandidates(ctx context.Context, metricID string, g *MetricGraph) ([]int64, error) {
	cands, _, err := (assertions.DecisionCandidateStore{DB: s.DB}).List(ctx, assertions.DecisionCandidateListFilter{
		SourceArtifactType: "metric",
		SourceArtifactID:   metricID,
		PageSize:           metricGraphNodeLimit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(cands))
	var assertionIDs []int64
	for _, c := range cands {
		out = append(out, map[string]any{
			"id":                     c.ID,
			"candidate_kind":         emptyToNil(c.CandidateKind),
			"logical_identity_key":   emptyToNil(c.LogicalIdentityKey),
			"status":                 emptyToNil(c.Status),
			"resulting_assertion_id": nullInt64Ptr(c.ResultingAssertionID),
		})
		if c.ResultingAssertionID != nil && *c.ResultingAssertionID > 0 {
			assertionIDs = append(assertionIDs, *c.ResultingAssertionID)
		}
	}
	g.Nodes["ev__dc"] = MetricGraphNode{Rows: nonNilRows(out)}
	return dedupeInt64(assertionIDs), nil
}

// loadResultingAssertions fills ev__sa and proc__project, and returns the
// class identity term ids (for mdef__contract) and the unit / quantity-kind /
// assertion-kind term ids (for mdef__term) carried on those assertions.
func (s metricGraphStore) loadResultingAssertions(ctx context.Context, ids []int64, g *MetricGraph) (instanceTermIDs, otherTermIDs []string, err error) {
	if len(ids) == 0 {
		return nil, nil, nil
	}
	store := assertions.AssertionStore{DB: s.DB}
	saRows := make([]map[string]any, 0, len(ids))
	projRows := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		a, gErr := store.GetByID(ctx, id)
		if gErr != nil {
			if errors.Is(gErr, sql.ErrNoRows) {
				continue
			}
			return nil, nil, gErr
		}
		saRows = append(saRows, map[string]any{
			"id":                     a.ID,
			"subject":                firstNonEmpty(a.SubjectRefID, a.SubjectObjectID),
			"predicate_term_id":      emptyToNil(a.PredicateTermID),
			"object":                 firstNonEmpty(a.ObjectRefID, string(a.ObjectLiteral)),
			"assertion_kind_term_id": emptyToNil(a.AssertionKindTermID),
			"confidence":             nullFloatPtr(a.Confidence),
		})
		projRows = append(projRows, map[string]any{
			"assertion_id":              a.ID,
			"status":                    emptyToNil(a.Status),
			"value_state_term_id":       emptyToNil(a.ValueStateTermID),
			"conformance_state_term_id": emptyToNil(a.ConformanceStateTermID),
			"normalized_against_contract_revision_id": nullInt64Ptr(a.NormalizedAgainstContractRevisionID),
		})
		if a.InstanceOfTermID != "" {
			instanceTermIDs = append(instanceTermIDs, a.InstanceOfTermID)
		}
		for _, t := range []string{a.UnitTermID, a.QuantityKindTermID, a.AssertionKindTermID} {
			if t != "" {
				otherTermIDs = append(otherTermIDs, t)
			}
		}
	}
	g.Nodes["ev__sa"] = MetricGraphNode{Rows: nonNilRows(saRows)}
	g.Nodes["proc__project"] = MetricGraphNode{Rows: nonNilRows(projRows)}
	return instanceTermIDs, otherTermIDs, nil
}

func (s metricGraphStore) loadOntologyTerms(ctx context.Context, termIDs []string, g *MetricGraph) error {
	if len(termIDs) == 0 {
		return nil
	}
	rows, err := s.DB.QueryContext(ctx, `
SELECT DISTINCT ON (term_id) term_id, term_kind, module_id, status, COALESCE(definition, '')
FROM kb.ontology_terms
WHERE term_id = ANY($1)
ORDER BY term_id, version DESC
LIMIT $2`, pq.Array(termIDs), metricGraphNodeLimit)
	if err != nil {
		return err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var termID, kind, module, status, definition string
		if err := rows.Scan(&termID, &kind, &module, &status, &definition); err != nil {
			return err
		}
		out = append(out, map[string]any{
			"term_id":    termID,
			"term_kind":  emptyToNil(kind),
			"module_id":  emptyToNil(module),
			"status":     emptyToNil(status),
			"definition": emptyToNil(definition),
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	g.Nodes["mdef__term"] = MetricGraphNode{Rows: nonNilRows(out)}
	return nil
}

func (s metricGraphStore) loadClassContracts(ctx context.Context, termIDs []string, g *MetricGraph) error {
	if len(termIDs) == 0 {
		return nil
	}
	store := classfoundation.ContractStore{DB: s.DB}
	var out []map[string]any
	for _, termID := range termIDs {
		rev, ok, err := store.Current(ctx, termID)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		out = append(out, map[string]any{
			"id":               rev.ID,
			"term_id":          rev.TermID,
			"revision":         rev.Revision,
			"definition_state": emptyToNil(rev.DefinitionState),
			"effective_from":   rev.CreateTime.Format("2006-01-02"),
		})
	}
	g.Nodes["mdef__contract"] = MetricGraphNode{Rows: nonNilRows(out)}
	return nil
}

func (s metricGraphStore) loadEvidence(ctx context.Context, metricID string, g *MetricGraph) error {
	ev, _, err := (assertions.EvidenceStore{DB: s.DB}).ListAdmin(ctx, assertions.EvidenceListFilter{
		ArtifactType: "metric",
		ArtifactID:   metricID,
		PageSize:     metricGraphNodeLimit,
	})
	if err != nil {
		return err
	}
	out := make([]map[string]any, 0, len(ev))
	for _, e := range ev {
		out = append(out, map[string]any{
			"id":                 e.ID,
			"assertion_id":       e.AssertionID,
			"input_record_id":    nullInt64Ptr(e.InputRecordID),
			"artifact_object_id": emptyToNil(e.ArtifactObjectID),
			"evidence_quote":     emptyToNil(e.EvidenceQuote),
			"source_line_spans":  rawJSONOrNil(e.SourceLineSpans),
		})
	}
	g.Nodes["ev__ae"] = MetricGraphNode{Rows: nonNilRows(out)}
	return nil
}

// loadProcessingOutcomes fills proc__normalize (stage_normalize) and
// proc__associate (stage_class_resolution + stage_associate) from the metric's
// active kb.semantic_processing_outcomes rows (design D10).
func (s metricGraphStore) loadProcessingOutcomes(ctx context.Context, metricID string, g *MetricGraph) error {
	rows, err := s.DB.QueryContext(ctx, `
SELECT stage_term_id, COALESCE(disposition_term_id, ''), execution_status,
       outcome_category, finding_count, COALESCE(highest_severity_term_id, ''), create_time
FROM kb.semantic_processing_outcomes
WHERE artifact_type = 'metric' AND artifact_id = $1 AND active = true
ORDER BY create_time, id
LIMIT $2`, metricID, metricGraphNodeLimit)
	if err != nil {
		return err
	}
	defer rows.Close()
	normalize := []map[string]any{}
	associate := []map[string]any{}
	for rows.Next() {
		var (
			stage, disposition, execStatus, category, severity string
			findingCount                                       int
			createTime                                         sql.NullTime
		)
		if err := rows.Scan(&stage, &disposition, &execStatus, &category, &findingCount, &severity, &createTime); err != nil {
			return err
		}
		row := map[string]any{
			"stage":            stage,
			"disposition":      emptyToNil(disposition),
			"execution_status": execStatus,
			"outcome_category": category,
			"finding_count":    findingCount,
			"highest_severity": emptyToNil(severity),
		}
		switch stage {
		case semantic.StageNormalize:
			normalize = append(normalize, row)
		case semantic.StageClassResolution, semantic.StageAssociate:
			associate = append(associate, row)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	g.Nodes["proc__normalize"] = MetricGraphNode{Rows: normalize}
	g.Nodes["proc__associate"] = MetricGraphNode{Rows: associate}
	return nil
}

// --- small scan/shape helpers, local to the metric-graph store ---

func emptyToNil(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func nullString(ns sql.NullString) any {
	if !ns.Valid || strings.TrimSpace(ns.String) == "" {
		return nil
	}
	return ns.String
}

func nullFloat(nf sql.NullFloat64) any {
	if !nf.Valid {
		return nil
	}
	return nf.Float64
}

func nullFloatPtr(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

func nullInt64Ptr(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

func nullTimeString(nt sql.NullTime) any {
	if !nt.Valid {
		return nil
	}
	return nt.Time.Format("2006-01-02T15:04:05Z07:00")
}

// rawJSONOrNil keeps a JSONB column's structure in the payload, or nil when it
// is empty / a JSON null.
func rawJSONOrNil(raw json.RawMessage) any {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return nil
	}
	return raw
}

// nonNilRows guarantees a node always serializes as a JSON array, never null.
func nonNilRows(rows []map[string]any) []map[string]any {
	if rows == nil {
		return []map[string]any{}
	}
	return rows
}

func dedupeNonEmpty(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func dedupeInt64(in []int64) []int64 {
	seen := make(map[int64]struct{}, len(in))
	out := make([]int64, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
