package keywords

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/lib/pq"
)

const tripleEvidenceRowsSQL = `SELECT e.id, e.source, e.external_id, e.release
FROM kb.keyword_surface_evidence e
WHERE e.surface_id = ANY($1)
ORDER BY e.id
FOR SHARE OF e`

const tripleSourcePoliciesSQL = `WITH requested AS (
	SELECT source, release FROM unnest($1::text[], $2::text[]) AS r(source, release)
)
SELECT s.source, s.release, s.identity_authority, s.allowed_scopes
FROM requested r
JOIN kb.keyword_sources s ON s.source = r.source AND s.release = r.release
ORDER BY s.source, s.release
FOR SHARE OF s`

const tripleConfiguredDeploymentsSQL = `SELECT d.deployment_key, d.source, d.release, d.enabled
FROM kb.keyword_identity_deployments d
WHERE d.deployment_key = ANY($1)
ORDER BY d.deployment_key
FOR SHARE OF d`

const tripleExternalMappingsSQL = `WITH requested AS (
	SELECT source, external_id, release
	FROM unnest($1::text[], $2::text[], $3::text[]) AS r(source, external_id, release)
)
SELECT x.source, x.external_id, x.release, x.concept_id, c.status, c.scope
FROM requested r
JOIN kb.keyword_external_ids x
	ON x.source = r.source AND x.external_id = r.external_id AND x.release = r.release
LEFT JOIN kb.keyword_concepts c ON c.concept_id = x.concept_id
ORDER BY x.source, x.external_id, x.release
FOR SHARE OF x`

// TripleEvidenceProvider adapts the governed source/evidence/external-ID
// tables into source-agnostic identity claims. DeploymentKeys is the explicit
// set of mutable deployment pointers trusted by this provider instance. The
// caller owns the candidate transaction and keyword-family advisory lock.
type TripleEvidenceProvider struct {
	DeploymentKeys []string
}

func (TripleEvidenceProvider) ProviderID() string { return TripleEvidenceProviderID }

type tripleEvidenceRowData struct {
	id         int64
	source     string
	externalID string
	release    string
}

type tripleSourceKey struct {
	source  string
	release string
}

type tripleExternalKey struct {
	source     string
	externalID string
	release    string
}

type tripleSourceState struct {
	identityAuthority bool
	allowedScopes     []string
}

type tripleMappingState struct {
	targetConceptID string
	status          string
	scope           string
}

func (provider TripleEvidenceProvider) LoadClaims(ctx context.Context, tx *sql.Tx, candidate CandidateIdentityContext) ([]IdentityClaim, error) {
	candidate, err := validateTripleCandidateContext(candidate)
	if err != nil {
		return nil, err
	}
	deploymentKeys, err := canonicalTripleDeploymentKeys(provider.DeploymentKeys)
	if err != nil {
		return nil, err
	}
	if tx == nil {
		return nil, errors.New("triple evidence provider requires a transaction")
	}
	if len(candidate.SurfaceIDs) == 0 {
		return []IdentityClaim{}, nil
	}

	evidenceRows, err := loadTripleEvidenceRows(ctx, tx, candidate.SurfaceIDs)
	if err != nil {
		return nil, err
	}
	if len(evidenceRows) == 0 {
		return []IdentityClaim{}, nil
	}

	sourceKeys := distinctTripleSourceKeys(evidenceRows)
	sources, err := loadTripleSourceStates(ctx, tx, sourceKeys)
	if err != nil {
		return nil, err
	}
	enabledSources, err := loadTripleConfiguredDeployments(ctx, tx, deploymentKeys)
	if err != nil {
		return nil, err
	}
	externalKeys := distinctTripleExternalKeys(evidenceRows)
	mappings, err := loadTripleMappingStates(ctx, tx, externalKeys)
	if err != nil {
		return nil, err
	}

	claims := make([]IdentityClaim, 0, len(evidenceRows))
	for _, evidence := range evidenceRows {
		sourceKey := tripleSourceKey{source: evidence.source, release: evidence.release}
		sourceState := sources[sourceKey]
		externalKey := tripleExternalKey{source: evidence.source, externalID: evidence.externalID, release: evidence.release}
		mapping, mapped := mappings[externalKey]
		if strings.TrimSpace(evidence.externalID) == "" {
			mapped = false
		}

		sourceRef := tripleSourceEvidenceRef(evidence.source, evidence.release)
		refs := []EvidenceRef{sourceRef, tripleSurfaceEvidenceRef(evidence.id)}
		if mapped {
			refs = append(refs, tripleMappingEvidenceRef(evidence.source, evidence.externalID, evidence.release))
		}
		claim := IdentityClaim{
			ProviderID:         TripleEvidenceProviderID,
			CandidateConceptID: candidate.CandidateConceptID,
			Relation:           IdentityRelationExactEquivalent,
			Authority:          IdentityAuthorityNonAuthoritative,
			EvidenceRefs:       refs,
		}
		if mapped {
			claim.TargetConceptID = mapping.targetConceptID
		}
		if mapped && sourceState.identityAuthority && enabledSources[sourceKey] &&
			containsString(sourceState.allowedScopes, candidate.Scope) &&
			mapping.status == "active" && mapping.scope == candidate.Scope {
			claim.Authority = IdentityAuthorityAuthoritative
			claim.AuthorityRef = sourceRef.Key
		}
		claims = append(claims, claim)
	}
	return claims, nil
}

