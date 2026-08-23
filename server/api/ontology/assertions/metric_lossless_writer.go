package assertions

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/ontology/classfoundation"
	"github.com/chendingplano/deepdoc/server/api/ontology/semantic"
)

// MetricLosslessWriterVersion identifies this writer's dependency-fingerprint
// parser/writer axis (DR10). Bumping it forces every outcome to be treated as
// changed on the next run, which is the correct behavior for a logic change
// that is not reflected in any other tracked axis.
const MetricLosslessWriterVersion = "metric_lossless_writer/0.1.0"

// writeMetricLossless is ADR 2026081801 DR5's atomic metric semantic
// transaction, gated behind LOSSLESS_SEMANTIC_WRITES_METRIC. It replaces
// processMetric's tail (governed-kind gating, straight-to-accepted) for the
// value-range-type states DR12 lists: every one of them materializes exactly
// one represented assertion plus its stage outcome envelopes, never a
// processor error and never a deferral for a mapping/value reason alone.
//
// Referent resolution (SubjectObjectID) and the governed *predicate*'s
// release status are checked by the caller before this function runs -- both
// are prerequisites this ADR does not change, unlike the governed
// *assertion-kind* check, which DR12 supersedes (an unparsed/unsupported kind
// no longer defers; it becomes a raw-preserved, value-unparsed assertion).
func (a AssociateSemantics) writeMetricLossless(
	ctx context.Context,
	dc DecisionCandidate,
	p metricCandidatePayload,
	inputRecordID int64,
	predicateTermID string,
) (string, error) {
	assertionKindTermID, valueStateTermID := resolveMetricAssertionKindAndValueState(a, ctx, p)
	mappingStateTermID, mappingFindingTermID := resolveMetricMappingState(p.ValueRangeTypeLookup)
	disposition := semantic.DispositionRawPreserved
	if valueStateTermID == semantic.ValuePresent && mappingStateTermID == semantic.MappingResolved {
		disposition = semantic.DispositionNormalized
	}
	unitTermID, quantityKindTermID := a.resolveUnitTerms(ctx, p.Unit)

	classTermID := provisionalMetricClassTermID(p)

	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin metric semantic transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	synthUnitTermIDs := []string(nil)
	if unitTermID != "" {
		synthUnitTermIDs = []string{unitTermID}
	}
	selectedClassTermID, classIdentityState, signatureAlternatives, err := resolveOrCreateMetricClass(ctx, tx, classTermID, ClassSynthesisInput{
		ConceptID:            p.ConceptID,
		CanonicalName:        p.MetricName,
		Definition:           p.Definition,
		ValueType:            p.ValueDataType,
		RangeType:            p.ValueRangeTypeText,
		PermittedUnitTermIDs: synthUnitTermIDs,
		RawUnit:              p.Unit,
		ExtraProperties:      p.ExtraProperties,
	})
	if err != nil {
		return "", fmt.Errorf("resolve metric class: %w", err)
	}
	if _, _, err := (classfoundation.ClassResolutionDecisionStore{DB: tx}).RecordIfChanged(ctx, classfoundation.ClassResolutionDecision{
		SourceArtifactType:  dc.SourceArtifactType,
		SourceArtifactID:    dc.SourceArtifactID,
		SourceInputRecordID: &inputRecordID,
		SelectedClassTermID: selectedClassTermID,
		// kb.ontology_class_resolution_decisions.identity_state is CHECK-
		// constrained to classfoundation's bare Resolution* strings
		// ("resolved_existing", ...), distinct from the semantic package's
		// governed Class* term IDs ("semantic:resolved_existing", ...) stored
		// on the assertion itself -- the two vocabularies agree on every
		// value modulo the "semantic:" prefix.
		IdentityState: strings.TrimPrefix(classIdentityState, "semantic:"),
		Method:        "deterministic_definition_term",
		CreateBy:      "metric_lossless_writer",
	}, signatureAlternatives); err != nil {
		return "", fmt.Errorf("record class resolution decision: %w", err)
	}

	claimID, _, err := (classfoundation.ClaimIdentityStore{DB: tx}).FindOrCreateShadow(ctx, classfoundation.CanonicalClaimInput{
		KeyVersion:  "identity/v1",
		ClassTermID: selectedClassTermID,
		Fields:      canonicalClaimFields(valueStateTermID, assertionKindTermID, quantityKindTermID, unitTermID, p),
		By:          "metric_lossless_writer",
	})
	if err != nil {
		return "", fmt.Errorf("find or create canonical claim: %w", err)
	}

	asStore := AssertionStore{DB: tx}
	evStore := EvidenceStore{DB: tx, Assertions: asStore}
	dcStoreTx := DecisionCandidateStore{DB: tx}

	assertion := Assertion{
		LogicalIdentityKey:           "claim:" + claimID,
		SubjectRefKind:               "object_node",
		SubjectRefID:                 p.SubjectObjectID,
		SubjectObjectID:              p.SubjectObjectID,
		PredicateTermID:              predicateTermID,
		ObjectRefKind:                "literal",
		ObjectLiteral:                metricObjectLiteral(valueStateTermID, p),
		AssertionKindTermID:          assertionKindTermID,
		Qualifiers:                   mustMarshal(p.Qualifiers),
		UnitTermID:                   unitTermID,
		QuantityKindTermID:           quantityKindTermID,
		ValueForm:                    p.ValueForm,
		NumericValue:                 p.NumericValue,
		LowerValue:                   p.LowerValue,
		UpperValue:                   p.UpperValue,
		Comparator:                   p.Comparator,
		RawText:                      p.RawText,
		Confidence:                   dc.Confidence,
		Status:                       StatusRepresented,
		InstanceOfTermID:             selectedClassTermID,
		ClassIdentityStateTermID:     classIdentityState,
		MappingResolutionStateTermID: mappingStateTermID,
		ValueStateTermID:             valueStateTermID,
		ConformanceStateTermID:       semantic.ConformanceNotEvaluated,
		RawPayload:                   mustMarshal(map[string]string{"raw_text": p.RawText, "value_form": p.ValueForm}),
		RawSnapshotFingerprint:       dc.PayloadFingerprint,
		CreateBy:                     "metric_lossless_writer",
		ModifyBy:                     "metric_lossless_writer",
	}
	created, err := a.persistAssertionTx(ctx, asStore, assertion)
	if err != nil {
		return "", fmt.Errorf("persist represented assertion: %w", err)
	}

	if err := supersedeMetricSupportEvidence(ctx, evStore, dc, p, inputRecordID, created.ID); err != nil {
		return "", fmt.Errorf("supersede metric support evidence: %w", err)
	}

	if err := recordMetricStageOutcomes(ctx, tx, dc, p, inputRecordID, created.ID, mappingStateTermID, mappingFindingTermID, valueStateTermID, classIdentityState, disposition); err != nil {
		return "", err
	}

	if _, err := dcStoreTx.SetResultingAssertion(ctx, dc.ID, created.ID); err != nil {
		return "", fmt.Errorf("set resulting assertion: %w", err)
	}
	if _, err := dcStoreTx.TransitionStatus(ctx, dc.ID, StatusAccepted, "persisted as kb.semantic_assertions (represented)", "associate_semantics"); err != nil {
		return "", fmt.Errorf("transition decision candidate: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit metric semantic transaction: %w", err)
	}
	return "represented", nil
}

