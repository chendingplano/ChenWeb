package docprocessing

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

const (
	defaultEntityObjectResolveMinConfidence = 0.85
	defaultEntityObjectResolveMaxAttempts   = 3
)

// EntityObjectClassification is the validated shape of one classifier
// verdict for an entity, per ADR 2026070101 Phase 4 §Classifier Contract.
type EntityObjectClassification struct {
	Decision         string // "exclude" | "associate" | "uncertain"
	Confidence       float64
	Rationale        string
	SelectedObjectID string // set only when Decision=associate and an existing candidate was chosen
	ObjectType       string // set only when Decision=associate and no candidate matched (used for CreateNode)
	ModelName        string
}

// EntityObjectClassifier is the LLM seam: given an entity and its current
// kb.object_nodes candidates, decide whether it should be excluded from
// object-linking permanently, associated with an object (existing or new),
// or left uncertain for a future retry.
type EntityObjectClassifier interface {
	ClassifyEntityForObjectLink(ctx context.Context, e pendingEntityRow, candidates []ObjectNodeCandidate) (EntityObjectClassification, error)
}

// EntityObjectResolveRow is one kb.entities row eligible for Phase 4
// resolution (object_link_status IN ('pending', 'deferred')), along with the
// retry-gating state needed to decide whether a classifier call is even
// warranted.
type EntityObjectResolveRow struct {
	Entity      pendingEntityRow
	Attempts    int
	Fingerprint string
}

// EntityObjectResolveStore is the DB seam ResolveEntityObjects needs beyond
// ObjectNodeStore and ArtifactObjectSingleInserter.
type EntityObjectResolveStore interface {
	LoadResolvable(ctx context.Context, limit int) ([]EntityObjectResolveRow, error)
	MarkExcluded(ctx context.Context, entityID string) error
	MarkLinked(ctx context.Context, entityID string) error
	MarkAttempted(ctx context.Context, entityID, status string, attempts int, fingerprint string) error
}

// ArtifactObjectSingleInserter is the persistence seam for Phase 4's
// cross-record, one-entity-at-a-time writes. ArtifactObjectSQLStore.InsertOne
// already implements this; declared as an interface so orchestration is
// testable without a database. Unlike ArtifactObjectPersister
// (ReplaceObjectsForRecord), this never deletes sibling rows — see
// entity_object_reconciliation_sql_test.go for why that distinction matters
// here.
type ArtifactObjectSingleInserter interface {
	InsertOne(ctx context.Context, obj ArtifactObject) error
}

// EntityObjectResolveConfig tunes ResolveEntityObjects. Zero values fall
// back to the same defaults DR7 (ADR 2026070701) established for the
// analogous artifact-object ambiguous-resolution job.
type EntityObjectResolveConfig struct {
	MinConfidence float64
	MaxAttempts   int
}

// EntityObjectResolveResult summarizes one call to ResolveEntityObjects.
type EntityObjectResolveResult struct {
	Scanned          int
	Classified       int
	SkippedUnchanged int
	Excluded         int
	Linked           int
	Deferred         int
	Exhausted        int
	Failed           int
}

// classificationTier collapses one classifier verdict into the three-way
// decision ADR 2026070101 Phase 4 specifies. A stated exclude/associate
// below minConfidence is treated as uncertain, never silently downgraded to
// a forced choice — mirrors ADR 2026070701 DR7's "must not silently force a
// low-confidence choice."
func classificationTier(decision string, confidence, minConfidence float64) string {
	if confidence < minConfidence {
		return "uncertain"
	}
	switch strings.ToLower(strings.TrimSpace(decision)) {
	case "exclude":
		return "exclude"
	case "associate":
		return "associate"
	default:
		return "uncertain"
	}
}

