package keywords

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/chendingplano/deepdoc/server/api/ontology/semid"
)

const promotionSchemaVersion = 1

// ReviewedPromotionFile is the operator-authored reviewed mapping from staging
// catalog identities to established keyword concepts. It records full
// evidence triples and reviewer decisions; identity is never inferred from
// labels.
type ReviewedPromotionFile struct {
	SchemaVersion      int                 `json:"schema_version"`
	Source             string              `json:"source"`
	Release            string              `json:"release"`
	Scope              string              `json:"scope"`
	DeploymentKey      string              `json:"deployment_key"`
	Promotions         []PositivePromotion `json:"promotions"`
	NegativePromotions []NegativePromotion `json:"negative_promotions,omitempty"`
}

// PositivePromotion maps one staging catalog identity to an established
// keyword concept with full reviewer provenance. Identity is never inferred
// from labels; every promotion records the exact relation and evidence.
type PositivePromotion struct {
	ExternalID           string            `json:"external_id"`
	ConceptID            string            `json:"concept_id"`
	Relation             string            `json:"relation"`
	UnitConstraints      []string          `json:"unit_constraints,omitempty"`
	DimensionConstraints []string          `json:"dimension_constraints,omitempty"`
	Surfaces             []PromotedSurface `json:"surfaces"`
	Reviewer             string            `json:"reviewer"`
	ReviewedAt           time.Time         `json:"reviewed_at"`
	ProvenanceLocator    string            `json:"provenance_locator"`
}

// PromotedSurface names one reviewed local surface carried by a positive
// promotion and the reviewer's evidence string for it.
type PromotedSurface struct {
	SurfaceID string `json:"surface_id"`
	Evidence  string `json:"evidence"`
}

// NegativePromotion records a reviewed scoped non-equivalence between two
// established concepts as a provenance-linked never_merge veto.
type NegativePromotion struct {
	NodeA             string            `json:"node_a"`
	NodeB             string            `json:"node_b"`
	Scope             string            `json:"scope"`
	Reason            string            `json:"reason"`
	Triples           []PromotionTriple `json:"triples"`
	Reviewer          string            `json:"reviewer"`
	ReviewedAt        time.Time         `json:"reviewed_at"`
	ProvenanceLocator string            `json:"provenance_locator"`
}

// PromotionTriple is one authoritative staging decision row cited by a
// negative promotion's veto.
type PromotionTriple struct {
	Source            string `json:"source"`
	Release           string `json:"release"`
	SubjectExternalID string `json:"subject_external_id"`
	ObjectExternalID  string `json:"object_external_id"`
	Relation          string `json:"relation"`
	ProvenanceLocator string `json:"provenance_locator"`
}

// PromotionResult reports the stable counts of one reviewed promotion batch.
// Counts are per-file entries, so an exact replay produces identical counts.
type PromotionResult struct {
	Source          string `json:"source"`
	Release         string `json:"release"`
	ExternalIDs     int    `json:"external_ids"`
	SurfaceEvidence int    `json:"surface_evidence"`
	NeverMerges     int    `json:"never_merges"`
}

