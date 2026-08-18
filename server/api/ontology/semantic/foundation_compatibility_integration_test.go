package semantic

import (
	"context"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/ontology/classfoundation"
)

func TestIntegrationFoundationCompatibilityForLegacyTermsAssertionsAndRedirects(t *testing.T) {
	db := freshSemanticTestDB(t)
	ctx := context.Background()

	for _, termID := range []string{"class:legacy", "class:current"} {
		if _, err := db.ExecContext(ctx, `
INSERT INTO kb.ontology_terms (term_id, version, term_kind, module_id, status)
VALUES ($1, 1, 'class', 'test', 'approved')`, termID); err != nil {
			t.Fatalf("insert legacy term %s: %v", termID, err)
		}
	}
	var currentTerm string
	if err := db.QueryRowContext(ctx, `SELECT term_id FROM kb.ontology_terms_current WHERE term_id = 'class:legacy'`).Scan(&currentTerm); err != nil || currentTerm != "class:legacy" {
		t.Fatalf("current-term compatibility read = %q, err = %v", currentTerm, err)
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO kb.semantic_assertions
  (logical_identity_key, subject_ref_kind, subject_ref_id, predicate_term_id,
   object_ref_kind, object_literal, status)
VALUES ('legacy:assertion','object_node','obj-1','mea:measured_by',
        'literal','{"value":1}'::jsonb,'candidate')`); err != nil {
		t.Fatalf("insert legacy assertion: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO kb.semantic_assertions
  (logical_identity_key, subject_ref_kind, subject_ref_id, predicate_term_id,
   object_ref_kind, object_literal, status, value_state_term_id)
VALUES ('represented:assertion','object_node','obj-2','mea:measured_by',
        'literal','{"value":2}'::jsonb,'represented',$1)`, ValuePresent); err != nil {
		t.Fatalf("insert represented assertion: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO kb.ontology_term_redirects (source_term_id, target_term_id, reason)
VALUES ('class:legacy', 'class:current', 'compatibility test')`); err != nil {
		t.Fatalf("insert term redirect: %v", err)
	}
	result := (classfoundation.RedirectResolver{Lookup: termRedirectLookup{db: db}}).Resolve(ctx, "class:legacy")
	if result.Status != classfoundation.RedirectResolved || result.TerminalTarget != "class:current" || len(result.Traversal) != 2 {
		t.Fatalf("redirect compatibility result = %#v", result)
	}
}
