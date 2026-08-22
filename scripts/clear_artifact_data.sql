-- ============================================================
-- Section A: wholesale clear -- pure processor output.
-- Order matters: children before parents (see FK graph in the
-- session that authored this script; re-derive with \d if this
-- script is reused after the schema changes).
-- ============================================================

BEGIN;

DELETE FROM kb.semantic_retry_queue;

DELETE FROM kb.ontology_observed_class_attribute_distributions;
DELETE FROM kb.ontology_observed_class_profile_exceptions;
DELETE FROM kb.ontology_observed_class_profile_examples;
DELETE FROM kb.ontology_observed_class_attribute_observations;
DELETE FROM kb.ontology_observed_class_profiles;

DELETE FROM kb.semantic_processing_findings;
DELETE FROM kb.unresolved_semantic_occurrences;
DELETE FROM kb.semantic_processing_outcomes;

DELETE FROM kb.assertion_evidence;
DELETE FROM kb.assertion_relations;
DELETE FROM kb.semantic_decision_candidates;

DELETE FROM kb.semantic_assertions;

DELETE FROM kb.metrics;
DELETE FROM kb.provisions;

DELETE FROM kb.projection_state;
UPDATE kb.ontology_term_headers
   SET current_contract_revision_id = NULL
 WHERE create_by = 'metric_lossless_writer';

UPDATE kb.ontology_class_contract_revisions
   SET supersedes_revision_id = NULL
 WHERE create_by = 'metric_lossless_writer';

DELETE FROM kb.ontology_class_capability_validation_results
 WHERE contract_revision_id IN (
   SELECT id FROM kb.ontology_class_contract_revisions
   WHERE create_by = 'metric_lossless_writer'
 );

DELETE FROM kb.ontology_class_contract_capabilities
 WHERE contract_revision_id IN (
   SELECT id FROM kb.ontology_class_contract_revisions
   WHERE create_by = 'metric_lossless_writer'
 );

DELETE FROM kb.ontology_class_contract_revisions
 WHERE create_by = 'metric_lossless_writer';

DELETE FROM kb.ontology_term_headers
 WHERE create_by = 'metric_lossless_writer';

DELETE FROM kb.ontology_term_revisions
 WHERE term_id IN (SELECT term_id FROM kb.ontology_terms WHERE term_kind='metric_definition' AND status='auto-promoted');

DELETE FROM kb.ontology_term_labels
 WHERE term_id IN (SELECT term_id FROM kb.ontology_terms WHERE term_kind='metric_definition' AND status='auto-promoted');

DELETE FROM kb.ontology_terms
 WHERE term_kind='metric_definition' AND status='auto-promoted';

DELETE FROM kb.ontology_term_headers
 WHERE term_id IN (SELECT term_id FROM kb.ontology_terms WHERE term_kind='metric_definition' AND status='auto-promoted');

COMMIT;