func (f ReviewedPromotionFile) Validate() error {
	if f.SchemaVersion != promotionSchemaVersion {
		return fmt.Errorf("unsupported promotion schema_version %d", f.SchemaVersion)
	}
	for _, required := range []struct{ name, value string }{
		{"source", f.Source}, {"release", f.Release}, {"scope", f.Scope}, {"deployment_key", f.DeploymentKey},
	} {
		if strings.TrimSpace(required.value) == "" {
			return fmt.Errorf("promotion %s is required", required.name)
		}
	}
	if len(f.Promotions) == 0 && len(f.NegativePromotions) == 0 {
		return errors.New("promotion requires at least one positive or negative entry")
	}
	for i, p := range f.Promotions {
		if strings.TrimSpace(p.ExternalID) == "" || strings.TrimSpace(p.ConceptID) == "" {
			return fmt.Errorf("promotion[%d] requires external_id and concept_id", i)
		}
		if p.Relation != string(IdentityRelationExactEquivalent) {
			return fmt.Errorf("promotion[%d] relation %q cannot be promoted as authoritative evidence", i, p.Relation)
		}
		if strings.TrimSpace(p.Reviewer) == "" || p.ReviewedAt.IsZero() || strings.TrimSpace(p.ProvenanceLocator) == "" {
			return fmt.Errorf("promotion[%d] requires reviewer, reviewed_at, and provenance_locator", i)
		}
		if len(p.Surfaces) == 0 {
			return fmt.Errorf("promotion[%d] requires at least one reviewed surface", i)
		}
		for j, surface := range p.Surfaces {
			if strings.TrimSpace(surface.SurfaceID) == "" {
				return fmt.Errorf("promotion[%d] surface[%d] requires surface_id", i, j)
			}
		}
	}
	for i, n := range f.NegativePromotions {
		if strings.TrimSpace(n.NodeA) == "" || strings.TrimSpace(n.NodeB) == "" || strings.TrimSpace(n.Scope) == "" {
			return fmt.Errorf("negative promotion[%d] requires node_a, node_b, and scope", i)
		}
		if n.NodeA == n.NodeB {
			return fmt.Errorf("negative promotion[%d] node_a and node_b must be distinct", i)
		}
		if strings.TrimSpace(n.Reason) == "" || strings.TrimSpace(n.Reviewer) == "" || n.ReviewedAt.IsZero() ||
			strings.TrimSpace(n.ProvenanceLocator) == "" {
			return fmt.Errorf("negative promotion[%d] requires reason, reviewer, reviewed_at, and provenance_locator", i)
		}
		if len(n.Triples) == 0 {
			return fmt.Errorf("negative promotion[%d] requires at least one evidence triple", i)
		}
		for j, triple := range n.Triples {
			if strings.TrimSpace(triple.Source) == "" || strings.TrimSpace(triple.Release) == "" ||
				strings.TrimSpace(triple.SubjectExternalID) == "" || strings.TrimSpace(triple.ObjectExternalID) == "" ||
				strings.TrimSpace(triple.Relation) == "" || strings.TrimSpace(triple.ProvenanceLocator) == "" {
				return fmt.Errorf("negative promotion[%d] triple[%d] requires full provenance", i, j)
			}
		}
	}
	return nil
}

const (
	promotionConceptSQL                 = `SELECT scope, status FROM kb.keyword_concepts WHERE concept_id = $1`
	promotionPolicySQL                  = `SELECT authority_role, license_review_status, authoritative_relations, allowed_scopes, identity_authority FROM kb.keyword_sources WHERE source = $1 AND release = $2`
	promotionDeploymentSQL              = `SELECT source, release, enabled FROM kb.keyword_identity_deployments WHERE deployment_key = $1`
	promotionStagingEntrySQL            = `SELECT native_payload FROM kb.keyword_catalog_entries WHERE source = $1 AND release = $2 AND external_id = $3`
	promotionExistingMappingSQL         = `SELECT concept_id FROM kb.keyword_external_ids WHERE source = $1 AND external_id = $2 AND release = $3`
	promotionNegativeEvidenceSQL        = `SELECT subject_external_id, object_external_id FROM kb.keyword_catalog_negative_decisions WHERE source = $1 AND release = $2 AND relation = 'different_from' AND (subject_external_id = $3 OR object_external_id = $3)`
	promotionNegativeMappingSQL         = `SELECT EXISTS (SELECT 1 FROM kb.keyword_external_ids WHERE source = $1 AND external_id = $2 AND release = $3 AND concept_id = $4)`
	promotionSurfaceSQL                 = `SELECT concept_id FROM kb.keyword_surfaces WHERE surface_id = $1`
	promotionSurfaceEvidenceConceptsSQL = `SELECT DISTINCT x.concept_id
FROM kb.keyword_surface_evidence e
JOIN kb.keyword_external_ids x
  ON x.source = e.source AND x.external_id = e.external_id AND x.release = e.release
WHERE e.surface_id = ANY($1) AND x.concept_id <> $2
ORDER BY x.concept_id`
	promotionExternalInsertSQL   = `INSERT INTO kb.keyword_external_ids (source, external_id, release, concept_id) VALUES ($1,$2,$3,$4) ON CONFLICT (source, external_id, release) DO NOTHING`
	promotionEvidenceInsertSQL   = `INSERT INTO kb.keyword_surface_evidence (surface_id, source, external_id, release, evidence, confidence) VALUES ($1,$2,$3,$4,$5,1.0) ON CONFLICT (surface_id, source, external_id, release) DO NOTHING`
	promotionNegativeDecisionSQL = `SELECT EXISTS (SELECT 1 FROM kb.keyword_catalog_negative_decisions WHERE source = $1 AND release = $2 AND subject_external_id = $3 AND object_external_id = $4 AND relation = $5)`
)