// nextDeferredStatus decides whether an uncertain (or failed) genuine
// classification attempt leaves the entity deferred for a future retry or
// exhausted at the attempt cap. attemptsAfter is the count *after*
// incrementing for this attempt.
func nextDeferredStatus(attemptsAfter, maxAttempts int) string {
	if attemptsAfter >= maxAttempts {
		return entityObjectLinkExhausted
	}
	return entityObjectLinkDeferred
}

// computeEntityObjectFingerprint hashes exactly what would be sent to the
// classifier: the entity's identity signature and the current candidate
// object_id set. Order-independent in the candidate set, so a fresh
// FindCandidates call that returns the same set in a different order does
// not spuriously look like a change. See ADR 2026070101 Phase 4
// §Fingerprint-Gated Retry.
func computeEntityObjectFingerprint(e pendingEntityRow, candidateObjectIDs []string) string {
	aliases := sortedCopy(e.Aliases)
	aliasesEn := sortedCopy(e.AliasesEN)
	candidates := sortedCopy(candidateObjectIDs)

	h := sha1.New()
	fmt.Fprintf(h, "entity|%s|%s|%s|%s|%s|%s|%s|%s\n",
		e.Entity, e.EntityEN, e.EntityType, e.EntityTypeEN,
		e.Desc, e.DescEN,
		strings.Join(aliases, ","), strings.Join(aliasesEn, ","),
	)
	fmt.Fprintf(h, "candidates|%s\n", strings.Join(candidates, ","))
	return hex.EncodeToString(h.Sum(nil))
}

func sortedCopy(in []string) []string {
	out := append([]string{}, in...)
	sort.Strings(out)
	return out
}

func candidateObjectIDsOf(candidates []ObjectNodeCandidate) []string {
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, c.Node.ObjectID)
	}
	return out
}

func candidateIDPresent(candidates []ObjectNodeCandidate, id string) bool {
	for _, c := range candidates {
		if c.Node.ObjectID == id {
			return true
		}
	}
	return false
}

