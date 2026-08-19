package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/lib/pq"
)

type ObjectNodeSQLStore struct {
	DB *sql.DB
}

func (s ObjectNodeSQLStore) FindCandidates(ctx context.Context, obj ArtifactObject, opts ObjectReconcileOptions) ([]ObjectNodeCandidate, error) {
	if s.DB == nil {
		return nil, fmt.Errorf("db is nil")
	}
	names := uniqueStrings(obj.NormalizedNames)
	if len(names) == 0 {
		return nil, nil
	}
	maxCandidates := opts.MaxCandidates
	if maxCandidates <= 0 {
		maxCandidates = 10
	}
	rows, err := s.DB.QueryContext(ctx, `
SELECT id,
       object_id,
       COALESCE(canonical_object_id, ''),
       COALESCE(canonical_name, ''),
       COALESCE(canonical_name_en, ''),
       COALESCE(canonical_name_zh, ''),
       COALESCE(primary_language, ''),
       COALESCE(object_type, ''),
       COALESCE(aliases, '[]'::jsonb),
       COALESCE(acronyms, '[]'::jsonb),
       COALESCE(normalized_names, '[]'::jsonb),
       COALESCE(description, ''),
       COALESCE(search_document, ''),
       COALESCE(reconcile_status, ''),
       COALESCE(ext_info, '{}'::jsonb)
FROM kb.object_nodes
WHERE reconcile_status NOT IN ('rejected', 'merged')
  AND (normalized_names ?| $1 OR canonical_name = $2 OR canonical_name_en = $2 OR canonical_name_zh = $2)
ORDER BY CASE WHEN object_type = $3 THEN 0 ELSE 1 END, id
LIMIT $4`, pq.Array(names), obj.ObjectName, obj.ObjectType, maxCandidates)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	nodes, err := scanObjectNodes(rows)
	if err != nil {
		return nil, err
	}

	out := make([]ObjectNodeCandidate, 0, len(nodes))
	for _, node := range nodes {
		score := 0.85
		method := "lexical_name"
		if objectTypesCompatible(obj.ObjectType, node.ObjectType) && objectNameBundlesOverlap(obj.NormalizedNames, node.NormalizedNames) {
			score = 1
			method = "exact_name"
		}
		out = append(out, ObjectNodeCandidate{Node: node, Score: score, Method: method})
	}
	return out, nil
}

