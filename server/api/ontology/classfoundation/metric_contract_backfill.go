package classfoundation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// StaleConformanceAssertion is one kb.semantic_assertions row this backfill
// can re-evaluate: its recorded normalized_against_contract_revision_id no
// longer matches its class's current contract revision.
type StaleConformanceAssertion struct {
	AssertionID    int64
	ClassTermID    string
	UnitTermID     string
	ValueIsPresent bool
}

// MetricContractBackfill re-evaluates conformance for metric assertions
// written against a contract revision that has since been superseded (most
// commonly: identity_only -> partially_defined) -- task 8's one-shot admin
// command, mirroring MetricSupportCleanup's report/apply shape.
//
// Simplification, stated plainly rather than hidden: the live write path
// (writeMetricLossless) compares an occurrence's resolved value *datatype*
// (from its own extracted metricCandidatePayload) against the contract, but
// kb.semantic_assertions does not persist that datatype directly. This
// backfill compares unit only -- the more reliably preserved signal
// (UnitTermID is a direct column) -- which is sufficient in practice because
// contract synthesis only ever promotes one unambiguous (datatype, unit)
// pair per class, so a matching unit and a present value are the same
// determination the live path would make for the overwhelming majority of
// real occurrences.
type MetricContractBackfill struct {
	DB DBX
}

// Report finds every assertion whose recorded contract revision no longer
// matches its class's current one. It naturally excludes assertions written
// against a still-identity_only contract that hasn't changed (recorded and
// current revision are the same row) and, once a class leaves identity_only,
// also naturally stops flagging assertions written after that point (which
// record the new revision at write time).
func (b MetricContractBackfill) Report(ctx context.Context) ([]StaleConformanceAssertion, error) {
	if b.DB == nil {
		return nil, errors.New("db is nil")
	}
	rows, err := b.DB.QueryContext(ctx, `
SELECT a.id, a.instance_of_term_id, a.unit_term_id, a.value_state_term_id
FROM kb.semantic_assertions a
JOIN kb.ontology_term_headers h ON h.term_id = a.instance_of_term_id
JOIN kb.ontology_class_contract_revisions r ON r.id = h.current_contract_revision_id
WHERE a.superseded_by IS NULL
  AND r.definition_state <> 'identity_only'
  AND a.normalized_against_contract_revision_id IS DISTINCT FROM h.current_contract_revision_id`)
	if err != nil {
		return nil, fmt.Errorf("query stale conformance assertions: %w", err)
	}
	defer rows.Close()
	var stale []StaleConformanceAssertion
	for rows.Next() {
		var (
			s          StaleConformanceAssertion
			unitTermID *string
			valueState string
		)
		if err := rows.Scan(&s.AssertionID, &s.ClassTermID, &unitTermID, &valueState); err != nil {
			return nil, fmt.Errorf("scan stale conformance assertion: %w", err)
		}
		if unitTermID != nil {
			s.UnitTermID = *unitTermID
		}
		s.ValueIsPresent = valueState == "semantic:value_present"
		stale = append(stale, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stale conformance assertions: %w", err)
	}
	return stale, nil
}

// Reevaluate re-runs the unit-based conformance comparison for every row
// Report finds, updates conformance_state_term_id and
// normalized_against_contract_revision_id in place, and returns how many
// rows changed. It never touches any other column -- conformance
// re-evaluation is not a new claim (semantic-assertion-lifecycle spec delta).
func (b MetricContractBackfill) Reevaluate(ctx context.Context) (updated int, err error) {
	stale, err := b.Report(ctx)
	if err != nil {
		return 0, err
	}
	store := ContractStore{DB: b.DB}
	for _, s := range stale {
		contract, ok, err := store.Current(ctx, s.ClassTermID)
		if err != nil {
			return updated, fmt.Errorf("load current contract for %s: %w", s.ClassTermID, err)
		}
		if !ok {
			continue // contract removed since Report(); leave it for the next run
		}
		state := "semantic:not_evaluated"
		if s.ValueIsPresent {
			var fields struct {
				PermittedUnitTermIDs []string `json:"permitted_unit_term_ids"`
			}
			if err := json.Unmarshal([]byte(contract.ContractPayload), &fields); err == nil {
				state = "semantic:conformance_contract_violation"
				for _, permitted := range fields.PermittedUnitTermIDs {
					if permitted == s.UnitTermID {
						state = "semantic:conforms"
						break
					}
				}
			}
		}
		if _, err := b.DB.ExecContext(ctx, `
UPDATE kb.semantic_assertions
SET conformance_state_term_id = $1, normalized_against_contract_revision_id = $2,
    modify_time = NOW(), modify_by = 'metric-contract-backfill'
WHERE id = $3`, state, contract.ID, s.AssertionID); err != nil {
			return updated, fmt.Errorf("update conformance state for assertion %d: %w", s.AssertionID, err)
		}
		updated++
	}
	return updated, nil
}
