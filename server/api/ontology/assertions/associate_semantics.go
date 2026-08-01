package assertions

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

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
	MetricID        string   `json:"metric_id"`
	MetricName      string   `json:"metric_name"`
	Unit            string   `json:"unit"`
	RawText         string   `json:"raw_text"`
	ValueForm       string   `json:"value_form"`
	Comparator      string   `json:"comparator"`
	AssertionKind   string   `json:"assertion_kind"`
	SubjectObjectID string   `json:"subject_object_id"`
	NumericValue    *float64 `json:"numeric_value"`
	LowerValue      *float64 `json:"lower_value"`
	UpperValue      *float64 `json:"upper_value"`
}

// governedMetricAssertionKinds are the measurement module's assertion-kind
// properties installed by P2 (server/cmd/ontology-seed). An assertion_kind
// outside this set -- including the normalizer's 'unparsed' -- has no
// governed term to reference yet, so it defers rather than inventing one.
var governedMetricAssertionKinds = map[string]bool{
	"lower_bound_requirement": true, "upper_bound_requirement": true,
	"interval_requirement": true, "observed_value": true,
	"target": true, "reference": true, "capability": true,
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
	if !governedMetricAssertionKinds[p.AssertionKind] {
		return a.deferCandidate(ctx, dcStore, dc, "no_governed_assertion_kind_term:"+p.AssertionKind)
	}
	predicateTermID := "mea:measured_by"
	assertionKindTermID := "mea:" + p.AssertionKind
	predicateOK, err := a.termExists(ctx, predicateTermID)
	if err != nil {
		return "", err
	}
	kindOK, err := a.termExists(ctx, assertionKindTermID)
	if err != nil {
		return "", err
	}
	if !predicateOK || !kindOK {
		return a.deferCandidate(ctx, dcStore, dc, "governed_term_not_released:"+predicateTermID+","+assertionKindTermID)
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
	qualifiers, err := json.Marshal(map[string]any{"metric_name": p.MetricName})
	if err != nil {
		return "", err
	}

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
	if _, err := dcStore.DeferCandidate(ctx, dc.ID, reason, reason, "associate_semantics"); err != nil {
		return "", err
	}
	return "deferred", nil
}
