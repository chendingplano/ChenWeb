package assertions

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lib/pq"
)

// AssociateReport summarizes one associate_semantics run over one input
// record's pending decision candidates (spec §10.9 telemetry feeds from
// this).
type AssociateReport struct {
	Examined int
	Accepted int
	Deferred int
	Rejected int
}

// AssociateSemantics implements spec §10.3-§10.7 (generate candidates is
// normalize_assertions' job; this covers resolve targets, validate,
// adjudicate, persist) over kb.semantic_decision_candidates left in status
// 'candidate'. Adjudication here is deterministic-only (spec §16.3 item 4:
// an LLM-only recommendation cannot activate an assertion -- trivially true
// because no LLM path exists yet; a future LLM path plugs in as a candidate
// scorer ahead of this step, never as a direct writer). Which source
// artifact types Run examines, and how each is resolved, comes from the
// AssociationResolver registry (DR11 seam 5): adding a family's resolver
// never requires editing Run or processOne.
type AssociateSemantics struct {
	DB *sql.DB
}

// Run resolves, validates, adjudicates, and persists every 'candidate'
// decision candidate proposed for the given input record, for every
// source_artifact_type with a registered AssociationResolver.
func (a AssociateSemantics) Run(ctx context.Context, inputRecordID int64) (AssociateReport, error) {
	report := AssociateReport{}
	if a.DB == nil {
		return report, fmt.Errorf("db is nil")
	}
	sourceArtifactTypes := RegisteredAssociationResolverTypes()
	if len(sourceArtifactTypes) == 0 {
		return report, nil
	}
	if err := a.recoverDeferredCandidates(ctx, inputRecordID, sourceArtifactTypes); err != nil {
		return report, err
	}

	// Also picks up rows stuck at 'in_review': that status is this stage's own
	// momentary bookkeeping (transition-in immediately followed by
	// resolve/adjudicate), not a human-review pause, so a row still sitting
	// there was orphaned by a crash mid-candidate and must be resumable
	// (spec §16.3 item 13), not left stuck forever.
	const stmt = `
SELECT id FROM kb.semantic_decision_candidates
WHERE status IN ('candidate', 'in_review') AND source_artifact_type = ANY($1)
  AND input_record_id = $2`
	rows, err := a.DB.QueryContext(ctx, stmt, pq.Array(sourceArtifactTypes), inputRecordID)
	if err != nil {
		return report, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return report, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return report, err
	}

	dcStore := DecisionCandidateStore{DB: a.DB}
	asStore := AssertionStore{DB: a.DB}
	evStore := EvidenceStore{DB: a.DB, Assertions: asStore}

	for _, id := range ids {
		report.Examined++
		outcome, err := a.processOne(ctx, dcStore, asStore, evStore, id, inputRecordID)
		if err != nil {
			return report, fmt.Errorf("process candidate %d: %w", id, err)
		}
		switch outcome {
		case "accepted":
			report.Accepted++
		case "deferred":
			report.Deferred++
		case "rejected":
			report.Rejected++
		}
	}
	return report, nil
}

