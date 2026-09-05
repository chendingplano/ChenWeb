package classfoundation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// SynthesisMethodUnambiguousAgreement names the one deterministic synthesis
// rule this change implements (metric-class-contract-synthesis spec: promote
// only on unambiguous, multi-document agreement -- never from a raw
// observation union).
const SynthesisMethodUnambiguousAgreement = "deterministic_unambiguous_observation_agreement"

// minSynthesisDocuments is the smallest number of distinct documents whose
// evidence must agree before a contract is promoted. One document's unit
// choice is not evidence of cross-document comparability, which is the
// entire point of a contract; this is deliberately the only invented
// threshold in this mechanism (design.md records why 2, not a higher
// number: no threshold study exists yet, and a low, recorded bar is easier
// to correct later than a hidden heuristic).
const minSynthesisDocuments = 2

// observedValueGroup is one distinct (logical_datatype, unit_term_id) pair
// seen for a class's "value" attribute, aggregated across
// kb.ontology_observed_class_attribute_observations rows in the 'present'
// observation state only -- unparsed/missing/malformed evidence never
// contributes to the agreement check, though it remains visible as evidence.
type observedValueGroup struct {
	LogicalDatatype string
	UnitTermID      string
	DocumentCount   int
}

// SynthesizeContractFromObservations promotes a metric class's contract from
// identity_only to partially_defined when, and only when, its recorded
// present-state observed-profile evidence agrees on exactly one
// (logical_datatype, unit_term_id) pair across at least minSynthesisDocuments
// distinct documents. It is a no-op -- not an error -- whenever the class's
// contract has already left identity_only (promotion happens at most once;
// a later contradiction is recorded as an observed-profile exception by the
// caller, not reverted here) or the evidence doesn't yet meet that bar.
func SynthesizeContractFromObservations(ctx context.Context, db DBX, classTermID string) (revision ContractRevision, promoted bool, err error) {
	if db == nil {
		return ContractRevision{}, false, errors.New("db is nil")
	}
	store := ContractStore{DB: db}
	current, ok, err := store.Current(ctx, classTermID)
	if err != nil {
		return ContractRevision{}, false, fmt.Errorf("load current contract: %w", err)
	}
	if !ok || current.DefinitionState != DefinitionIdentityOnly {
		return current, false, nil
	}

	groups, err := observedValueGroups(ctx, db, classTermID)
	if err != nil {
		return ContractRevision{}, false, err
	}
	if len(groups) != 1 || groups[0].DocumentCount < minSynthesisDocuments {
		return current, false, nil
	}
	group := groups[0]

	payload, err := json.Marshal(map[string]any{
		"value_type":              group.LogicalDatatype,
		"permitted_unit_term_ids": []string{group.UnitTermID},
	})
	if err != nil {
		return ContractRevision{}, false, fmt.Errorf("marshal synthesized contract payload: %w", err)
	}
	provenance, err := json.Marshal(map[string]any{
		"logical_datatype": group.LogicalDatatype,
		"unit_term_id":     group.UnitTermID,
		"document_count":   group.DocumentCount,
	})
	if err != nil {
		return ContractRevision{}, false, fmt.Errorf("marshal synthesis provenance: %w", err)
	}

	appended, err := store.AppendContractRevision(ctx, ContractRevision{
		TermID:                classTermID,
		ContractSchemaVersion: "contract/v1",
		IdentitySchemaVersion: "identity/v1",
		DefinitionState:       DefinitionPartiallyDefined,
		ContractPayload:       string(payload),
		SynthesisMethod:       SynthesisMethodUnambiguousAgreement,
		Provenance:            string(provenance),
		CreateBy:              "metric_lossless_writer",
	})
	if err != nil {
		return ContractRevision{}, false, fmt.Errorf("append synthesized contract revision: %w", err)
	}
	return appended, true, nil
}

func observedValueGroups(ctx context.Context, db DBX, classTermID string) ([]observedValueGroup, error) {
	rows, err := db.QueryContext(ctx, `
SELECT a.logical_datatype, a.unit_term_id, SUM(a.document_count) AS document_count
FROM kb.ontology_observed_class_attribute_observations a
JOIN kb.ontology_observed_class_profiles p ON p.id = a.profile_id
WHERE p.class_term_id = $1 AND a.attribute_key = 'value' AND a.observation_state = 'present'
GROUP BY a.logical_datatype, a.unit_term_id`, classTermID)
	if err != nil {
		return nil, fmt.Errorf("query observed value groups: %w", err)
	}
	defer rows.Close()
	var groups []observedValueGroup
	for rows.Next() {
		var g observedValueGroup
		if err := rows.Scan(&g.LogicalDatatype, &g.UnitTermID, &g.DocumentCount); err != nil {
			return nil, fmt.Errorf("scan observed value group: %w", err)
		}
		groups = append(groups, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate observed value groups: %w", err)
	}
	return groups, nil
}