// ResolveEntityObjects implements ADR 2026070101 Phase 4: for each
// kb.entities row at object_link_status IN ('pending', 'deferred'), cheaply
// re-fetch candidates and gate on the input fingerprint before spending an
// LLM call (§Fingerprint-Gated Retry). A genuine classification then maps to
// exclude (terminal) / associate (persist a kb.artifact_objects row, terminal
// on success) / uncertain (deferred, or exhausted at the attempt cap).
//
// Call repeatedly with a bounded limit to drain the backlog, the same
// operational convention as /kb/objects/resolve-ambiguous
// (ADR 2026070701 DR5).
func ResolveEntityObjects(
	ctx context.Context,
	store EntityObjectResolveStore,
	objectStore ArtifactObjectSingleInserter,
	reconciler ObjectReconciler,
	classifier EntityObjectClassifier,
	cfg EntityObjectResolveConfig,
	limit int,
	logger ApiTypes.JimoLogger,
) (EntityObjectResolveResult, error) {
	var res EntityObjectResolveResult

	minConfidence := cfg.MinConfidence
	if minConfidence <= 0 {
		minConfidence = defaultEntityObjectResolveMinConfidence
	}
	maxAttempts := cfg.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultEntityObjectResolveMaxAttempts
	}

	rows, err := store.LoadResolvable(ctx, limit)
	if err != nil {
		return res, err
	}
	res.Scanned = len(rows)

	for _, row := range rows {
		entityID := row.Entity.EntityID

		objType, eligible := entityObjectTypeCandidate(row.Entity.EntityTypeEN, row.Entity.EntityType)
		if !eligible {
			// Defensive: Phase 3 should already have excluded type-ineligible
			// entities before they ever reach object_link_status='pending',
			// but a caller invoking Phase 4 out of order must not crash or
			// silently mis-file this row.
			if err := store.MarkExcluded(ctx, entityID); err != nil {
				res.Failed++
				continue
			}
			res.Excluded++
			continue
		}

		obj := entityToArtifactObjectCandidate(row.Entity, objType)
		candidates, err := reconciler.Store.FindCandidates(ctx, obj, reconciler.Options)
		if err != nil {
			res.Failed++
			if logger != nil {
				logger.Warn("resolve entity object: find candidates failed", "entity_id", entityID, "error", err.Error())
			}
			continue
		}

		fingerprint := computeEntityObjectFingerprint(row.Entity, candidateObjectIDsOf(candidates))
		if row.Fingerprint != "" && fingerprint == row.Fingerprint {
			res.SkippedUnchanged++
			continue
		}

		res.Classified++
		cls, clsErr := classifier.ClassifyEntityForObjectLink(ctx, row.Entity, candidates)
		if clsErr != nil {
			res.Failed++
			if logger != nil {
				logger.Warn("resolve entity object: classifier failed", "entity_id", entityID, "error", clsErr.Error())
			}
			attempts := row.Attempts + 1
			status := nextDeferredStatus(attempts, maxAttempts)
			if err := store.MarkAttempted(ctx, entityID, status, attempts, fingerprint); err != nil && logger != nil {
				logger.Warn("resolve entity object: mark attempted failed", "entity_id", entityID, "error", err.Error())
			}
			if status == entityObjectLinkExhausted {
				res.Exhausted++
			} else {
				res.Deferred++
			}
			continue
		}

		switch classificationTier(cls.Decision, cls.Confidence, minConfidence) {
		case "exclude":
			if err := store.MarkExcluded(ctx, entityID); err != nil {
				res.Failed++
				continue
			}
			res.Excluded++

		case "associate":
			if _, linkErr := linkEntityToObject(ctx, reconciler.Store, objectStore, obj, candidates, cls); linkErr != nil {
				res.Failed++
				if logger != nil {
					logger.Warn("resolve entity object: link failed", "entity_id", entityID, "error", linkErr.Error())
				}
				continue
			}
			if err := store.MarkLinked(ctx, entityID); err != nil {
				res.Failed++
				continue
			}
			res.Linked++

		default: // uncertain
			attempts := row.Attempts + 1
			status := nextDeferredStatus(attempts, maxAttempts)
			if err := store.MarkAttempted(ctx, entityID, status, attempts, fingerprint); err != nil {
				res.Failed++
				continue
			}
			if status == entityObjectLinkExhausted {
				res.Exhausted++
			} else {
				res.Deferred++
			}
		}
	}
	return res, nil
}

// linkEntityToObject applies an "associate" classification: reuse the
// already-selected existing candidate if the classifier named one that is
// actually in the candidate set (never trust an id outside it), otherwise
// create a new kb.object_nodes row. Phase 4's own classifier already is the
// ambiguity-adjudication step (it saw the same candidates DR7's adjudicator
// would), so this does not layer a second LLM decision on top via
// reconcileArtifactObjectsWithLLM — it applies the one decision already made.
func linkEntityToObject(ctx context.Context, nodeStore ObjectNodeStore, objectStore ArtifactObjectSingleInserter, obj ArtifactObject, candidates []ObjectNodeCandidate, cls EntityObjectClassification) (string, error) {
	objectID := strings.TrimSpace(cls.SelectedObjectID)
	method := "llm_associate_existing"
	if objectID != "" && !candidateIDPresent(candidates, objectID) {
		objectID = ""
	}
	if objectID == "" {
		if t := strings.TrimSpace(cls.ObjectType); t != "" {
			obj.ObjectType = normalizeObjectToken(t)
		}
		node, err := nodeStore.CreateNode(ctx, obj)
		if err != nil {
			return "", err
		}
		objectID = node.ObjectID
		method = "llm_associate_new_node"
	}

	obj.ObjectID = objectID
	obj.ReconcileStatus = ObjectReconcileMatched
	obj.ReconcileConfidence = cls.Confidence
	obj.ExtInfo["reconcile_method"] = method
	if err := objectStore.InsertOne(ctx, obj); err != nil {
		return "", err
	}
	return objectID, nil
}