// resolveMetricAssertionKindAndValueState implements the assertion-kind half
// of DR12's disposition table. A governed, released kind keeps the existing
// behavior (value present). An empty or ungoverned kind -- what today defers
// the candidate -- no longer does: it falls back to the generic mea:
// observed_value kind (already governed and released) with an honest
// unparsed or missing value state, so the claim still materializes.
func resolveMetricAssertionKindAndValueState(a AssociateSemantics, ctx context.Context, p metricCandidatePayload) (assertionKindTermID, valueStateTermID string) {
	if kindTermID, supported := metricAssertionKindTermID(p.AssertionKind); supported {
		if ok, err := a.termExists(ctx, kindTermID); err == nil && ok {
			return kindTermID, semantic.ValuePresent
		}
	}
	if strings.TrimSpace(p.RawText) == "" && p.NumericValue == nil && p.LowerValue == nil && p.UpperValue == nil {
		return "mea:observed_value", semantic.ValueMissing
	}
	return "mea:observed_value", semantic.ValueUnparsed
}

// MetricMappingFindingFingerprint is the dependency fingerprint a mapping
// finding (mapping_unresolved or mapping_ambiguous) carries for one raw
// range-type value at one governed mapping state. It is exported so the
// admin mapping-decision handler (task 6.8) can compute the exact "current"
// fingerprint of the findings a mapping approval affects, and schedule
// retry scoped to that one raw value via
// semantic.RetryQueue.ScheduleForKeyedDependencyChange -- matching on
// ScheduleForDependencyChange's plain "not yet at target" predicate would
// instead sweep every other raw value's still-unresolved findings too.
func MetricMappingFindingFingerprint(rawValueRangeType, mappingStateTermID string) string {
	return semantic.Dependencies{
		MappingRevision: mappingStateTermID,
		Extra:           map[string]string{"value_range_type_raw": rawValueRangeType},
	}.Fingerprint()
}