// PromotionStore applies reviewed promotion files. Every write shares one
// transaction and the keyword family advisory lock; replay is idempotent and
// any mutation of an existing identity is rejected.
type PromotionStore struct {
	DB DBX
}

// ApplyReviewedPromotion validates and writes one reviewed promotion batch.
func (s PromotionStore) ApplyReviewedPromotion(ctx context.Context, file ReviewedPromotionFile) (PromotionResult, error) {
	if err := file.Validate(); err != nil {
		return PromotionResult{}, err
	}
	if s.DB == nil {
		return PromotionResult{}, errors.New("promotion store database is nil")
	}
	result := PromotionResult{Source: file.Source, Release: file.Release}
	err := withKeywordIdentityMutation(ctx, s.DB, func(db DBX) error {
		for _, promotion := range file.Promotions {
			if err := validatePositivePromotion(ctx, db, file, promotion); err != nil {
				return err
			}
			if err := writePositivePromotion(ctx, db, file, promotion, &result); err != nil {
				return err
			}
		}
		for _, negative := range file.NegativePromotions {
			if err := writeNegativePromotion(ctx, db, file, negative, &result); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return PromotionResult{}, err
	}
	return result, nil
}

func validatePositivePromotion(ctx context.Context, db DBX, file ReviewedPromotionFile, promotion PositivePromotion) error {
	var conceptScope, conceptStatus string
	err := db.QueryRowContext(ctx, promotionConceptSQL, promotion.ConceptID).Scan(&conceptScope, &conceptStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("unknown concept %q", promotion.ConceptID)
	}
	if err != nil {
		return fmt.Errorf("read concept %s: %w", promotion.ConceptID, err)
	}
	if conceptStatus != "active" {
		return fmt.Errorf("promotion target %q must be active, got %q", promotion.ConceptID, conceptStatus)
	}
	if conceptScope != file.Scope {
		return fmt.Errorf("promotion scope mismatch: concept %q scope %q, file scope %q", promotion.ConceptID, conceptScope, file.Scope)
	}

	role, licenseReview, relations, scopes, identityAuthority, err := readPromotionPolicy(ctx, db, file.Source, file.Release)
	if err != nil {
		return err
	}
	if role != string(ExactIdentityAuthority) || !identityAuthority {
		return fmt.Errorf("source policy %s/%s is not an exact identity authority", file.Source, file.Release)
	}
	if licenseReview != LicenseReviewApproved {
		return fmt.Errorf("source policy %s/%s license is not approved", file.Source, file.Release)
	}
	if !containsString(relations, string(IdentityRelationExactEquivalent)) {
		return fmt.Errorf("source policy %s/%s does not authorize exact_equivalent", file.Source, file.Release)
	}
	if !containsString(scopes, file.Scope) {
		return fmt.Errorf("source policy %s/%s does not allow scope %q", file.Source, file.Release, file.Scope)
	}

	var deploymentSource, deploymentRelease string
	var deploymentEnabled bool
	err = db.QueryRowContext(ctx, promotionDeploymentSQL, file.DeploymentKey).Scan(&deploymentSource, &deploymentRelease, &deploymentEnabled)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("deployment key %q is not registered", file.DeploymentKey)
	}
	if err != nil {
		return fmt.Errorf("read deployment %s: %w", file.DeploymentKey, err)
	}
	if !deploymentEnabled || deploymentSource != file.Source || deploymentRelease != file.Release {
		return fmt.Errorf("deployment %q does not enable %s/%s", file.DeploymentKey, file.Source, file.Release)
	}

	if promotion.Relation != string(IdentityRelationExactEquivalent) {
		return fmt.Errorf("promotion relation %q cannot be authoritative", promotion.Relation)
	}

	var payload []byte
	err = db.QueryRowContext(ctx, promotionStagingEntrySQL, file.Source, file.Release, promotion.ExternalID).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("staging entry %s/%s/%s is not imported", file.Source, file.Release, promotion.ExternalID)
	}
	if err != nil {
		return fmt.Errorf("read staging entry: %w", err)
	}
	if err := validatePromotionConstraints(payload, promotion); err != nil {
		return err
	}

	var existingConcept string
	err = db.QueryRowContext(ctx, promotionExistingMappingSQL, file.Source, promotion.ExternalID, file.Release).Scan(&existingConcept)
	if err == nil && existingConcept != promotion.ConceptID {
		return fmt.Errorf("%w: %s/%s/%s already maps to %s", ErrImmutableSourceRelease, file.Source, promotion.ExternalID, file.Release, existingConcept)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read existing external mapping: %w", err)
	}

	rows, err := db.QueryContext(ctx, promotionNegativeEvidenceSQL, file.Source, file.Release, promotion.ExternalID)
	if err != nil {
		return fmt.Errorf("read negative evidence: %w", err)
	}
	// Drain and close the evidence cursor before issuing the per-row mapping
	// probes: lib/pq does not tolerate a nested query on the same connection
	// while a result set is still open (protocol desync).
	type negativeEvidenceRow struct{ subject, object string }
	var negativeEvidence []negativeEvidenceRow
	for rows.Next() {
		var subject, object string
		if err := rows.Scan(&subject, &object); err != nil {
			rows.Close()
			return fmt.Errorf("scan negative evidence: %w", err)
		}
		negativeEvidence = append(negativeEvidence, negativeEvidenceRow{subject: subject, object: object})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate negative evidence: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close negative evidence: %w", err)
	}
	for _, evidence := range negativeEvidence {
		subject, object := evidence.subject, evidence.object
		counterpart := object
		if counterpart == promotion.ExternalID {
			counterpart = subject
		}
		var sameConcept bool
		if err := db.QueryRowContext(ctx, promotionNegativeMappingSQL, file.Source, counterpart, file.Release, promotion.ConceptID).Scan(&sameConcept); err != nil {
			return fmt.Errorf("read negative-evidence mapping: %w", err)
		}
		if sameConcept {
			return fmt.Errorf("authoritative non-equivalence: %s and %s are different_from but both map to %s",
				promotion.ExternalID, counterpart, promotion.ConceptID)
		}
	}

	for i, surface := range promotion.Surfaces {
		var surfaceConcept string
		err := db.QueryRowContext(ctx, promotionSurfaceSQL, surface.SurfaceID).Scan(&surfaceConcept)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("unknown surface %q", surface.SurfaceID)
		}
		if err != nil {
			return fmt.Errorf("read surface %s: %w", surface.SurfaceID, err)
		}
		if surfaceConcept != promotion.ConceptID {
			return fmt.Errorf("surface %q belongs to %s, not promoted concept %s (surface[%d])",
				surface.SurfaceID, surfaceConcept, promotion.ConceptID, i)
		}
	}

	// The promoted surfaces may already carry evidence for other governed
	// external identities. Those identities map to established concepts; if
	// any of them is vetoed never-mergeable with the promotion target, or is
	// aligned to a different governed term, the promotion would create a
	// merge the operators have explicitly refused, so it must fail closed.
	otherConcepts, err := loadPromotionEvidenceConcepts(ctx, db, promotion.Surfaces, promotion.ConceptID)
	if err != nil {
		return err
	}
	for _, other := range otherConcepts {
		blocked, err := isNeverMerge(ctx, db, promotion.ConceptID, other)
		if err != nil {
			return err
		}
		if blocked {
			return fmt.Errorf("promotion of %s to %s conflicts with never_merge veto involving %s", promotion.ExternalID, promotion.ConceptID, other)
		}
		conflict, err := (AlignmentsStore{}).MergeConflict(ctx, db, promotion.ConceptID, other)
		if err != nil {
			return err
		}
		if conflict {
			return fmt.Errorf("promotion of %s to %s conflicts with alignment of %s", promotion.ExternalID, promotion.ConceptID, other)
		}
	}
	return nil
}