func validateTripleCandidateContext(candidate CandidateIdentityContext) (CandidateIdentityContext, error) {
	if strings.TrimSpace(candidate.CandidateConceptID) == "" {
		return CandidateIdentityContext{}, errors.New("triple evidence candidate concept ID is blank")
	}
	if strings.TrimSpace(candidate.Scope) == "" {
		return CandidateIdentityContext{}, errors.New("triple evidence candidate scope is blank")
	}
	for _, surfaceID := range candidate.SurfaceIDs {
		if strings.TrimSpace(surfaceID) == "" {
			return CandidateIdentityContext{}, errors.New("triple evidence candidate surface ID is blank")
		}
	}
	return CanonicalCandidateIdentityContext(candidate), nil
}

func canonicalTripleDeploymentKeys(keys []string) ([]string, error) {
	canonical := append([]string(nil), keys...)
	sort.Strings(canonical)
	for index, key := range canonical {
		if strings.TrimSpace(key) == "" {
			return nil, errors.New("triple evidence deployment key is blank")
		}
		if index > 0 && key == canonical[index-1] {
			return nil, fmt.Errorf("duplicate triple evidence deployment key %q", key)
		}
	}
	return canonical, nil
}

func loadTripleEvidenceRows(ctx context.Context, tx *sql.Tx, surfaceIDs []string) ([]tripleEvidenceRowData, error) {
	rows, err := tx.QueryContext(ctx, tripleEvidenceRowsSQL, pq.Array(surfaceIDs))
	if err != nil {
		return nil, fmt.Errorf("load candidate surface identity evidence: %w", err)
	}
	defer rows.Close()
	var evidenceRows []tripleEvidenceRowData
	for rows.Next() {
		var row tripleEvidenceRowData
		if err := rows.Scan(&row.id, &row.source, &row.externalID, &row.release); err != nil {
			return nil, fmt.Errorf("scan candidate surface identity evidence: %w", err)
		}
		evidenceRows = append(evidenceRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate candidate surface identity evidence: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close candidate surface identity evidence: %w", err)
	}
	return evidenceRows, nil
}

func distinctTripleSourceKeys(evidenceRows []tripleEvidenceRowData) []tripleSourceKey {
	set := make(map[tripleSourceKey]struct{}, len(evidenceRows))
	for _, row := range evidenceRows {
		set[tripleSourceKey{source: row.source, release: row.release}] = struct{}{}
	}
	keys := make([]tripleSourceKey, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].source < keys[j].source || (keys[i].source == keys[j].source && keys[i].release < keys[j].release)
	})
	return keys
}