// resolveMetricMappingState implements DR12's mapping half of the disposition
// table. ValueRangeTypeLookup is the status normalize_assertions already
// recorded via ValueRangeTypeMapper.Lookup; reading it here (rather than
// calling Lookup again) is what keeps the mapping observation idempotent --
// retrying this candidate never increments kb.metric_value_range_type_map's
// occurrence_count a second time.
func resolveMetricMappingState(lookupStatus string) (stateTermID, findingTermID string) {
	switch lookupStatus {
	case "approved":
		return semantic.MappingResolved, ""
	case "proposed":
		return semantic.MappingUnresolved, semantic.FindingMappingUnresolved
	case "ambiguous":
		return semantic.MappingAmbiguous, semantic.FindingMappingAmbiguous
	default: // "absent" or unset: no range-type field to map.
		return semantic.MappingNotRequired, ""
	}
}

// provisionalMetricClassTermID is the deterministic identity-cluster key
// approved for this writer: a metric's own metric_definition_term_id when
// present (already a stable, namespaced identifier), otherwise a stable hash
// of its normalized name. Retrying with the same input always proposes the
// same term ID, which is what makes resolveOrCreateMetricClass idempotent.
func provisionalMetricClassTermID(p metricCandidatePayload) string {
	if key := strings.TrimSpace(p.MetricDefinitionTermID); key != "" {
		return key
	}
	name := strings.ToLower(strings.TrimSpace(p.MetricName))
	sum := sha256.Sum256([]byte(name))
	return "measurement:auto:defname_" + hex.EncodeToString(sum[:])[:12]
}

// metricDefinitionTermKind is the only term_kind this pass wires
// signature-based resolution for (ADR 2026082203 design.md non-goals: a
// future term_kind gets its own signature composition via DR4's config,
// not a code change to matchClassBySignature).
const metricDefinitionTermKind = "metric_definition"