// FindByCanonicalName returns every kb.object_nodes row whose canonical_name
// exactly matches name. Used by the "Create New" action on the Resolve
// Ambiguous Objects admin page to avoid creating a duplicate node for a name
// that already exists.
func (s ObjectNodeSQLStore) FindByCanonicalName(ctx context.Context, name string) ([]ObjectNode, error) {
	if s.DB == nil {
		return nil, fmt.Errorf("db is nil")
	}
	rows, err := s.DB.QueryContext(ctx, `
SELECT id,
       object_id,
       COALESCE(canonical_object_id, ''),
       COALESCE(canonical_name, ''),
       COALESCE(canonical_name_en, ''),
       COALESCE(canonical_name_zh, ''),
       COALESCE(primary_language, ''),
       COALESCE(object_type, ''),
       COALESCE(aliases, '[]'::jsonb),
       COALESCE(acronyms, '[]'::jsonb),
       COALESCE(normalized_names, '[]'::jsonb),
       COALESCE(description, ''),
       COALESCE(search_document, ''),
       COALESCE(reconcile_status, ''),
       COALESCE(ext_info, '{}'::jsonb)
FROM kb.object_nodes
WHERE canonical_name = $1
ORDER BY object_id`, name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return scanObjectNodes(rows)
}

// scanObjectNodes scans the shared column set used by FindCandidates and
// FindByCanonicalName into ObjectNode rows.
func scanObjectNodes(rows *sql.Rows) ([]ObjectNode, error) {
	var out []ObjectNode
	for rows.Next() {
		var (
			node                    ObjectNode
			aliasesRaw, acronymsRaw []byte
			namesRaw, extRaw        []byte
		)
		if err := rows.Scan(
			&node.ID,
			&node.ObjectID,
			&node.CanonicalObjectID,
			&node.CanonicalName,
			&node.CanonicalNameEn,
			&node.CanonicalNameZh,
			&node.PrimaryLanguage,
			&node.ObjectType,
			&aliasesRaw,
			&acronymsRaw,
			&namesRaw,
			&node.Description,
			&node.SearchDocument,
			&node.ReconcileStatus,
			&extRaw,
		); err != nil {
			return nil, err
		}
		node.Aliases = jsonStringArray(aliasesRaw)
		node.Acronyms = jsonStringArray(acronymsRaw)
		node.NormalizedNames = jsonStringArray(namesRaw)
		_ = json.Unmarshal(extRaw, &node.ExtInfo)
		out = append(out, node)
	}
	return out, rows.Err()
}

func (s ObjectNodeSQLStore) CreateNode(ctx context.Context, obj ArtifactObject) (ObjectNode, error) {
	if s.DB == nil {
		return ObjectNode{}, fmt.Errorf("db is nil")
	}
	node := ObjectNode{
		ObjectID:          buildObjectID(obj.InputRecordID, firstNonEmptyTrimmed(obj.ObjectName, obj.ObjectNameEn, obj.ObjectNameZh), 1),
		CanonicalName:     firstNonEmptyTrimmed(obj.ObjectName, obj.ObjectNameEn, obj.ObjectNameZh),
		CanonicalNameEn:   obj.ObjectNameEn,
		CanonicalNameZh:   obj.ObjectNameZh,
		PrimaryLanguage:   obj.Language,
		ObjectType:        firstNonEmptyTrimmed(obj.ObjectType, "other"),
		Aliases:           obj.Aliases,
		Acronyms:          obj.Acronyms,
		NormalizedNames:   obj.NormalizedNames,
		Description:       obj.Description,
		SearchDocument:    buildObjectNodeSearchDocument(obj),
		ReconcileStatus:   "active",
		ExtInfo:           map[string]any{"source": "object_reconciliation"},
		CanonicalObjectID: "",
	}
	aliases, _ := json.Marshal(orEmptySlice(node.Aliases))
	acronyms, _ := json.Marshal(orEmptySlice(node.Acronyms))
	names, _ := json.Marshal(orEmptySlice(node.NormalizedNames))
	ext, _ := json.Marshal(node.ExtInfo)
	err := s.DB.QueryRowContext(ctx, `
INSERT INTO kb.object_nodes (
	object_id, canonical_name, canonical_name_en, canonical_name_zh,
	primary_language, object_type, aliases, acronyms, normalized_names,
	description, search_document, reconcile_status, ext_info
) VALUES (
	$1,$2,$3,$4,$5,$6,$7::jsonb,$8::jsonb,$9::jsonb,$10,$11,$12,$13::jsonb
)
ON CONFLICT (object_id) DO UPDATE SET
	normalized_names = (
		COALESCE(
			(
				SELECT jsonb_agg(DISTINCT value)
				FROM jsonb_array_elements_text(kb.object_nodes.normalized_names || EXCLUDED.normalized_names) AS t(value)
			),
			'[]'::jsonb
		)
	),
	aliases = (
		COALESCE(
			(
				SELECT jsonb_agg(DISTINCT value)
				FROM jsonb_array_elements_text(kb.object_nodes.aliases || EXCLUDED.aliases) AS t(value)
			),
			'[]'::jsonb
		)
	),
	acronyms = (
		COALESCE(
			(
				SELECT jsonb_agg(DISTINCT value)
				FROM jsonb_array_elements_text(kb.object_nodes.acronyms || EXCLUDED.acronyms) AS t(value)
			),
			'[]'::jsonb
		)
	),
	search_document = EXCLUDED.search_document
RETURNING object_id`,
		node.ObjectID,
		node.CanonicalName,
		nullEmpty(node.CanonicalNameEn),
		nullEmpty(node.CanonicalNameZh),
		nullEmpty(node.PrimaryLanguage),
		node.ObjectType,
		string(aliases),
		string(acronyms),
		string(names),
		nullEmpty(node.Description),
		node.SearchDocument,
		node.ReconcileStatus,
		string(ext),
	).Scan(&node.ObjectID)
	if err != nil {
		return ObjectNode{}, err
	}
	return node, nil
}

type AmbiguousObjectLLMResolver interface {
	ResolveAmbiguousObject(ctx context.Context, obj ArtifactObject, candidates []ObjectNodeCandidate) (AmbiguousObjectLLMDecision, error)
}

func reconcileArtifactObjects(ctx context.Context, objects []ArtifactObject, reconciler ObjectReconciler, logger ApiTypes.JimoLogger) ([]ArtifactObject, error) {
	return reconcileArtifactObjectsWithLLM(ctx, objects, reconciler, logger, nil, defaultResolveAmbiguousMinConf, objectReconcileLogSink{})
}

// resolveAmbiguousObjectConcurrency bounds the number of parallel LLM
// adjudication calls made during object reconciliation.
func resolveAmbiguousObjectConcurrency() int {
	// return envInt("RESOLVE_AMBIGUOUS_OBJECT_CONCURRENCY", 5, 1)
	return envInt("MAX_DOC_PROCESSOR_TASKS", 5, 1)
}

// reconcileObjectItem carries per-object reconcile state across the three phases.
type reconcileObjectItem struct {
	obj        ArtifactObject
	result     ObjectReconcileResult
	candidates []ObjectNodeCandidate
	ambiguous  bool // needs LLM adjudication (ambiguous AND a resolver is configured)
}

// ambiguousResolveResult is one ambiguous object's LLM adjudication outcome.
type ambiguousResolveResult struct {
	decision AmbiguousObjectLLMDecision
	err      error
	ms       int64
}

// reconcileArtifactObjectsWithLLM reconciles artifact objects against
// kb.object_nodes in three phases:
//
//  1. Sequential reconcile — ReconcileOne creates nodes on the no-match path, so
//     it must stay ordered (parallel runs would race same-named objects into
//     duplicate nodes).
//  2. Parallel LLM adjudication — ResolveAmbiguousObject is a pure, network-bound
//     call with no DB writes; this is the latency-dominant step, so it runs with
//     bounded concurrency.
//  3. Sequential apply + logging — decision application performs DB merges/updates
//     (kept ordered), emits per-object warnings, and records every LLM outcome
//     (resolved, unresolved, failed) to kb.doc_proc_logs via logSink.
func reconcileArtifactObjectsWithLLM(ctx context.Context, objects []ArtifactObject, reconciler ObjectReconciler, logger ApiTypes.JimoLogger, llmResolver AmbiguousObjectLLMResolver, minConfidence float64, logSink objectReconcileLogSink) ([]ArtifactObject, error) {
	out := make([]ArtifactObject, len(objects))
	items := make([]reconcileObjectItem, len(objects))

	// Phase 1 (sequential): reconcile every object and collect the ambiguous ones.
	var ambiguousIdx []int
	for i, obj := range objects {
		result, err := reconciler.ReconcileOne(ctx, obj)
		if err != nil {
			return nil, err
		}
		it := reconcileObjectItem{obj: obj, result: result}
		if result.Status == ObjectReconcileAmbiguous {
			candidates, candErr := reconciler.Store.FindCandidates(ctx, obj, reconciler.Options)
			if candErr != nil {
				return nil, candErr
			}
			it.candidates = candidates
			if llmResolver != nil {
				it.ambiguous = true
				ambiguousIdx = append(ambiguousIdx, i)
			}
		}
		items[i] = it
	}

	// Phase 2 (parallel): adjudicate the ambiguous objects via the LLM.
	resolved := make([]ambiguousResolveResult, len(ambiguousIdx))
	if len(ambiguousIdx) > 0 {
		concurrency := resolveAmbiguousObjectConcurrency()
		if logger != nil {
			logger.Info("object reconciliation: resolving ambiguous objects",
				"count", len(ambiguousIdx), "concurrency", concurrency)
		}
		res, err := runConcurrent(ctx, concurrency, len(ambiguousIdx),
			func(ctx context.Context, k int) (ambiguousResolveResult, error) {
				it := items[ambiguousIdx[k]]
				start := time.Now()
				decision, resolveErr := llmResolver.ResolveAmbiguousObject(ctx, it.obj, it.candidates)
				ms := time.Since(start).Milliseconds()
				if logger != nil {
					if resolveErr != nil {
						logger.Warn("object reconciliation: LLM call failed",
							"artifact_id", it.obj.ArtifactID, "ms_used", ms, "err", resolveErr)
					} else {
						logger.Info("object reconciliation: LLM call done",
							"artifact_id", it.obj.ArtifactID, "ms_used", ms,
							"confidence", decision.ResolutionConfidence)
					}
				}
				return ambiguousResolveResult{decision: decision, err: resolveErr, ms: ms}, nil
			})
		if err != nil {
			return nil, err
		}
		resolved = res
	}
	resolvedByIdx := make(map[int]ambiguousResolveResult, len(ambiguousIdx))
	for k, idx := range ambiguousIdx {
		resolvedByIdx[idx] = resolved[k]
	}

	// Phase 3 (sequential): apply decisions, warn, and log every LLM outcome.
	for i := range items {
		it := items[i]
		obj := it.obj

		if it.ambiguous {
			rr := resolvedByIdx[i]
			if rr.err != nil {
				if logger != nil {
					logger.Warn("LLM ambiguous object resolution failed",
						"artifact_id", obj.ArtifactID,
						"err", rr.err,
						"llm_response", structuredOutputRawResponse(rr.err),
					)
				}
				logSink.logReconcileOutcome(ctx, logger, objectReconcileOutcome{
					Status: reconcileOutcomeLLMFailed, Object: obj, Candidates: it.candidates,
					CandidateDisplay: objectReconcileCandidateDisplay(it.candidates),
					Err:              rr.err, MSUsed: rr.ms,
				})
			} else {
				var applyStore AmbiguousObjectLLMApplyStore
				if s, ok := reconciler.Store.(AmbiguousObjectLLMApplyStore); ok {
					applyStore = s
				}
				resolvedObj, applied, applyErr := ApplyAmbiguousObjectLLMDecision(ctx, obj, it.candidates, rr.decision, applyStore, minConfidence)
				if applyErr != nil {
					if logger != nil {
						logger.Warn("apply LLM ambiguous object resolution failed", "artifact_id", obj.ArtifactID, "err", applyErr)
					}
					logSink.logReconcileOutcome(ctx, logger, objectReconcileOutcome{
						Status: reconcileOutcomeApplyFailed, Object: obj, Candidates: it.candidates,
						CandidateDisplay: objectReconcileCandidateDisplay(it.candidates),
						Decision:         rr.decision, Err: applyErr, MSUsed: rr.ms,
					})
				} else if applied {
					logSink.logReconcileOutcome(ctx, logger, objectReconcileOutcome{
						Status: reconcileOutcomeResolved, Object: resolvedObj, Candidates: it.candidates,
						CandidateDisplay: objectReconcileCandidateDisplay(it.candidates),
						Decision:         rr.decision, ResolvedID: resolvedObj.ObjectID, MSUsed: rr.ms,
					})
					out[i] = resolvedObj
					continue
				} else {
					// LLM answered but confidence below threshold → left ambiguous.
					logSink.logReconcileOutcome(ctx, logger, objectReconcileOutcome{
						Status: reconcileOutcomeUnresolved, Object: obj, Candidates: it.candidates,
						CandidateDisplay: objectReconcileCandidateDisplay(it.candidates),
						Decision:         rr.decision, MSUsed: rr.ms,
					})
				}
			}
		}

		if it.result.Status == ObjectReconcileAmbiguous {
			names := objectReconcileCandidateDisplay(it.candidates)
			if logger != nil {
				logger.Warn("object reconciliation ambiguous",
					"input_record_id", obj.InputRecordID,
					"artifact_type", obj.ArtifactType,
					"artifact_id", obj.ArtifactID,
					"object_name", obj.ObjectName,
					"top_score", it.result.Confidence,
					"candidates", names,
				)
			}
			if !it.ambiguous {
				logSink.logReconcileOutcome(ctx, logger, objectReconcileOutcome{
					Status: reconcileOutcomeUnresolved, Object: obj, Candidates: it.candidates,
					CandidateDisplay: names,
				})
			}
		}
		obj.ObjectID = it.result.ObjectID
		obj.ReconcileStatus = it.result.Status
		obj.ReconcileConfidence = it.result.Confidence
		if obj.ExtInfo == nil {
			obj.ExtInfo = map[string]any{}
		}
		obj.ExtInfo["reconcile_method"] = it.result.Method
		out[i] = obj
	}
	return out, nil
}

func objectReconcileCandidateDisplay(candidates []ObjectNodeCandidate) []string {
	names := make([]string, 0, len(candidates))
	for _, c := range candidates {
		names = append(names, c.Node.ObjectID+":"+c.Node.CanonicalName)
	}
	return names
}

func buildObjectNodeSearchDocument(obj ArtifactObject) string {
	return joinUniqueSearchParts(
		obj.ObjectName,
		obj.ObjectNameEn,
		obj.ObjectNameZh,
		searchDocumentArrayText(obj.Aliases),
		searchDocumentArrayText(obj.Acronyms),
		obj.ObjectType,
		obj.ObjectRole,
		obj.Description,
		obj.EvidenceQuote,
	)
}

func jsonStringArray(raw []byte) []string {
	var out []string
	_ = json.Unmarshal(raw, &out)
	return uniqueStrings(out)
}

func ObjectReconcileOptionsFromEnv() ObjectReconcileOptions {
	return ObjectReconcileOptions{
		EmbeddingEnabled: strings.EqualFold(strings.TrimSpace(envString("OBJECT_RECONCILE_EMBEDDING_ENABLED", "false")), "true"),
		MaxCandidates:    envInt("OBJECT_RECONCILE_MAX_CANDIDATES", 10, 1),
	}
}

func envString(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}