func loadPromotionEvidenceConcepts(ctx context.Context, db DBX, surfaces []PromotedSurface, excludeConcept string) ([]string, error) {
	surfaceIDs := make([]string, 0, len(surfaces))
	for _, surface := range surfaces {
		surfaceIDs = append(surfaceIDs, surface.SurfaceID)
	}
	rows, err := db.QueryContext(ctx, promotionSurfaceEvidenceConceptsSQL, pq.Array(surfaceIDs), excludeConcept)
	if err != nil {
		return nil, fmt.Errorf("read surface-linked concepts: %w", err)
	}
	defer rows.Close()
	var concepts []string
	for rows.Next() {
		var conceptID string
		if err := rows.Scan(&conceptID); err != nil {
			return nil, fmt.Errorf("scan surface-linked concept: %w", err)
		}
		concepts = append(concepts, conceptID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate surface-linked concepts: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close surface-linked concepts: %w", err)
	}
	return concepts, nil
}

func readPromotionPolicy(ctx context.Context, db DBX, source, release string) (role, licenseReview string, relations, scopes []string, identityAuthority bool, err error) {
	var relationsArray, scopesArray pq.StringArray
	err = db.QueryRowContext(ctx, promotionPolicySQL, source, release).
		Scan(&role, &licenseReview, &relationsArray, &scopesArray, &identityAuthority)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil, nil, false, fmt.Errorf("source policy %s/%s is not registered", source, release)
	}
	if err != nil {
		return "", "", nil, nil, false, fmt.Errorf("read source policy: %w", err)
	}
	return role, licenseReview, []string(relationsArray), []string(scopesArray), identityAuthority, nil
}