// EntityObjectResolveSQLStore implements EntityObjectResolveStore against
// kb.entities.
type EntityObjectResolveSQLStore struct {
	DB *sql.DB
}

// LoadResolvable loads entities at object_link_status IN ('pending',
// 'deferred') — the two non-terminal states — bounded by limit.
func (s EntityObjectResolveSQLStore) LoadResolvable(ctx context.Context, limit int) ([]EntityObjectResolveRow, error) {
	if s.DB == nil {
		return nil, fmt.Errorf("db is nil")
	}
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.DB.QueryContext(ctx, `
SELECT e.entity_id, e.input_record_id, e.entity, COALESCE(e.entity_en, ''),
       COALESCE(e.entity_type, ''), COALESCE(e.entity_type_en, ''),
       COALESCE(e.aliases, '[]'::jsonb), COALESCE(e.aliases_en, '[]'::jsonb),
       COALESCE(e.desc_text, ''), COALESCE(e.desc_text_en, ''),
       e.object_link_attempts, COALESCE(e.object_link_fingerprint, '')
FROM kb.entities e
WHERE e.object_link_status IN ($1, $2)
ORDER BY e.id
LIMIT $3`, entityObjectLinkPending, entityObjectLinkDeferred, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []EntityObjectResolveRow
	for rows.Next() {
		var (
			row                      EntityObjectResolveRow
			aliasesRaw, aliasesEnRaw []byte
		)
		if err := rows.Scan(
			&row.Entity.EntityID, &row.Entity.InputRecordID, &row.Entity.Entity, &row.Entity.EntityEN,
			&row.Entity.EntityType, &row.Entity.EntityTypeEN,
			&aliasesRaw, &aliasesEnRaw,
			&row.Entity.Desc, &row.Entity.DescEN,
			&row.Attempts, &row.Fingerprint,
		); err != nil {
			return nil, err
		}
		row.Entity.Aliases = parseJSONStringArray(aliasesRaw)
		row.Entity.AliasesEN = parseJSONStringArray(aliasesEnRaw)
		out = append(out, row)
	}
	return out, rows.Err()
}

// MarkExcluded sets a terminal, never-revisited status.
func (s EntityObjectResolveSQLStore) MarkExcluded(ctx context.Context, entityID string) error {
	return s.setStatus(ctx, entityID, entityObjectLinkExcluded)
}

// MarkLinked sets a terminal, successful status.
func (s EntityObjectResolveSQLStore) MarkLinked(ctx context.Context, entityID string) error {
	return s.setStatus(ctx, entityID, entityObjectLinkLinked)
}

func (s EntityObjectResolveSQLStore) setStatus(ctx context.Context, entityID, status string) error {
	if s.DB == nil {
		return fmt.Errorf("db is nil")
	}
	_, err := s.DB.ExecContext(ctx, `
UPDATE kb.entities
SET object_link_status = $1
WHERE entity_id = $2`, status, entityID)
	return err
}

// MarkAttempted records a genuine (fingerprint-changed) classification
// attempt that did not reach a terminal outcome: status is 'deferred' or
// 'exhausted', attempts and fingerprint are updated together, and
// object_link_last_attempt_at is stamped so this is distinguishable from a
// skipped-unchanged pass, which touches none of these fields.
func (s EntityObjectResolveSQLStore) MarkAttempted(ctx context.Context, entityID, status string, attempts int, fingerprint string) error {
	if s.DB == nil {
		return fmt.Errorf("db is nil")
	}
	_, err := s.DB.ExecContext(ctx, `
UPDATE kb.entities
SET object_link_status = $1,
    object_link_attempts = $2,
    object_link_fingerprint = $3,
    object_link_last_attempt_at = NOW()
WHERE entity_id = $4`, status, attempts, fingerprint, entityID)
	return err
}
