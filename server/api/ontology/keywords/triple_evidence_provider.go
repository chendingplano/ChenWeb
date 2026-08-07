package keywords

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/lib/pq"
)

const tripleEvidenceRowsSQL = `SELECT e.id, e.source, e.external_id, e.release
FROM kb.keyword_surface_evidence e
WHERE e.surface_id = ANY($1)
ORDER BY e.id
FOR SHARE OF e`

const tripleSourcePolicySQL = `SELECT s.identity_authority
FROM kb.keyword_sources s
WHERE s.source = $1 AND s.release = $2
FOR SHARE OF s`

const tripleDeploymentsSQL = `SELECT d.enabled
FROM kb.keyword_identity_deployments d
WHERE d.source = $1 AND d.release = $2
ORDER BY d.deployment_key
FOR SHARE OF d`

const tripleExternalMappingSQL = `SELECT x.concept_id, c.status, c.scope
FROM kb.keyword_external_ids x
JOIN kb.keyword_concepts c ON c.concept_id = x.concept_id
WHERE x.source = $1 AND x.external_id = $2 AND x.release = $3
FOR SHARE OF x`

// TripleEvidenceProvider adapts the governed source/evidence/external-ID
// tables into source-agnostic identity claims. The caller owns the candidate
// transaction and the keyword-family advisory lock.
type TripleEvidenceProvider struct{}

func (TripleEvidenceProvider) ProviderID() string { return TripleEvidenceProviderID }

type tripleEvidenceRowData struct {
	id         int64
	source     string
	externalID string
	release    string
}

type tripleSourceState struct {
	identityAuthority bool
	enabled           bool
}

type tripleMappingState struct {
	targetConceptID string
	status          string
	scope           string
	found           bool
}

func (TripleEvidenceProvider) LoadClaims(ctx context.Context, tx *sql.Tx, candidate CandidateIdentityContext) ([]IdentityClaim, error) {
	if tx == nil {
		return nil, errors.New("triple evidence provider requires a transaction")
	}
	if len(candidate.SurfaceIDs) == 0 {
		return []IdentityClaim{}, nil
	}

	rows, err := tx.QueryContext(ctx, tripleEvidenceRowsSQL, pq.Array(candidate.SurfaceIDs))
	if err != nil {
		return nil, fmt.Errorf("load candidate surface identity evidence: %w", err)
	}
	evidenceRows := make([]tripleEvidenceRowData, 0)
	for rows.Next() {
		var row tripleEvidenceRowData
		if err := rows.Scan(&row.id, &row.source, &row.externalID, &row.release); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan candidate surface identity evidence: %w", err)
		}
		evidenceRows = append(evidenceRows, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate candidate surface identity evidence: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close candidate surface identity evidence: %w", err)
	}

	sources := make(map[string]tripleSourceState)
	for _, evidence := range evidenceRows {
		cacheKey := tripleTupleCacheKey(evidence.source, evidence.release)
		if _, ok := sources[cacheKey]; ok {
			continue
		}
		state, err := loadTripleSourceState(ctx, tx, evidence.source, evidence.release)
		if err != nil {
			return nil, err
		}
		sources[cacheKey] = state
	}

	mappings := make(map[string]tripleMappingState)
	claims := make([]IdentityClaim, 0, len(evidenceRows))
	for _, evidence := range evidenceRows {
		sourceState := sources[tripleTupleCacheKey(evidence.source, evidence.release)]
		mapping := tripleMappingState{}
		if strings.TrimSpace(evidence.externalID) != "" {
			cacheKey := tripleTupleCacheKey(evidence.source, evidence.externalID, evidence.release)
			var ok bool
			mapping, ok = mappings[cacheKey]
			if !ok {
				mapping, err = loadTripleMappingState(ctx, tx, evidence.source, evidence.externalID, evidence.release)
				if err != nil {
					return nil, err
				}
				mappings[cacheKey] = mapping
			}
		}

		sourceRef := tripleSourceEvidenceRef(evidence.source, evidence.release)
		refs := []EvidenceRef{sourceRef, tripleSurfaceEvidenceRef(evidence.id)}
		if mapping.found {
			refs = append(refs, tripleMappingEvidenceRef(evidence.source, evidence.externalID, evidence.release))
		}
		claim := IdentityClaim{
			ProviderID:         TripleEvidenceProviderID,
			CandidateConceptID: candidate.CandidateConceptID,
			TargetConceptID:    mapping.targetConceptID,
			Relation:           IdentityRelationExactEquivalent,
			Authority:          IdentityAuthorityNonAuthoritative,
			EvidenceRefs:       refs,
		}
		if mapping.found && sourceState.identityAuthority && sourceState.enabled &&
			mapping.status == "active" && mapping.scope == candidate.Scope {
			claim.Authority = IdentityAuthorityAuthoritative
			claim.AuthorityRef = sourceRef.Key
		}
		claims = append(claims, claim)
	}
	return claims, nil
}

func loadTripleSourceState(ctx context.Context, tx *sql.Tx, source, release string) (tripleSourceState, error) {
	var state tripleSourceState
	if err := tx.QueryRowContext(ctx, tripleSourcePolicySQL, source, release).Scan(&state.identityAuthority); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return tripleSourceState{}, fmt.Errorf("unregistered keyword identity source %q release %q", source, release)
		}
		return tripleSourceState{}, fmt.Errorf("load keyword identity source %q release %q: %w", source, release, err)
	}
	rows, err := tx.QueryContext(ctx, tripleDeploymentsSQL, source, release)
	if err != nil {
		return tripleSourceState{}, fmt.Errorf("load keyword identity deployments for %q release %q: %w", source, release, err)
	}
	for rows.Next() {
		var enabled bool
		if err := rows.Scan(&enabled); err != nil {
			rows.Close()
			return tripleSourceState{}, fmt.Errorf("scan keyword identity deployment for %q release %q: %w", source, release, err)
		}
		state.enabled = state.enabled || enabled
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return tripleSourceState{}, fmt.Errorf("iterate keyword identity deployments for %q release %q: %w", source, release, err)
	}
	if err := rows.Close(); err != nil {
		return tripleSourceState{}, fmt.Errorf("close keyword identity deployments for %q release %q: %w", source, release, err)
	}
	return state, nil
}

func loadTripleMappingState(ctx context.Context, tx *sql.Tx, source, externalID, release string) (tripleMappingState, error) {
	var state tripleMappingState
	err := tx.QueryRowContext(ctx, tripleExternalMappingSQL, source, externalID, release).Scan(
		&state.targetConceptID, &state.status, &state.scope,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return tripleMappingState{}, nil
	}
	if err != nil {
		return tripleMappingState{}, fmt.Errorf("load keyword external mapping %q/%q release %q: %w", source, externalID, release, err)
	}
	state.found = true
	return state, nil
}

func tripleTupleCacheKey(parts ...string) string {
	var builder strings.Builder
	for _, part := range parts {
		builder.WriteString(strconv.Itoa(len(part)))
		builder.WriteByte(':')
		builder.WriteString(part)
	}
	return builder.String()
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
