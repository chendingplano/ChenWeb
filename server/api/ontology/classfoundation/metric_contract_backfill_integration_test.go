package classfoundation

import (
	"context"
	"database/sql"
	"testing"
)

// seedGovernedTermForBackfillTest mirrors assertions package's
// seedGovernedTerm helper (duplicated per-package in this codebase's existing
// test style, e.g. freshAssertionsTestDB / freshClassfoundationTestDB above).
func seedGovernedTermForBackfillTest(t *testing.T, db *sql.DB, termID, termKind, moduleID string) {
	t.Helper()
	if _, err := db.Exec(`
INSERT INTO kb.ontology_terms (term_id, version, term_kind, module_id, status)
VALUES ($1, 1, $2, $3, 'included_in_release')
ON CONFLICT DO NOTHING`, termID, termKind, moduleID); err != nil {
		t.Fatalf("seed governed term %s: %v", termID, err)
	}
}

// TestMetricContractBackfillReevaluatesStaleAssertion seeds an assertion
// written against an identity_only contract (recorded revision matches the
// original, honest not_evaluated state), promotes the class's contract out
// from under it the way a later write would, and checks that Reevaluate
// updates only the conformance fields -- not any identity, value, or
// evidence data -- exactly as the semantic-assertion-lifecycle spec delta
// requires.
func TestMetricContractBackfillReevaluatesStaleAssertion(t *testing.T) {
	db := freshClassfoundationTestDB(t)
	ctx := context.Background()
	const classTermID = "measurement:backfill-probe"

	seedGovernedTermForBackfillTest(t, db, "mea:measured_by", "property", "measurement")
	if _, err := db.Exec(`INSERT INTO kb.object_nodes (object_id, canonical_name, object_type) VALUES ('obj-backfill-1', 'obj-backfill-1', 'other')`); err != nil {
		t.Fatalf("seed object node: %v", err)
	}

	initial, err := (ContractStore{DB: db}).EnsureHeader(ctx, ClassIdentity{TermID: classTermID, ModuleID: "measurement", By: "test"})
	if err != nil {
		t.Fatalf("EnsureHeader: %v", err)
	}

	var assertionID int64
	if err := db.QueryRowContext(ctx, `
INSERT INTO kb.semantic_assertions (
    logical_identity_key, subject_ref_kind, subject_ref_id, subject_object_id,
    predicate_term_id, object_ref_kind, object_literal, assertion_kind_term_id,
    unit_term_id, value_form, numeric_value, status, instance_of_term_id,
    class_identity_state_term_id, mapping_resolution_state_term_id,
    value_state_term_id, conformance_state_term_id,
    normalized_against_contract_revision_id, create_by, modify_by
) VALUES (
    'claim:backfill-probe-1', 'object_node', 'obj-backfill-1', 'obj-backfill-1',
    'mea:measured_by', 'literal', '10'::jsonb, 'mea:observed_value',
    'quantity:unit_SEC', 'single', 10, 'represented', $1,
    'semantic:provisional_new', 'semantic:mapping_not_required',
    'semantic:value_present', 'semantic:not_evaluated',
    $2, 'test', 'test'
) RETURNING id`, classTermID, initial.ID).Scan(&assertionID); err != nil {
		t.Fatalf("seed stale assertion: %v", err)
	}

	backfill := MetricContractBackfill{DB: db}
	before, err := backfill.Report(ctx)
	if err != nil {
		t.Fatalf("Report before promotion: %v", err)
	}
	if len(before) != 0 {
		t.Fatalf("Report before promotion = %d rows, want 0 (contract still identity_only)", len(before))
	}

	seedPresentObservation(t, db, classTermID, "doc-1", "number", "quantity:unit_SEC")
	seedPresentObservation(t, db, classTermID, "doc-2", "number", "quantity:unit_SEC")
	promoted, wasPromoted, err := SynthesizeContractFromObservations(ctx, db, classTermID)
	if err != nil || !wasPromoted {
		t.Fatalf("promote contract: promoted=%v err=%v", wasPromoted, err)
	}

	after, err := backfill.Report(ctx)
	if err != nil {
		t.Fatalf("Report after promotion: %v", err)
	}
	if len(after) != 1 || after[0].AssertionID != assertionID {
		t.Fatalf("Report after promotion = %#v, want exactly the one stale assertion", after)
	}

	updated, err := backfill.Reevaluate(ctx)
	if err != nil {
		t.Fatalf("Reevaluate: %v", err)
	}
	if updated != 1 {
		t.Fatalf("Reevaluate updated = %d, want 1", updated)
	}

	var conformance string
	var normalizedAgainst int64
	var unitTermID string
	var status string
	if err := db.QueryRowContext(ctx, `
SELECT conformance_state_term_id, normalized_against_contract_revision_id, unit_term_id, status
FROM kb.semantic_assertions WHERE id = $1`, assertionID).Scan(&conformance, &normalizedAgainst, &unitTermID, &status); err != nil {
		t.Fatalf("reload assertion: %v", err)
	}
	if conformance != "semantic:conforms" {
		t.Fatalf("conformance_state_term_id = %q, want semantic:conforms", conformance)
	}
	if normalizedAgainst != promoted.ID {
		t.Fatalf("normalized_against_contract_revision_id = %d, want %d", normalizedAgainst, promoted.ID)
	}
	if unitTermID != "quantity:unit_SEC" {
		t.Fatalf("unit_term_id changed to %q, want unchanged quantity:unit_SEC", unitTermID)
	}
	if status != "represented" {
		t.Fatalf("status changed to %q, want unchanged represented", status)
	}

	rerun, err := backfill.Reevaluate(ctx)
	if err != nil {
		t.Fatalf("Reevaluate rerun: %v", err)
	}
	if rerun != 0 {
		t.Fatalf("Reevaluate rerun updated = %d, want 0 (already caught up)", rerun)
	}
}