func validatePromotionConstraints(payload []byte, promotion PositivePromotion) error {
	var stored map[string]any
	if err := json.Unmarshal(payload, &stored); err != nil {
		return fmt.Errorf("staging entry payload is not JSON: %w", err)
	}
	if err := compareStringList(stored, "unit_constraints", promotion.UnitConstraints); err != nil {
		return fmt.Errorf("unit constraint mismatch: %w", err)
	}
	if err := compareStringList(stored, "dimension_constraints", promotion.DimensionConstraints); err != nil {
		return fmt.Errorf("dimension constraint mismatch: %w", err)
	}
	return nil
}

func compareStringList(payload map[string]any, key string, want []string) error {
	raw, ok := payload[key]
	if !ok {
		if len(want) == 0 {
			return nil
		}
		return fmt.Errorf("staging entry has no %s", key)
	}
	values, ok := raw.([]any)
	if !ok {
		// A null staging value is the adapter's "no constraints" encoding.
		if len(want) == 0 && raw == nil {
			return nil
		}
		return fmt.Errorf("staging entry %s is not a list", key)
	}
	if len(want) == 0 {
		if len(values) > 0 {
			return fmt.Errorf("staging %s %v, promotion declares none", key, values)
		}
		return nil
	}
	got := make([]string, 0, len(values))
	for _, value := range values {
		got = append(got, fmt.Sprintf("%v", value))
	}
	if !reflect.DeepEqual(got, want) {
		return fmt.Errorf("staging %s %v, promotion declares %v", key, got, want)
	}
	return nil
}