func loadTripleSourceStates(ctx context.Context, tx *sql.Tx, keys []tripleSourceKey) (map[tripleSourceKey]tripleSourceState, error) {
	sourceValues := make([]string, len(keys))
	releaseValues := make([]string, len(keys))
	for index, key := range keys {
		sourceValues[index], releaseValues[index] = key.source, key.release
	}
	requested := make(map[tripleSourceKey]struct{}, len(keys))
	for _, key := range keys {
		requested[key] = struct{}{}
	}
	rows, err := tx.QueryContext(ctx, tripleSourcePoliciesSQL, pq.Array(sourceValues), pq.Array(releaseValues))
	if err != nil {
		return nil, fmt.Errorf("load keyword identity sources: %w", err)
	}
	defer rows.Close()
	states := make(map[tripleSourceKey]tripleSourceState, len(keys))
	for rows.Next() {
		var source, release string
		var identityAuthority bool
		var allowedScopes pq.StringArray
		if err := rows.Scan(&source, &release, &identityAuthority, &allowedScopes); err != nil {
			return nil, fmt.Errorf("scan keyword identity source: %w", err)
		}
		key := tripleSourceKey{source: source, release: release}
		if _, ok := requested[key]; !ok {
			return nil, fmt.Errorf("keyword identity source query returned unexpected %q release %q", source, release)
		}
		if _, duplicate := states[key]; duplicate {
			return nil, fmt.Errorf("keyword identity source query returned duplicate %q release %q", source, release)
		}
		if allowedScopes == nil || (identityAuthority && len(allowedScopes) == 0) {
			return nil, fmt.Errorf("keyword identity source %q release %q has malformed allowed_scopes", source, release)
		}
		for _, scope := range allowedScopes {
			if strings.TrimSpace(scope) == "" {
				return nil, fmt.Errorf("keyword identity source %q release %q has malformed allowed_scopes", source, release)
			}
		}
		states[key] = tripleSourceState{identityAuthority: identityAuthority, allowedScopes: append([]string(nil), allowedScopes...)}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate keyword identity sources: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close keyword identity sources: %w", err)
	}
	for _, key := range keys {
		if _, ok := states[key]; !ok {
			return nil, fmt.Errorf("unregistered keyword identity source %q release %q", key.source, key.release)
		}
	}
	return states, nil
}

func loadTripleConfiguredDeployments(ctx context.Context, tx *sql.Tx, keys []string) (map[tripleSourceKey]bool, error) {
	enabledSources := make(map[tripleSourceKey]bool)
	if len(keys) == 0 {
		return enabledSources, nil
	}
	rows, err := tx.QueryContext(ctx, tripleConfiguredDeploymentsSQL, pq.Array(keys))
	if err != nil {
		return nil, fmt.Errorf("load configured keyword identity deployments: %w", err)
	}
	defer rows.Close()
	seenKeys := make(map[string]bool, len(keys))
	for rows.Next() {
		var key, source, release string
		var enabled bool
		if err := rows.Scan(&key, &source, &release, &enabled); err != nil {
			return nil, fmt.Errorf("scan configured keyword identity deployment: %w", err)
		}
		index := sort.SearchStrings(keys, key)
		if index == len(keys) || keys[index] != key {
			return nil, fmt.Errorf("deployment query returned unconfigured key %q", key)
		}
		if seenKeys[key] {
			return nil, fmt.Errorf("deployment query returned duplicate key %q", key)
		}
		seenKeys[key] = true
		if enabled {
			enabledSources[tripleSourceKey{source: source, release: release}] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate configured keyword identity deployments: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close configured keyword identity deployments: %w", err)
	}
	return enabledSources, nil
}

func distinctTripleExternalKeys(evidenceRows []tripleEvidenceRowData) []tripleExternalKey {
	set := make(map[tripleExternalKey]struct{}, len(evidenceRows))
	for _, row := range evidenceRows {
		if strings.TrimSpace(row.externalID) != "" {
			set[tripleExternalKey{source: row.source, externalID: row.externalID, release: row.release}] = struct{}{}
		}
	}
	keys := make([]tripleExternalKey, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].source != keys[j].source {
			return keys[i].source < keys[j].source
		}
		if keys[i].externalID != keys[j].externalID {
			return keys[i].externalID < keys[j].externalID
		}
		return keys[i].release < keys[j].release
	})
	return keys
}