// resolveOrCreateMetricClass implements ADR 2026082203 DR5: it first
// attempts signature-based resolution (matchClassBySignature) before
// falling back to today's unchanged order -- delegating to the registered
// ClassSynthesizer (metric-class-synthesis-seam, bug 2026082101 finding 5)
// to create or select the metric's class, running inside the caller's
// transaction so the class write commits or rolls back with the rest of the
// metric semantic transaction. When input.ConceptID is set, the
// synthesizer's own concept-based existence check makes it idempotent on
// retry, and the resulting term_id may differ from candidateTermID (an
// existing concept-linked term wins over the hash-derived candidate). When
// no concept is available, the synthesizer's create path is not itself
// idempotent (mirroring the prior CreateIdentityOnlyClass's doc comment), so
// a header-existence pre-check on candidateTermID keeps a retried attempt
// from appending a redundant term.
func resolveOrCreateMetricClass(ctx context.Context, tx *sql.Tx, candidateTermID string, input ClassSynthesisInput) (termID, identityState string, alternatives []classfoundation.ClassResolutionAlternative, err error) {
	signatureTermID, tiedAlternatives, err := matchClassBySignature(ctx, tx, input)
	if err != nil {
		return "", "", nil, fmt.Errorf("match class by signature: %w", err)
	}
	if signatureTermID != "" {
		return signatureTermID, semantic.ClassResolvedExisting, nil, nil
	}

	if input.ConceptID == "" {
		var exists bool
		if err := tx.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM kb.ontology_term_headers WHERE term_id = $1)`, candidateTermID).Scan(&exists); err != nil {
			return "", "", nil, err
		}
		if exists {
			return candidateTermID, semantic.ClassResolvedExisting, nil, nil
		}
	}
	synthTermID, created, err := SynthesizeClass(ctx, tx, candidateTermID, input)
	if err != nil {
		return "", "", nil, err
	}
	if !created {
		return synthTermID, semantic.ClassResolvedExisting, nil, nil
	}
	// A signature tie (design.md D3 gap-fix 2) never guesses between the
	// tied existing candidates: it still provisions a new class through the
	// unchanged create path above, but flags the decision ambiguous and
	// carries the tied candidates as alternatives for
	// ClassResolutionDecisionStore.RecordIfChanged to persist.
	if len(tiedAlternatives) > 0 {
		return synthTermID, semantic.ClassAmbiguousCandidates, tiedAlternatives, nil
	}
	return synthTermID, semantic.ClassProvisionalNew, nil, nil
}

// resolvedSignatureDimensions extracts the occurrence's resolved
// identity-bearing signature dimensions from ExtraProperties -- only
// entries in BuildConfiguredProperties' {"raw":..., "resolved":...} shape
// (openspec change governed-property-normalization) with a non-empty
// resolved value count as "resolved" for signature matching. A plain
// (non-normalized) property value is silently skipped, not an error -- it
// simply carries no identity signal. This is mechanism-agnostic: a
// "resolved" value may have come from keyword-concept identity resolution
// (metric_name/subject), a curated bucket map (value_class/value_type/
// range_type), or any future method -- the shape is uniform regardless.
func resolvedSignatureDimensions(extraProperties map[string]any) map[string]string {
	resolved := map[string]string{}
	for property, v := range extraProperties {
		entry, ok := v.(map[string]any)
		if !ok {
			continue
		}
		value, _ := entry["resolved"].(string)
		if value == "" {
			continue
		}
		resolved[property] = value
	}
	return resolved
}

// matchClassBySignature implements ADR 2026082203 DR5 / design.md D3: find
// an existing class whose stored properties signature agrees with the
// occurrence's resolved identity-bearing dimensions.
//
// A candidate class must share at least one dimension resolved to the same
// non-empty value on both sides to count as a match at all -- "matches on
// every dimension resolved on both sides" is otherwise vacuously true for
// zero shared dimensions, which would match every class in the catalog to a
// wholly-unresolved occurrence (design.md D3 gap-fix 1). Among qualifying
// candidates, the one(s) agreeing on the most shared dimensions win; a tie
// is never guessed between (gap-fix 2) -- termID is empty and alternatives
// lists every tied candidate for the caller to record and resolve by
// provisioning a new class instead.
func matchClassBySignature(ctx context.Context, tx *sql.Tx, input ClassSynthesisInput) (termID string, alternatives []classfoundation.ClassResolutionAlternative, err error) {
	resolved := resolvedSignatureDimensions(input.ExtraProperties)
	if len(resolved) == 0 {
		return "", nil, nil
	}

	properties := make([]string, 0, len(resolved))
	for property := range resolved {
		properties = append(properties, property)
	}
	sort.Strings(properties) // deterministic query arg order

	query := `SELECT term_id, properties FROM kb.ontology_terms_current
WHERE term_kind = $1 AND status IN ('included_in_release', 'auto-promoted') AND properties IS NOT NULL`
	args := []any{metricDefinitionTermKind}
	for _, property := range properties {
		args = append(args, property, resolved[property])
		dimArg, valueArg := len(args)-1, len(args)
		query += fmt.Sprintf(" AND (properties->$%d->>'resolved' IS NULL OR properties->$%d->>'resolved' = $%d)", dimArg, dimArg, valueArg)
	}

	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return "", nil, fmt.Errorf("query signature candidates: %w", err)
	}
	defer rows.Close()

	type candidate struct {
		termID    string
		agreement int
	}
	var best []candidate
	bestCount := 0
	for rows.Next() {
		var candTermID string
		var propsRaw json.RawMessage
		if err := rows.Scan(&candTermID, &propsRaw); err != nil {
			return "", nil, err
		}
		var candProps map[string]any
		if err := json.Unmarshal(propsRaw, &candProps); err != nil {
			return "", nil, fmt.Errorf("unmarshal candidate properties for %s: %w", candTermID, err)
		}
		agreement := 0
		for _, property := range properties {
			entry, ok := candProps[property].(map[string]any)
			if !ok {
				continue
			}
			if v, _ := entry["resolved"].(string); v != "" && v == resolved[property] {
				agreement++
			}
		}
		if agreement == 0 {
			continue
		}
		switch {
		case agreement > bestCount:
			bestCount = agreement
			best = []candidate{{termID: candTermID, agreement: agreement}}
		case agreement == bestCount:
			best = append(best, candidate{termID: candTermID, agreement: agreement})
		}
	}
	if err := rows.Err(); err != nil {
		return "", nil, err
	}

	switch len(best) {
	case 0:
		return "", nil, nil
	case 1:
		return best[0].termID, nil, nil
	default:
		alts := make([]classfoundation.ClassResolutionAlternative, len(best))
		for i, c := range best {
			// Evidence is inserted as ::jsonb (class_resolution_decisions_store.go's
			// normalizedJSON only defaults an empty string to "{}"; it does not
			// escape arbitrary text into JSON), so this must already be valid JSON.
			evidence, err := json.Marshal(map[string]any{"reason": "signature_tie", "agreement_count": c.agreement})
			if err != nil {
				return "", nil, err
			}
			alts[i] = classfoundation.ClassResolutionAlternative{
				CandidateClassTermID: c.termID,
				// Rank is left zero: kb.ontology_class_resolution_alternatives
				// enforces uniqueness on (decision_id, rank), and
				// ClassResolutionDecisionStore.record auto-numbers an unset
				// Rank by slice position -- every tied candidate is equally
				// ranked in truth, but the table still needs a distinct
				// ordinal per row.
				Evidence: string(evidence),
			}
		}
		return "", alts, nil
	}
}

// canonicalClaimFields implements DR2's identity-value branch selection.
// Provenance (source record, artifact ID, wording beyond what identifies the
// value, model, prompt) is deliberately excluded so semantically equal
// occurrences converge on the same claim.
func canonicalClaimFields(valueStateTermID, assertionKindTermID, quantityKindTermID, unitTermID string, p metricCandidatePayload) map[string]string {
	switch valueStateTermID {
	case semantic.ValuePresent:
		return map[string]string{
			"branch":         "normalized_value",
			"assertion_kind": assertionKindTermID,
			"quantity_kind":  quantityKindTermID,
			"unit":           unitTermID,
			"comparator":     p.Comparator,
			"value":          formatMetricValue(p),
		}
	case semantic.ValueMissing:
		return map[string]string{"branch": "missing"}
	default: // unparsed, datatype_mismatch, unknown: raw_fingerprint branch.
		sum := sha256.Sum256([]byte(strings.TrimSpace(p.RawText)))
		return map[string]string{
			"branch":          "raw_fingerprint",
			"raw_fingerprint": hex.EncodeToString(sum[:]),
			"assertion_kind":  assertionKindTermID,
		}
	}
}

func formatMetricValue(p metricCandidatePayload) string {
	switch {
	case p.NumericValue != nil:
		return strconv.FormatFloat(*p.NumericValue, 'g', -1, 64)
	case p.LowerValue != nil && p.UpperValue != nil:
		return strconv.FormatFloat(*p.LowerValue, 'g', -1, 64) + ".." + strconv.FormatFloat(*p.UpperValue, 'g', -1, 64)
	default:
		return ""
	}
}

// metricObjectLiteral builds the assertion's object payload. DR6 forbids a
// fabricated object for a genuinely missing value: subject, class, and
// evidence still identify the claim, but no literal is invented.
func metricObjectLiteral(valueStateTermID string, p metricCandidatePayload) json.RawMessage {
	if valueStateTermID == semantic.ValueMissing {
		return nil
	}
	literal := map[string]any{"unit": p.Unit}
	if p.NumericValue != nil {
		literal["value"] = *p.NumericValue
	}
	if p.LowerValue != nil {
		literal["lower"] = *p.LowerValue
	}
	if p.UpperValue != nil {
		literal["upper"] = *p.UpperValue
	}
	return mustMarshal(literal)
}

func mustMarshal(v any) json.RawMessage {
	buf, err := json.Marshal(v)
	if err != nil {
		// Every caller passes only maps of strings/numbers/bools, which cannot
		// fail to marshal.
		return json.RawMessage("{}")
	}
	return buf
}

// persistAssertionTx mirrors persistAssertion but is explicit about running
// inside the caller's transaction, since asStore is already tx-scoped.
func (a AssociateSemantics) persistAssertionTx(ctx context.Context, asStore AssertionStore, assertion Assertion) (Assertion, error) {
	_, err := asStore.GetLatest(ctx, assertion.LogicalIdentityKey)
	switch err {
	case nil:
		return asStore.CreateRevision(ctx, assertion)
	case sql.ErrNoRows:
		return asStore.CreateAssertion(ctx, assertion)
	default:
		return Assertion{}, err
	}
}

// supersedeMetricSupportEvidence enforces DR5's metric support-link
// cardinality: at most one active 'supports' link per (artifact_type,
// artifact_id, input_record_id). An existing link to a different assertion is
// soft-deleted (which correctly demotes that other assertion to unsupported
// if it was its last support); an existing link to the SAME assertion is left
// alone so a retry does not churn evidence rows.
func supersedeMetricSupportEvidence(ctx context.Context, evStore EvidenceStore, dc DecisionCandidate, p metricCandidatePayload, inputRecordID, newAssertionID int64) error {
	existing, err := activeMetricSupportEvidence(ctx, evStore, dc.SourceArtifactType, dc.SourceArtifactID, inputRecordID)
	if err != nil {
		return err
	}
	if existing != nil {
		if existing.AssertionID == newAssertionID {
			return nil
		}
		if err := evStore.DeleteEvidence(ctx, existing.ID, "metric_lossless_writer", "superseded by new metric semantic transaction"); err != nil {
			return err
		}
	}
	recordID := inputRecordID
	_, err = evStore.AddEvidence(ctx, Evidence{
		AssertionID:      newAssertionID,
		InputRecordID:    &recordID,
		ArtifactType:     dc.SourceArtifactType,
		ArtifactID:       dc.SourceArtifactID,
		ArtifactObjectID: p.SubjectObjectID,
		EvidenceQuote:    p.RawText,
		SourceLineSpans:  dc.SourceLineSpans,
		ExtractionRun:    p.ExtractionRun,
		Model:            p.Model,
		PromptVersion:    p.PromptVersion,
		Confidence:       dc.Confidence,
		EvidenceRole:     "supports",
		ActorKind:        "processor",
		CreateBy:         "metric_lossless_writer",
	})
	return err
}

// activeMetricSupportEvidence queries by artifact rather than by assertion
// (EvidenceStore.ListForAssertion is keyed the other way), since DR5's
// cardinality invariant is scoped to the metric occurrence, not to any one
// assertion.
func activeMetricSupportEvidence(ctx context.Context, evStore EvidenceStore, artifactType, artifactID string, inputRecordID int64) (*Evidence, error) {
	row := evStore.DB.QueryRowContext(ctx, `
SELECT `+evidenceColumns+`
FROM kb.assertion_evidence
WHERE artifact_type = $1 AND artifact_id = $2 AND input_record_id = $3
  AND evidence_role = 'supports' AND NOT deleted
LIMIT 1`, artifactType, artifactID, inputRecordID)
	e, err := scanEvidence(row.Scan)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// recordMetricStageOutcomes writes the mandatory outcome envelope for each of
// the metric adapter's three required stages (DR5): one active envelope per
// stage, each with its own decision-scoped findings.
func recordMetricStageOutcomes(ctx context.Context, tx *sql.Tx, dc DecisionCandidate, p metricCandidatePayload, inputRecordID, assertionID int64, mappingStateTermID, mappingFindingTermID, valueStateTermID, classIdentityState, disposition string) error {
	artifactType, artifactID := dc.SourceArtifactType, dc.SourceArtifactID
	inputFingerprint := dc.PayloadFingerprint
	if inputFingerprint == "" {
		inputFingerprint = "unknown"
	}

	// Stage 1: normalize -- mapping and value findings, each keyed within its
	// own declared decision scope so they never collide (DR4).
	var normalizeFindings []semantic.Finding
	if mappingFindingTermID != "" {
		normalizeFindings = append(normalizeFindings, semantic.Finding{
			FindingKey:            semantic.FindingKey("range_type_mapping", semantic.DimensionMapping),
			DimensionTermID:       semantic.DimensionMapping,
			FindingTermID:         mappingFindingTermID,
			SeverityTermID:        semantic.SeverityWarning,
			DependencyFingerprint: MetricMappingFindingFingerprint(p.ValueRangeTypeRaw, mappingStateTermID),
			CreateBy:              "metric_lossless_writer",
		})
	}
	if valueStateTermID == semantic.ValueUnparsed || valueStateTermID == semantic.ValueMissing {
		findingTerm := semantic.FindingUnparsed
		if valueStateTermID == semantic.ValueMissing {
			findingTerm = semantic.FindingValueMissing
		}
		normalizeFindings = append(normalizeFindings, semantic.Finding{
			FindingKey:            semantic.FindingKey("value_literal", semantic.DimensionValue),
			DimensionTermID:       semantic.DimensionValue,
			FindingTermID:         findingTerm,
			SeverityTermID:        semantic.SeverityWarning,
			DependencyFingerprint: semantic.Dependencies{ParserVersion: MetricLosslessWriterVersion}.Fingerprint(),
			CreateBy:              "metric_lossless_writer",
		})
	}
	normalizeStageDeps := semantic.Dependencies{MappingRevision: mappingStateTermID, ParserVersion: MetricLosslessWriterVersion}.Fingerprint()
	if err := recordStageOutcome(ctx, tx, artifactType, artifactID, inputRecordID, assertionID,
		semantic.StageNormalize, disposition, inputFingerprint, normalizeStageDeps, normalizeFindings); err != nil {
		return fmt.Errorf("record normalize stage outcome: %w", err)
	}

	// Stage 2: class resolution.
	var classFindings []semantic.Finding
	switch classIdentityState {
	case semantic.ClassProvisionalNew:
		classFindings = append(classFindings, semantic.Finding{
			FindingKey:            semantic.FindingKey("class_identity", semantic.DimensionClass),
			DimensionTermID:       semantic.DimensionClass,
			FindingTermID:         semantic.FindingClassProvisional,
			SeverityTermID:        semantic.SeverityInfo,
			DependencyFingerprint: semantic.Dependencies{ClassIdentityDecision: classIdentityState}.Fingerprint(),
			CreateBy:              "metric_lossless_writer",
		})
	case semantic.ClassAmbiguousCandidates:
		classFindings = append(classFindings, semantic.Finding{
			FindingKey:            semantic.FindingKey("class_identity", semantic.DimensionClass),
			DimensionTermID:       semantic.DimensionClass,
			FindingTermID:         semantic.FindingClassAmbiguous,
			SeverityTermID:        semantic.SeverityWarning,
			DependencyFingerprint: semantic.Dependencies{ClassIdentityDecision: classIdentityState}.Fingerprint(),
			CreateBy:              "metric_lossless_writer",
		})
	}
	classDisposition := semantic.DispositionNormalized
	if classIdentityState != semantic.ClassResolvedExisting {
		classDisposition = semantic.DispositionRawPreserved
	}
	classStageDeps := semantic.Dependencies{ClassIdentityDecision: classIdentityState}.Fingerprint()
	if err := recordStageOutcome(ctx, tx, artifactType, artifactID, inputRecordID, assertionID,
		semantic.StageClassResolution, classDisposition, inputFingerprint, classStageDeps, classFindings); err != nil {
		return fmt.Errorf("record class resolution stage outcome: %w", err)
	}

	// Stage 3: associate -- conformance/conflict checking is not implemented
	// in this vertical slice (provisional classes are identity_only, with no
	// validation capability to check against yet), so this stage honestly
	// reports not_applicable with no findings rather than a fabricated pass.
	associateStageDeps := semantic.Dependencies{ParserVersion: MetricLosslessWriterVersion}.Fingerprint()
	if err := recordStageOutcome(ctx, tx, artifactType, artifactID, inputRecordID, assertionID,
		semantic.StageAssociate, semantic.DispositionNotApplicable, inputFingerprint, associateStageDeps, nil); err != nil {
		return fmt.Errorf("record associate stage outcome: %w", err)
	}
	return nil
}

// recordStageOutcome writes one stage's outcome envelope and findings inside
// the caller's transaction, deriving execution category from whether any
// finding was produced (DR3: a finding never fails execution by itself).
func recordStageOutcome(ctx context.Context, tx *sql.Tx, artifactType, artifactID string, inputRecordID, assertionID int64,
	stageTermID, dispositionTermID, inputFingerprint, stageDependencyFingerprint string, findings []semantic.Finding) error {
	category := semantic.CategorySemanticSuccess
	if len(findings) > 0 {
		category = semantic.CategorySemanticFinding
	}
	out := semantic.Outcome{
		OutcomeKey:            semantic.OutcomeKey(inputRecordID, artifactType, artifactID, stageTermID),
		InputRecordID:         &inputRecordID,
		ArtifactType:          artifactType,
		ArtifactID:            artifactID,
		AssertionID:           &assertionID,
		StageTermID:           stageTermID,
		DispositionTermID:     dispositionTermID,
		ExecutionStatus:       semantic.ExecutionStatusFor(category),
		OutcomeCategory:       category,
		DependencyFingerprint: stageDependencyFingerprint,
		InputFingerprint:      inputFingerprint,
		ProcessorName:         "metric_lossless_writer",
		ProcessorVersion:      MetricLosslessWriterVersion,
		CreateBy:              "metric_lossless_writer",
	}
	_, err := semantic.RecordTx(ctx, tx, out, findings)
	return err
}