func writePositivePromotion(ctx context.Context, db DBX, file ReviewedPromotionFile, promotion PositivePromotion, result *PromotionResult) error {
	if _, err := db.ExecContext(ctx, promotionExternalInsertSQL,
		file.Source, promotion.ExternalID, file.Release, promotion.ConceptID); err != nil {
		return fmt.Errorf("insert external identity %s: %w", promotion.ExternalID, err)
	}
	result.ExternalIDs++
	for _, surface := range promotion.Surfaces {
		if _, err := db.ExecContext(ctx, promotionEvidenceInsertSQL,
			surface.SurfaceID, file.Source, promotion.ExternalID, file.Release, surface.Evidence); err != nil {
			return fmt.Errorf("insert surface evidence for %s: %w", surface.SurfaceID, err)
		}
		result.SurfaceEvidence++
	}
	return nil
}

func writeNegativePromotion(ctx context.Context, db DBX, file ReviewedPromotionFile, negative NegativePromotion, result *PromotionResult) error {
	scopes, err := readPromotionPolicyScopes(ctx, db, file.Source, file.Release)
	if err != nil {
		return err
	}
	if !containsString(scopes, file.Scope) {
		return fmt.Errorf("source policy %s/%s does not allow negative scope %q", file.Source, file.Release, file.Scope)
	}
	for _, node := range []string{negative.NodeA, negative.NodeB} {
		var nodeScope, nodeStatus string
		err := db.QueryRowContext(ctx, promotionConceptSQL, node).Scan(&nodeScope, &nodeStatus)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("unknown concept %q in never_merge veto", node)
		}
		if err != nil {
			return fmt.Errorf("read never_merge node %s: %w", node, err)
		}
		if nodeStatus != "active" {
			return fmt.Errorf("never_merge node %q must be active, got %q", node, nodeStatus)
		}
		if nodeScope != file.Scope {
			return fmt.Errorf("never_merge scope mismatch: concept %q scope %q, file scope %q", node, nodeScope, file.Scope)
		}
	}
	for i, triple := range negative.Triples {
		role, licenseReview, _, _, identityAuthority, err := readPromotionPolicy(ctx, db, triple.Source, triple.Release)
		if err != nil {
			return err
		}
		if role != string(ExactIdentityAuthority) || !identityAuthority || licenseReview != LicenseReviewApproved {
			return fmt.Errorf("negative promotion[%d] triple source %s/%s is not authoritative (proposal-only distinctions cannot veto)", i, triple.Source, triple.Release)
		}
		if triple.Relation != "different_from" {
			return fmt.Errorf("negative promotion[%d] triple relation must be different_from, got %q", i, triple.Relation)
		}
		var exists bool
		if err := db.QueryRowContext(ctx, promotionNegativeDecisionSQL,
			triple.Source, triple.Release, triple.SubjectExternalID, triple.ObjectExternalID, triple.Relation).Scan(&exists); err != nil {
			return fmt.Errorf("read negative decision evidence: %w", err)
		}
		if !exists {
			return fmt.Errorf("negative promotion[%d] triple has no authoritative staging evidence", i)
		}
	}

	reason, err := json.Marshal(map[string]any{
		"scope": negative.Scope, "triples": negative.Triples,
		"reviewer": negative.Reviewer, "reviewed_at": negative.ReviewedAt,
		"provenance_locator": negative.ProvenanceLocator,
	})
	if err != nil {
		return fmt.Errorf("encode never_merge provenance: %w", err)
	}
	if err := (semid.NeverMergeStore{DB: db}).Add(ctx, "keyword", negative.NodeA, negative.NodeB, string(reason), negative.Reviewer); err != nil {
		return fmt.Errorf("write never_merge veto: %w", err)
	}
	result.NeverMerges++
	return nil
}

func readPromotionPolicyScopes(ctx context.Context, db DBX, source, release string) ([]string, error) {
	var scopesArray pq.StringArray
	err := db.QueryRowContext(ctx, `SELECT allowed_scopes FROM kb.keyword_sources WHERE source = $1 AND release = $2`, source, release).Scan(&scopesArray)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("source policy %s/%s is not registered", source, release)
	}
	if err != nil {
		return nil, fmt.Errorf("read source policy scopes: %w", err)
	}
	return []string(scopesArray), nil
}