func loadTripleMappingStates(ctx context.Context, tx *sql.Tx, keys []tripleExternalKey) (map[tripleExternalKey]tripleMappingState, error) {
	states := make(map[tripleExternalKey]tripleMappingState, len(keys))
	if len(keys) == 0 {
		return states, nil
	}
	sources := make([]string, len(keys))
	externalIDs := make([]string, len(keys))
	releases := make([]string, len(keys))
	for index, key := range keys {
		sources[index], externalIDs[index], releases[index] = key.source, key.externalID, key.release
	}
	requested := make(map[tripleExternalKey]struct{}, len(keys))
	for _, key := range keys {
		requested[key] = struct{}{}
	}
	rows, err := tx.QueryContext(ctx, tripleExternalMappingsSQL, pq.Array(sources), pq.Array(externalIDs), pq.Array(releases))
	if err != nil {
		return nil, fmt.Errorf("load keyword external mappings: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var source, externalID, release, targetConceptID string
		var status, scope sql.NullString
		if err := rows.Scan(&source, &externalID, &release, &targetConceptID, &status, &scope); err != nil {
			return nil, fmt.Errorf("scan keyword external mapping: %w", err)
		}
		key := tripleExternalKey{source: source, externalID: externalID, release: release}
		if _, ok := requested[key]; !ok {
			return nil, fmt.Errorf("keyword external mapping query returned unexpected %q/%q release %q", source, externalID, release)
		}
		if _, duplicate := states[key]; duplicate {
			return nil, fmt.Errorf("keyword external mapping query returned duplicate %q/%q release %q", source, externalID, release)
		}
		if strings.TrimSpace(targetConceptID) == "" || !status.Valid || !scope.Valid ||
			!AllowedConceptStatuses[status.String] || strings.TrimSpace(scope.String) == "" {
			return nil, fmt.Errorf("keyword external mapping %q/%q release %q references a missing or corrupt concept", source, externalID, release)
		}
		states[key] = tripleMappingState{targetConceptID: targetConceptID, status: status.String, scope: scope.String}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate keyword external mappings: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close keyword external mappings: %w", err)
	}
	return states, nil
}

func tripleB64(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func tripleSourceEvidenceRef(source, release string) EvidenceRef {
	encodedSource, encodedRelease := tripleB64(source), tripleB64(release)
	return EvidenceRef{
		Key:     "keyword_source:" + encodedSource + ":" + encodedRelease,
		Kind:    "authority_configuration",
		Locator: "postgres:kb.keyword_sources/" + encodedSource + "/" + encodedRelease,
	}
}

func tripleSurfaceEvidenceRef(id int64) EvidenceRef {
	decimalID := strconv.FormatInt(id, 10)
	return EvidenceRef{
		Key:     "keyword_surface_evidence:" + decimalID,
		Kind:    "candidate_surface_evidence",
		Locator: "postgres:kb.keyword_surface_evidence/" + decimalID,
	}
}

func tripleMappingEvidenceRef(source, externalID, release string) EvidenceRef {
	encodedSource, encodedExternalID, encodedRelease := tripleB64(source), tripleB64(externalID), tripleB64(release)
	return EvidenceRef{
		Key:     "keyword_external_id:" + encodedSource + ":" + encodedExternalID + ":" + encodedRelease,
		Kind:    "target_external_mapping",
		Locator: "postgres:kb.keyword_external_ids/" + encodedSource + "/" + encodedExternalID + "/" + encodedRelease,
	}
}