// recoverDeferredCandidates advances only candidates whose prerequisite has
// materially changed. A newly reconciled referent changes the normalized
// payload, so it is recovered through re-normalization; governed-term
// availability leaves the payload intact and uses RetryDeferred with a stable
// availability fingerprint instead.
func (a AssociateSemantics) recoverDeferredCandidates(ctx context.Context, inputRecordID int64, sourceArtifactTypes []string) error {
	const stmt = `
SELECT id, source_artifact_type, proposed_payload, dependency_fingerprint
FROM kb.semantic_decision_candidates
WHERE status = 'deferred' AND source_artifact_type = ANY($1)
  AND input_record_id = $2`
	rows, err := a.DB.QueryContext(ctx, stmt, pq.Array(sourceArtifactTypes), inputRecordID)
	if err != nil {
		return err
	}
	defer rows.Close()

	type deferredCandidate struct {
		id                                 int64
		artifactType, payload, fingerprint string
	}
	var deferred []deferredCandidate
	for rows.Next() {
		var d deferredCandidate
		if err := rows.Scan(&d.id, &d.artifactType, &d.payload, &d.fingerprint); err != nil {
			return err
		}
		deferred = append(deferred, d)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	needsRenormalization := false
	dcStore := DecisionCandidateStore{DB: a.DB}
	for _, d := range deferred {
		if d.fingerprint == "unresolved_referent" {
			needsRenormalization = true
			continue
		}
		if d.artifactType != "metric" {
			continue
		}
		var p metricCandidatePayload
		if err := json.Unmarshal([]byte(d.payload), &p); err != nil {
			return fmt.Errorf("decode deferred metric candidate %d: %w", d.id, err)
		}
		kindTermID, supported := metricAssertionKindTermID(p.AssertionKind)
		if !supported {
			continue
		}
		predicateTermID := "mea:measured_by"
		predicateOK, err := a.termExists(ctx, predicateTermID)
		if err != nil {
			return err
		}
		kindOK, err := a.termExists(ctx, kindTermID)
		if err != nil {
			return err
		}
		fingerprint := governedTermDependencyFingerprint(predicateTermID, predicateOK, kindTermID, kindOK)
		if fingerprint != d.fingerprint {
			if _, err := dcStore.RetryDeferred(ctx, d.id, fingerprint, "associate_semantics"); err != nil {
				return err
			}
		}
	}
	if needsRenormalization {
		if err := NormalizeAllFamilies(ctx, a.DB, inputRecordID); err != nil {
			return err
		}
	}
	return nil
}

// termExists reports whether a governed, released term exists (spec §10.5:
// validation must check ontology term kind/namespace/module compatibility;
// a candidate whose referenced term does not exist is never accepted).
func (a AssociateSemantics) termExists(ctx context.Context, termID string) (bool, error) {
	const stmt = `
SELECT EXISTS (
	SELECT 1 FROM kb.ontology_terms
	WHERE term_id = $1 AND status = 'included_in_release'
)`
	var exists bool
	err := a.DB.QueryRowContext(ctx, stmt, termID).Scan(&exists)
	return exists, err
}

func (a AssociateSemantics) processOne(ctx context.Context, dcStore DecisionCandidateStore, asStore AssertionStore, evStore EvidenceStore, id, inputRecordID int64) (string, error) {
	dc, err := dcStore.GetByID(ctx, id)
	if err != nil {
		return "", err
	}
	if dc.Status == StatusCandidate {
		if _, err := dcStore.TransitionStatus(ctx, dc.ID, StatusInReview, "", "associate_semantics"); err != nil {
			return "", err
		}
	}
	// dc.Status == StatusInReview here means a prior run was interrupted
	// after transitioning in but before resolving -- resume from where it
	// left off rather than re-transitioning (which would be refused).

	resolver, ok := LookupAssociationResolver(dc.SourceArtifactType)
	if !ok {
		return a.deferCandidate(ctx, dcStore, dc, "unrecognized_source_artifact_type")
	}
	return resolver(ctx, a, dcStore, asStore, evStore, dc, inputRecordID)
}

// init registers the metric and provision resolvers with the default
// AssociationResolver registry (DR11 seam 5): the same self-registration
// pattern MetricNormalizer/ProvisionNormalizer already use for
// normalize_assertions.
func init() {
	if err := RegisterAssociationResolver("metric", func(ctx context.Context, a AssociateSemantics, dcStore DecisionCandidateStore, asStore AssertionStore, evStore EvidenceStore, dc DecisionCandidate, inputRecordID int64) (string, error) {
		return a.processMetric(ctx, dcStore, asStore, evStore, dc, inputRecordID)
	}); err != nil {
		panic(err)
	}
	if err := RegisterAssociationResolver("provision", func(ctx context.Context, a AssociateSemantics, dcStore DecisionCandidateStore, _ AssertionStore, _ EvidenceStore, dc DecisionCandidate, _ int64) (string, error) {
		return a.processProvision(ctx, dcStore, dc)
	}); err != nil {
		panic(err)
	}
}

type metricCandidatePayload struct {
	MetricID               string   `json:"metric_id"`
	MetricName             string   `json:"metric_name"`
	MetricDefinitionTermID string   `json:"metric_definition_term_id"`
	Unit                   string   `json:"unit"`
	RawText                string   `json:"raw_text"`
	ValueForm              string   `json:"value_form"`
	Comparator             string   `json:"comparator"`
	AssertionKind          string   `json:"assertion_kind"`
	SubjectObjectID        string   `json:"subject_object_id"`
	Condition              string   `json:"condition"`
	NumericValue           *float64 `json:"numeric_value"`
	LowerValue             *float64 `json:"lower_value"`
	UpperValue             *float64 `json:"upper_value"`
}

// metricAssertionKindTermID rejects only missing and unparsed values. Every
// concrete kind is governed by the released ontology catalog through
// termExists below; keeping a second hardcoded allowlist here previously made
// a released mea:exact_value unusable.
func metricAssertionKindTermID(assertionKind string) (string, bool) {
	assertionKind = strings.TrimSpace(assertionKind)
	if assertionKind == "" || assertionKind == "unparsed" {
		return "", false
	}
	return "mea:" + assertionKind, true
}

func metricQualifiers(p metricCandidatePayload) map[string]any {
	qualifierMap := map[string]any{"metric_name": p.MetricName}
	if p.MetricDefinitionTermID != "" {
		qualifierMap["metric_definition_term_id"] = p.MetricDefinitionTermID
	}
	if p.Condition != "" {
		qualifierMap["condition"] = p.Condition
	}
	return qualifierMap
}

func governedTermDependencyFingerprint(predicateTermID string, predicateReleased bool, assertionKindTermID string, assertionKindReleased bool) string {
	return fmt.Sprintf("governed_terms:%s=%t,%s=%t", predicateTermID, predicateReleased, assertionKindTermID, assertionKindReleased)
}

func (a AssociateSemantics) processMetric(ctx context.Context, dcStore DecisionCandidateStore, asStore AssertionStore, evStore EvidenceStore, dc DecisionCandidate, inputRecordID int64) (string, error) {
	var p metricCandidatePayload
	if err := json.Unmarshal(dc.ProposedPayload, &p); err != nil {
		if _, err := dcStore.TransitionStatus(ctx, dc.ID, StatusRejected, "malformed proposed_payload", "associate_semantics"); err != nil {
			return "", err
		}
		return "rejected", nil
	}

	if p.SubjectObjectID == "" {
		if _, err := dcStore.SetResolution(ctx, dc.ID, "deferred", "ambiguous_targets"); err != nil {
			return "", err
		}
		return a.deferCandidate(ctx, dcStore, dc, "unresolved_referent")
	}
	assertionKindTermID, supported := metricAssertionKindTermID(p.AssertionKind)
	if !supported {
		return a.deferCandidate(ctx, dcStore, dc, "no_governed_assertion_kind_term:"+p.AssertionKind)
	}
	predicateTermID := "mea:measured_by"
	predicateOK, err := a.termExists(ctx, predicateTermID)
	if err != nil {
		return "", err
	}
	kindOK, err := a.termExists(ctx, assertionKindTermID)
	if err != nil {
		return "", err
	}
	if !predicateOK || !kindOK {
		return a.deferCandidateWithDependency(ctx, dcStore, dc,
			"governed_term_not_released:"+predicateTermID+","+assertionKindTermID,
			governedTermDependencyFingerprint(predicateTermID, predicateOK, assertionKindTermID, kindOK))
	}

	if _, err := dcStore.SetResolution(ctx, dc.ID, "matched", "subject reconciled; governed terms released"); err != nil {
		return "", err
	}

	objectLiteral := map[string]any{"unit": p.Unit}
	if p.NumericValue != nil {
		objectLiteral["value"] = *p.NumericValue
	}
	if p.LowerValue != nil {
		objectLiteral["lower"] = *p.LowerValue
	}
	if p.UpperValue != nil {
		objectLiteral["upper"] = *p.UpperValue
	}
	objectLiteralJSON, err := json.Marshal(objectLiteral)
	if err != nil {
		return "", err
	}
	qualifiers, err := json.Marshal(metricQualifiers(p))
	if err != nil {
		return "", err
	}
	unitTermID, quantityKindTermID := a.resolveUnitTerms(ctx, p.Unit)

	assertion := Assertion{
		LogicalIdentityKey:  dc.LogicalIdentityKey,
		SubjectRefKind:      "object_node",
		SubjectRefID:        p.SubjectObjectID,
		SubjectObjectID:     p.SubjectObjectID,
		PredicateTermID:     predicateTermID,
		ObjectRefKind:       "literal",
		ObjectLiteral:       objectLiteralJSON,
		AssertionKindTermID: assertionKindTermID,
		Qualifiers:          qualifiers,
		UnitTermID:          unitTermID,
		QuantityKindTermID:  quantityKindTermID,
		ValueForm:           p.ValueForm,
		NumericValue:        p.NumericValue,
		LowerValue:          p.LowerValue,
		UpperValue:          p.UpperValue,
		Comparator:          p.Comparator,
		RawText:             p.RawText,
		Confidence:          dc.Confidence,
		Status:              StatusCandidate,
		CreateBy:            "associate_semantics",
		ModifyBy:            "associate_semantics",
	}

	created, err := a.persistAssertion(ctx, asStore, assertion)
	if err != nil {
		return "", err
	}

	recordID := inputRecordID
	if _, err := evStore.AddEvidence(ctx, Evidence{
		AssertionID:   created.ID,
		InputRecordID: &recordID,
		ArtifactType:  dc.SourceArtifactType,
		ArtifactID:    dc.SourceArtifactID,
		EvidenceQuote: p.RawText,
		EvidenceRole:  "supports",
		ActorKind:     "processor",
		CreateBy:      "associate_semantics",
	}); err != nil {
		return "", err
	}
	if _, err := asStore.TransitionStatus(ctx, created.ID, StatusInReview, "", "associate_semantics"); err != nil {
		return "", err
	}
	if _, err := asStore.TransitionStatus(ctx, created.ID, StatusAccepted, "deterministic policy: released governed predicate and assertion-kind terms, resolved subject", "associate_semantics"); err != nil {
		return "", err
	}
	if _, err := dcStore.SetResultingAssertion(ctx, dc.ID, created.ID); err != nil {
		return "", err
	}
	if _, err := dcStore.TransitionStatus(ctx, dc.ID, StatusAccepted, "persisted as kb.semantic_assertions", "associate_semantics"); err != nil {
		return "", err
	}
	return "accepted", nil
}

// persistAssertion creates the assertion, or -- if a prior revision exists
// for this logical identity -- creates the next revision (spec §10.7:
// unchanged accepted decisions remain accepted; a changed payload creates a
// new revision rather than mutating the prior one).
func (a AssociateSemantics) persistAssertion(ctx context.Context, asStore AssertionStore, assertion Assertion) (Assertion, error) {
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

// resolveUnitTerms maps the raw unit string on an accepted metric assertion to
// a governed QUDT unit term and its quantity-kind term (P3 log §8 item 13),
// so DR21 verdicts can compare canonical units instead of raw strings. It is
// best-effort enrichment, never a gate: an unresolved unit leaves the term IDs
// empty and the assertion is still accepted (the comparison layer's unitDef
// registry remains the conversion authority).
//
// Lookup is by term_id against the QUDT quantity module. The import stores
// term rows (kb.ontology_terms, 4151 quantity terms) but did not populate
// kb.ontology_term_labels for them -- only core/mea/da have labels -- so a
// label join would resolve nothing. canonicalUnitForm maps the raw unit string
// to the catalog's local-name suffix; units absent from the catalog (e.g.
// cd/m² -- no candela unit was imported) resolve to empty term IDs, which the
// comparison layer's unitDef already handles for conversions.
func (a AssociateSemantics) resolveUnitTerms(ctx context.Context, rawUnit string) (unitTermID, quantityKindTermID string) {
	canonical := canonicalUnitForm(rawUnit)
	if canonical == "" || a.DB == nil {
		return "", ""
	}
	termID := "quantity:unit_" + canonical
	var exists bool
	err := a.DB.QueryRowContext(ctx, `
SELECT EXISTS (
	SELECT 1 FROM kb.ontology_terms
	WHERE term_id = $1 AND module_id = 'quantity'
	  AND status = 'included_in_release' AND term_kind = 'unit'
)`, termID).Scan(&exists)
	if err != nil || !exists {
		return "", ""
	}
	return termID, unitQuantityKindMap[canonical]
}

// unitQuantityKindMap maps QUDT unit local-names (the suffix after
// quantity:unit_) to their quantity-kind term IDs, covering the pilot corpus's
// units as actually imported into the catalog (local names verified against
// the dev DB: unit_SEC, unit_MilliSEC, unit_ONE, unit_DEG, unit_COUNT;
// quantity:qk_*). The QUDT import stores no unit→quantity-kind relation, so
// the mapping is a small hardcoded table over the pilot quantity kinds (DR12:
// metrics is the pilot family).
var unitQuantityKindMap = map[string]string{
	"MilliSEC": "quantity:qk_Time",
	"SEC":      "quantity:qk_Time",
	"ONE":      "quantity:qk_Dimensionless",
	"DEG":      "quantity:qk_Angle",
	"COUNT":    "quantity:qk_Count",
}

// canonicalUnitForm maps a raw unit string to the QUDT catalog's unit
// local-name (the suffix after quantity:unit_). Units absent from the imported
// catalog -- including cd/m², which has no candela unit -- return "" and
// resolve to empty term IDs.
func canonicalUnitForm(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "ms", "msec", "millisecond", "milliseconds", "毫秒":
		return "MilliSEC"
	case "s", "sec", "second", "seconds", "秒":
		return "SEC"
	case "ratio", "one", "1", "dimensionless", "无量纲":
		return "ONE"
	case "degree", "deg", "°", "度", "degrees":
		return "DEG"
	case "count", "counts", "个":
		return "COUNT"
	}
	return ""
}

type provisionCandidatePayload struct {
	ProvID          string `json:"prov_id"`
	ProvisionType   string `json:"provision_type"`
	Subject         string `json:"subject"`
	RawText         string `json:"raw_text"`
	Modality        string `json:"modality"`
	AssertionKind   string `json:"assertion_kind"`
	SubjectObjectID string `json:"subject_object_id"`
}

// processProvision resolves and validates a provision candidate. No core
// module term yet represents a deontic (required/prohibited/permitted)
// predicate (a real, documented content gap -- see the P3 implementation
// log), so every provision candidate defers here rather than inventing a
// term reference (spec §10.5: missing context produces deferred, not an
// invented target).
func (a AssociateSemantics) processProvision(ctx context.Context, dcStore DecisionCandidateStore, dc DecisionCandidate) (string, error) {
	var p provisionCandidatePayload
	if err := json.Unmarshal(dc.ProposedPayload, &p); err != nil {
		if _, err := dcStore.TransitionStatus(ctx, dc.ID, StatusRejected, "malformed proposed_payload", "associate_semantics"); err != nil {
			return "", err
		}
		return "rejected", nil
	}
	if _, err := dcStore.SetResolution(ctx, dc.ID, "deferred", "no governed deontic predicate term"); err != nil {
		return "", err
	}
	return a.deferCandidate(ctx, dcStore, dc, "no_governed_deontic_predicate")
}

func (a AssociateSemantics) deferCandidate(ctx context.Context, dcStore DecisionCandidateStore, dc DecisionCandidate, reason string) (string, error) {
	return a.deferCandidateWithDependency(ctx, dcStore, dc, reason, reason)
}

func (a AssociateSemantics) deferCandidateWithDependency(ctx context.Context, dcStore DecisionCandidateStore, dc DecisionCandidate, reason, dependencyFingerprint string) (string, error) {
	if _, err := dcStore.DeferCandidate(ctx, dc.ID, dependencyFingerprint, reason, "associate_semantics"); err != nil {
		return "", err
	}
	return "deferred", nil
}
