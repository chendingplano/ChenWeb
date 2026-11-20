package docsync

// Each table enumerates its portable columns and identity fields.
var portableTables = map[string]tableContract{
	"inputs": {name: "inputs", id: "id", scope: "id",
		fields:     "title doc_no publish_date authors doc_metadata",
		jsonFields: "doc_metadata",
	},
	"chunks": {name: "chunks", id: "id", scope: "source_record_id", key: "chunking_method chunking_size overlap_percent normal_lines",
		fields: "chunking_method chunking_size overlap_percent notes overlap_lines normal_lines chunk_lines",
	},
	"ex_summaries": {name: "ex_summaries", id: "id", scope: "input_record_id", key: "summary_id",
		fields: "summary_id algorithm summary_level summary_seq_no source_line_spans source_sentence_spans " +
			"child_summary_ids keywords keywords_en summary_text summary_text_en language extraction_config",
		jsonFields: "source_line_spans source_sentence_spans child_summary_ids keywords keywords_en extraction_config",
	},
	"summaries": {name: "summaries", id: "id", scope: "input_record_id", key: "summary_id",
		fields: "summary_id summary_level summary_seq_no source_line_spans child_summary_ids keywords keywords_en " +
			"summary_text summary_text_en language",
		jsonFields: "source_line_spans child_summary_ids keywords keywords_en",
	},
	"topics": {name: "topics", id: "id", scope: "input_record_id", key: "topic_id",
		fields: "topic_id topic_seq_no topic_type topic_type_en source_line_spans keywords keywords_en category_paths " +
			"category_paths_en category_path_items category_path_items_en topic_desc topic_desc_en",
		jsonFields: "source_line_spans keywords keywords_en category_paths category_paths_en category_path_items category_path_items_en",
	},
	"doc_facets": {name: "doc_facets", scope: "record_id",
		fields: "knowledge_store_binding input_doc_type source_language has_document_number ks_store_id",
	},
	"doc_facet_values": {name: "doc_facet_values", id: "id", scope: "record_id", key: "path method decision_attempt_id invocation_id",
		fields: "path value state method confidence evidence source_fingerprint decision_attempt_id invocation_id " +
			"vocabulary_release_id",
		jsonFields: "value evidence",
		refs:       []rowReference{{"vocabulary_release_id", "ontology_module_releases", false}},
	},
	"metrics": {name: "metrics", id: "id", scope: "input_record_id", key: "metric_id",
		fields: "metric_name source_line_spans metric_subject metric_desc metric_context metric_keywords location_type " +
			"metric_unit formula_or_definition threshold_or_target measurement_frequency confidence is_explicit_metric " +
			"reasoning_tags ext_info metric_name_en metric_subject_en metric_desc_en metric_context_en " +
			"metric_keywords_en model_name prompt_name metric_unit_en table_name_or_section metric_value " +
			"value_data_type value_range_type value_class value_class_en metric_id category_paths category_paths_en " +
			"metric_categories metric_categories_en value_min value_max condition metric_definition_term_id " +
			"value_range_type_error source_table_rows provision_id ref_mention_id ref_input_record_id ref_metric_id " +
			"ref_metric_key condition_en frame value_structure",
		jsonFields: "source_line_spans metric_keywords reasoning_tags ext_info metric_keywords_en category_paths category_paths_en source_table_rows frame value_structure",
		refs:       []rowReference{{"provision_id", "provisions", false}, {"ref_mention_id", "ref_mentions", false}, {"ref_input_record_id", "inputs", false}},
	},
	"metrics_dropped": {name: "metrics_dropped", id: "id", scope: "input_record_id", key: "drop_id",
		fields:     "drop_id candidate_id drop_stage drop_reason decision row_data",
		jsonFields: "decision row_data",
	},
	"provisions": {name: "provisions", id: "id", scope: "input_record_id", key: "prov_id",
		fields: "input_filename provision_type source_text source_line_spans provision provision_en provision_subject " +
			"location_type provision_keywords confidence is_explicit need_verify ext_info prov_id prov_name prov_desc " +
			"prov_context category_paths status public_info notes error_msg model_name prompt_name prov_name_en " +
			"provision_subject_en prov_desc_en prov_context_en provision_keywords_en category_paths_en extract_id",
		jsonFields: "source_line_spans provision_keywords ext_info category_paths public_info provision_keywords_en category_paths_en",
	},
	"products": {name: "products", id: "id", scope: "input_record_id", key: "product_rel_id",
		fields: "product_rel_id product_name product_name_en canonical_name canonical_name_en product_type relation_type " +
			"relation_summary relation_summary_en evidence_quote evidence_lines obligation_level requirement_text " +
			"requirement_text_en conditions exceptions parameters related_products responsible_actor confidence " +
			"confidence_reason category_paths category_paths_en status model_name prompt_name public_info notes " +
			"error_msg confidence_reason_en product_name_id",
		jsonFields: "evidence_lines conditions exceptions parameters related_products category_paths category_paths_en public_info",
		refs:       []rowReference{{"product_name_id", "product_names", false}},
	},
	"product_names": {name: "product_names", id: "id", key: "source seq_no product_name", shared: true,
		fields: "seq_no sub_catalog category_l1 category_l2 description intended_use product_name product_name_en " +
			"regulatory_class aliases keywords source notes status code_group child_code_group industry_code " +
			"cpc entry_no entry_no_new",
		jsonFields: "aliases keywords extra_info",
	},
	"ref_mentions": {name: "ref_mentions", id: "id", scope: "input_record_id", key: "ref_id",
		fields: "ref_id section source_line_spans evidence ref_form list_kind list_entry_id normativity in_requirement " +
			"list_mismatch mechanism cited_raw identifiers target_type target_subtype target_subtype_label target_title " +
			"target_version relationships target_scope target_locator target_content metric_name metric_name_en " +
			"metric_subject metric_subject_en condition internal confidence resource_id version_id r1_state " +
			"target_input_id r2_state target_section r3_state metric_state mention_key ext_info source_table_rows",
		jsonFields: "source_line_spans identifiers ext_info source_table_rows",
		refs:       []rowReference{{"list_entry_id", "ref_mentions", true}, {"resource_id", "ref_resources", false}, {"version_id", "ref_resource_versions", false}, {"target_input_id", "inputs", false}},
	},
	"ref_resources": {name: "ref_resources", id: "id", key: "identity_key visibility", shared: true,
		fields: "identity_key visibility target_type target_subtype title title_en issuer",
	},
	"ref_resource_versions": {name: "ref_resource_versions", id: "id", key: "resource_id version_label", shared: true,
		fields: "resource_id version_label status supersedes_version_id adoption_degree",
		refs:   []rowReference{{"resource_id", "ref_resources", false}, {"supersedes_version_id", "ref_resource_versions", true}},
	},
	"ref_resource_identifiers": {name: "ref_resource_identifiers", id: "id", key: "kind value visibility", shared: true,
		fields: "resource_id version_id kind value visibility",
		refs:   []rowReference{{"resource_id", "ref_resources", false}, {"version_id", "ref_resource_versions", false}},
	},
	"reference_artifact_links": {name: "reference_artifact_links", id: "id", scope: "citing_input_id", key: "ref_mention_id artifact_family target_artifact_key",
		fields: "ref_mention_id artifact_family target_input_id target_artifact_table target_artifact_id " +
			"target_artifact_key rank score match_method status policy_version resolved_artifact_id",
		refs: []rowReference{{"ref_mention_id", "ref_mentions", false}, {"target_input_id", "inputs", false}},
	},
	"entities": {name: "entities", id: "id", scope: "input_record_id", key: "entity_id",
		fields: "entity_id language entity entity_en entity_type entity_type_en aliases aliases_en desc_text desc_text_en " +
			"keywords keywords_en confidence model_name prompt_name ext_info line_spans entity_context doc_name " +
			"categories entity_status canonical_entity_id reconcile_status content_fingerprint object_link_status",
		jsonFields: "aliases aliases_en keywords keywords_en ext_info line_spans categories",
	},
	"entity_names": {name: "entity_names", id: "name_id", key: "name_key", shared: true,
		fields:     "name_key name_raw display_names aliases name_desc status",
		jsonFields: "display_names aliases",
	},
	"relations": {name: "relations", id: "id", scope: "input_record_id", key: "relation_id",
		fields: "relation_id language subject subject_en predicate predicate_en object object_en desc_text desc_text_en " +
			"keywords keywords_en confidence model_name prompt_name ext_info line_spans subject_entity_id " +
			"object_entity_id categories subject_lines predicate_lines object_lines subject_desc object_desc",
		jsonFields: "keywords keywords_en ext_info line_spans categories subject_lines predicate_lines object_lines",
	},
	"relation_predicates": {name: "relation_predicates", id: "predicate_id", key: "predicate_key", shared: true,
		fields:     "predicate_key predicate_raw display_names aliases predicate_desc status",
		jsonFields: "display_names aliases",
	},
	"inventory_items": {name: "inventory_items", id: "id", scope: "input_record_id", key: "inventory_item_id",
		fields: "inventory_item_id language item_name canonical_name manufacturer brand model_number part_number " +
			"normalized_specs raw_specs standards aliases evidence_quote source_line_spans validation_flags " +
			"missing_required_attrs dedupe_key schema_version dictionary_version confidence confidence_reason " +
			"model_name prompt_name ext_info item_categories canonical_item_id reconcile_status",
		jsonFields: "normalized_specs raw_specs standards aliases source_line_spans validation_flags missing_required_attrs ext_info item_categories",
	},
	"inventory_item_duplicates": {name: "inventory_item_duplicates", id: "id", scope: "input_record_id", key: "inventory_item_id",
		fields: "inventory_item_id duplicate_of language item_name canonical_name manufacturer brand model_number " +
			"part_number normalized_specs raw_specs standards aliases evidence_quote source_line_spans validation_flags " +
			"missing_required_attrs dedupe_key schema_version dictionary_version confidence confidence_reason " +
			"model_name prompt_name ext_info item_categories",
		jsonFields: "normalized_specs raw_specs standards aliases source_line_spans validation_flags missing_required_attrs ext_info item_categories",
	},
	"semantic_projections": {name: "semantic_projections", id: "id", scope: "input_record_id", key: "semantic_proj_id",
		fields: "semantic_proj_id language descriptive_name descriptive_name_en keywords keywords_en category_paths " +
			"category_paths_en model_name prompt_name ext_info semantic_projection semantic_projection_en line_spans",
		jsonFields: "keywords keywords_en category_paths category_paths_en ext_info line_spans",
	},
	"knowledges": {name: "knowledges", id: "id", scope: "input_record_id", key: "knowledge_id",
		fields: "knowledge_id language knowledge_type knowledge_value knowledge_value_en desc_text desc_text_en keywords " +
			"keywords_en lines category_paths category_paths_en model_name prompt_name ext_info",
		jsonFields: "keywords keywords_en lines category_paths category_paths_en ext_info",
	},
	"scene_objects": {name: "scene_objects", id: "id", scope: "input_record_id", key: "object_id",
		fields: "object_id scene_id scene_type title summary actors resources preconditions triggers states actions " +
			"constraints decisions outcomes failure_modes root_causes resolutions relationships discriminators keywords " +
			"confidence model_name prompt_name ext_info line_spans",
		jsonFields: "actors resources preconditions triggers states actions constraints decisions outcomes failure_modes root_causes resolutions relationships discriminators keywords ext_info line_spans",
	},
	"artifact_objects": {name: "artifact_objects", id: "id", scope: "source_record_id", key: "artifact_type artifact_id object_name object_role",
		fields: "input_record_id artifact_type artifact_id object_id object_name object_name_en object_name_zh language " +
			"object_type object_role aliases acronyms normalized_names description evidence_quote source_line_spans " +
			"confidence reconcile_status reconcile_confidence ext_info",
		jsonFields: "aliases acronyms normalized_names source_line_spans ext_info",
		refs:       []rowReference{{"input_record_id", "inputs", false}},
	},
	"object_nodes": {name: "object_nodes", id: "id", key: "object_id", shared: true,
		fields: "object_id canonical_object_id canonical_name canonical_name_en canonical_name_zh primary_language object_type aliases acronyms " +
			"normalized_names description reconcile_status ext_info merged_into scope_key ontological_level identity_scope " +
			"external_identifiers",
		jsonFields: "aliases acronyms normalized_names ext_info external_identifiers",
	},
	"artifact_categories": {name: "artifact_categories", id: "category_id", key: "category_type category_key", shared: true,
		fields: "category_key category_type status canonical_of display_names required_attrs specs plausible_ranges aliases " +
			"acronyms category_desc category_keywords match_keys parent_categories related_categories",
		jsonFields: "display_names required_attrs specs plausible_ranges aliases acronyms category_keywords match_keys parent_categories related_categories",
	},
	"ontology_candidates": {name: "ontology_candidates", id: "id", key: "fingerprint", shared: true,
		fields: "candidate_kind proposed_payload proposed_module_id source_type source_ref source_line_spans " +
			"discovery_method confidence fingerprint status decision_reason dependency_fingerprint identity_key proposed_by",
		jsonFields: "proposed_payload source_line_spans",
	},
	"semantic_assertions": {name: "semantic_assertions", id: "id", key: "logical_identity_key revision",
		fields: "logical_identity_key revision subject_ref_kind subject_ref_id subject_object_id predicate_term_id " +
			"object_ref_kind object_ref_id object_object_id object_literal assertion_kind_term_id polarity modality " +
			"qualifiers confidence value_form numeric_value lower_value upper_value lower_inclusive upper_inclusive " +
			"comparator unit_term_id quantity_kind_term_id raw_text status decision_reason dependency_fingerprint " +
			"superseded_by valid_time_start valid_time_end unsupported_prior_status class_identity_state_term_id " +
			"mapping_resolution_state_term_id value_state_term_id conformance_state_term_id raw_payload " +
			"raw_snapshot_fingerprint processing_error_details normalized_against_contract_revision_id " +
			"instance_of_term_id",
		jsonFields: "object_literal qualifiers raw_payload processing_error_details",
		refs:       []rowReference{{"superseded_by", "semantic_assertions", true}, {"normalized_against_contract_revision_id", "ontology_class_contract_revisions", false}},
	},
	"assertion_evidence": {name: "assertion_evidence", id: "id", scope: "input_record_id", key: "assertion_id artifact_type artifact_id evidence_role",
		fields: "assertion_id artifact_type artifact_id artifact_object_id evidence_quote source_line_spans model " +
			"prompt_version confidence evidence_role actor_kind deleted deleted_reason",
		jsonFields: "source_line_spans",
		refs:       []rowReference{{"assertion_id", "semantic_assertions", false}},
	},
	"assertion_relations": {name: "assertion_relations", id: "id", key: "assertion_id related_assertion_id relation_kind",
		fields: "assertion_id related_assertion_id relation_kind rationale",
		refs:   []rowReference{{"assertion_id", "semantic_assertions", false}, {"related_assertion_id", "semantic_assertions", false}},
	},
	"semantic_assertion_redirects": {name: "semantic_assertion_redirects", id: "id", key: "source_assertion_id target_assertion_id active",
		fields:     "source_assertion_id target_assertion_id active superseded_by_redirect_id reason evidence",
		jsonFields: "evidence",
		refs:       []rowReference{{"source_assertion_id", "semantic_assertions", false}, {"target_assertion_id", "semantic_assertions", false}, {"superseded_by_redirect_id", "semantic_assertion_redirects", true}},
	},
	"semantic_decision_candidates": {name: "semantic_decision_candidates", id: "id", scope: "input_record_id", key: "logical_identity_key revision",
		fields: "logical_identity_key revision payload_fingerprint candidate_kind proposed_payload method " +
			"resolution_outcome resolution_reason source_artifact_type source_artifact_id source_line_spans confidence " +
			"status decision_reason dependency_fingerprint superseded_by resulting_assertion_id",
		jsonFields: "proposed_payload source_line_spans",
		refs:       []rowReference{{"superseded_by", "semantic_decision_candidates", true}, {"resulting_assertion_id", "semantic_assertions", false}},
	},
	"unresolved_semantic_occurrences": {name: "unresolved_semantic_occurrences", id: "id", scope: "input_record_id",
		key:        "occurrence_key input_fingerprint dependency_fingerprint",
		fields:     "occurrence_key artifact_type artifact_id source_revision raw_payload provenance materialization_state resulting_assertion_id current_outcome_id supersedes_occurrence_id input_fingerprint dependency_fingerprint active",
		jsonFields: "raw_payload provenance",
		refs:       []rowReference{{"resulting_assertion_id", "semantic_assertions", false}, {"current_outcome_id", "semantic_processing_outcomes", false}, {"supersedes_occurrence_id", "unresolved_semantic_occurrences", true}},
	},
	"semantic_claim_identities": {name: "semantic_claim_identities", key: "claim_id", shared: true,
		fields:     "claim_id key_version canonical_key class_term_id identity_payload",
		jsonFields: "identity_payload",
	},

	"semantic_processing_outcomes": {name: "semantic_processing_outcomes", id: "id", scope: "input_record_id", key: "outcome_key input_fingerprint dependency_fingerprint",
		fields: "outcome_key artifact_type artifact_id assertion_id stage_term_id disposition_term_id execution_status " +
			"outcome_category finding_count highest_severity_term_id raw_fragment dependency_fingerprint " +
			"input_fingerprint processor_name processor_version model prompt_version supersedes_outcome_id active",
		refs: []rowReference{{"assertion_id", "semantic_assertions", false}, {"supersedes_outcome_id", "semantic_processing_outcomes", true}},
	},
	"semantic_processing_findings": {name: "semantic_processing_findings", id: "id", key: "outcome_id finding_key dependency_fingerprint",
		fields: "outcome_id finding_key dimension_term_id finding_term_id severity_term_id retry_state_term_id error_code " +
			"details dependency_fingerprint supersedes_finding_id active",
		jsonFields: "details",
		refs:       []rowReference{{"outcome_id", "semantic_processing_outcomes", false}, {"supersedes_finding_id", "semantic_processing_findings", true}},
	},
	"ontology_modules": {name: "ontology_modules", id: "id", key: "module_id", shared: true,
		fields: "module_id title description depends_on status",
	},
	"ontology_module_releases": {name: "ontology_module_releases", id: "id", key: "module_id version", shared: true,
		fields:     "module_id version title payload content_checksum dependency_releases superseded_by_release_id",
		jsonFields: "payload dependency_releases",
		refs:       []rowReference{{"superseded_by_release_id", "ontology_module_releases", true}},
	},
	"ontology_term_headers": {name: "ontology_term_headers", key: "term_id", shared: true,
		fields: "term_id term_kind module_id current_contract_revision_id",
		refs:   []rowReference{{"current_contract_revision_id", "ontology_class_contract_revisions", true}},
	},
	"ontology_terms": {name: "ontology_terms", id: "id", key: "term_id version", shared: true,
		fields:     "term_id version term_kind module_id status definition scope released_in_release_id properties",
		jsonFields: "properties",
		refs:       []rowReference{{"released_in_release_id", "ontology_module_releases", false}},
	},
	"ontology_term_revisions": {name: "ontology_term_revisions", id: "id", key: "term_id revision", shared: true,
		fields: "term_id revision source_term_row_id term_kind module_id status definition scope released_in_release_id " +
			"properties",
		jsonFields: "properties",
		refs:       []rowReference{{"source_term_row_id", "ontology_terms", false}, {"released_in_release_id", "ontology_module_releases", false}},
	},
	"ontology_term_labels": {name: "ontology_term_labels", id: "id", key: "term_id version lang label_role label", shared: true,
		fields: "term_id version label lang label_role status released_in_release_id",
		refs:   []rowReference{{"released_in_release_id", "ontology_module_releases", false}},
	},
	"ontology_term_redirects": {name: "ontology_term_redirects", id: "id", key: "source_term_id target_term_id active", shared: true,
		fields:     "source_term_id target_term_id active superseded_by_redirect_id reason evidence",
		jsonFields: "evidence",
		refs:       []rowReference{{"superseded_by_redirect_id", "ontology_term_redirects", true}},
	},
	"ontology_mappings": {name: "ontology_mappings", id: "id", key: "mapping_id version", shared: true,
		fields: "mapping_id version from_term_id to_term_id to_iri relation evidence approval_status module_id status " +
			"released_in_release_id",
		jsonFields: "evidence",
		refs:       []rowReference{{"released_in_release_id", "ontology_module_releases", false}},
	},
	"ontology_axioms": {name: "ontology_axioms", id: "id", key: "axiom_id version", shared: true,
		fields: "axiom_id version axiom_kind subject_term_id predicate_term_id object_term_id object_iri module_id status " +
			"released_in_release_id",
		refs: []rowReference{{"released_in_release_id", "ontology_module_releases", false}},
	},
	"ontology_class_contract_revisions": {name: "ontology_class_contract_revisions", id: "id", key: "term_id revision", shared: true,
		fields: "term_id revision contract_schema_version identity_schema_version definition_state contract_payload " +
			"synthesis_method confidence policy_version provenance supersedes_revision_id",
		jsonFields: "contract_payload provenance",
		refs:       []rowReference{{"supersedes_revision_id", "ontology_class_contract_revisions", true}},
	},
	"ontology_class_contract_capabilities": {name: "ontology_class_contract_capabilities", key: "contract_revision_id capability_term_id", shared: true,
		fields: "contract_revision_id capability_term_id result_state",
		refs:   []rowReference{{"contract_revision_id", "ontology_class_contract_revisions", false}},
	},
	"ontology_profiles": {name: "ontology_profiles", id: "id", key: "profile_id version", shared: true,
		fields:     "profile_id version module_id status title applicability closed_dimensions released_in_release_id",
		jsonFields: "applicability closed_dimensions",
		refs:       []rowReference{{"released_in_release_id", "ontology_module_releases", false}},
	},
	"ontology_profile_rules": {name: "ontology_profile_rules", id: "id", key: "rule_id version", shared: true,
		fields: "rule_id version profile_id profile_version rule_kind status severity rule_config applicability " +
			"released_in_release_id",
		jsonFields: "rule_config applicability",
		refs:       []rowReference{{"released_in_release_id", "ontology_module_releases", false}},
	},
}

var vocabularyTables = []string{
	"ontology_modules", "ontology_module_releases", "ontology_term_headers", "ontology_terms",
	"ontology_term_revisions", "ontology_term_labels", "ontology_term_redirects",
	"ontology_mappings", "ontology_axioms", "ontology_class_contract_revisions",
	"ontology_class_contract_capabilities", "ontology_profiles", "ontology_profile_rules",
}

var processorContractOrder = []string{
	VocabularyProcessor, "static_analyzer", "structure_analyzer", "blocking", "chunking", "extract_doc_metadata",
	"facet_tier1", "facet_tier2", "classify_document", "extract_summaries",
	"generate_summaries", "generate_topics", "extract_references", "extract_products",
	"extract_provisions", "extract_metrics", "extract_metrics_simple", "extract_entity",
	"extract_relation", "extract_inventory_items", "extract_semantic_projections",
	"extract_structured_knowledge", "generate_scene_blocks", "extract_metric_definitions",
	"extract_product_structure", "extract_test_methods", "normalize_assertions", "associate_semantics", "project_semantics",
}

const assertionScope = `t.id IN (
	SELECT assertion_id FROM kb.assertion_evidence WHERE input_record_id=$1
	UNION SELECT resulting_assertion_id FROM kb.semantic_decision_candidates WHERE input_record_id=$1
	UNION SELECT assertion_id FROM kb.semantic_processing_outcomes WHERE input_record_id=$1
	UNION SELECT resulting_assertion_id FROM kb.unresolved_semantic_occurrences WHERE input_record_id=$1
)`

const assertionOwnership = assertionScope + `
	AND NOT EXISTS (SELECT 1 FROM kb.assertion_evidence e WHERE e.assertion_id=t.id AND e.input_record_id IS DISTINCT FROM $1)
	AND NOT EXISTS (SELECT 1 FROM kb.ontology_class_resolution_decisions d WHERE d.source_assertion_id=t.id)`

var processorContracts = map[string]processorContract{
	VocabularyProcessor:    {steps: steps(vocabularyTables...)},
	"structure_analyzer":   {},
	"blocking":             {},
	"static_analyzer":      {},
	"chunking":             {steps: steps("chunks")},
	"extract_doc_metadata": {[]string{"chunking"}, steps("inputs")},
	"facet_tier1": {nil, []contractStep{
		{"doc_facets", ""}, {"doc_facet_values", "t.record_id=$1 AND t.method='deterministic'"},
	}},
	"facet_tier2": {[]string{"extract_doc_metadata"}, []contractStep{
		{"doc_facet_values", "t.record_id=$1 AND t.method='metadata'"},
	}},
	"classify_document": {[]string{"chunking"}, []contractStep{
		{"doc_facet_values", "t.record_id=$1 AND t.method='classifier'"},
	}},
	"extract_summaries":  {[]string{"chunking"}, steps("ex_summaries")},
	"generate_summaries": {[]string{"chunking"}, steps("summaries")},
	"generate_topics":    {[]string{"chunking"}, steps("topics")},
	"extract_references": {[]string{"chunking"}, []contractStep{
		{"ref_mentions", ""},
		{"reference_artifact_links", ""},
		{"metrics", "t.input_record_id=$1 AND t.ref_mention_id IS NOT NULL"},
	}},
	"extract_products":             {[]string{"chunking"}, steps("products")},
	"extract_provisions":           {[]string{"chunking"}, withObjects("provisions", "provision")},
	"extract_metrics":              {[]string{"chunking", "extract_provisions"}, metricSteps()},
	"extract_metrics_simple":       {[]string{"chunking", "extract_provisions"}, metricSteps()},
	"extract_entity":               {[]string{"chunking"}, withObjects("entities", "entity")},
	"extract_relation":             {[]string{"chunking"}, withObjects("relations", "relation")},
	"extract_inventory_items":      {[]string{"chunking"}, append(withObjects("inventory_items", "inventory_item"), contractStep{table: "inventory_item_duplicates"})},
	"extract_semantic_projections": {[]string{"chunking"}, steps("semantic_projections")},
	"extract_structured_knowledge": {[]string{"chunking"}, steps("knowledges")},
	"generate_scene_blocks":        {[]string{"chunking"}, steps("scene_objects")},
	"extract_metric_definitions": {[]string{"chunking"}, []contractStep{
		{"ontology_candidates", "t.source_ref='input_record:' || $1::text AND t.proposed_by='extract_metric_definitions'"},
	}},
	"extract_test_methods": {[]string{"chunking"}, []contractStep{
		{"ontology_candidates", "t.source_ref='input_record:' || $1::text AND t.proposed_by='extract_test_methods'"},
	}},
	"extract_product_structure": {[]string{"chunking", "extract_entity", "extract_relation"}, []contractStep{
		{"semantic_decision_candidates", "(t.input_record_id=$1 OR starts_with(t.logical_identity_key, 'product_structure:' || $1::text || ':')) AND t.source_artifact_type='product_structure'"},
	}},
	"normalize_assertions": {[]string{"chunking", "extract_metrics", "extract_metrics_simple", "extract_provisions", "extract_references"}, []contractStep{
		{"semantic_assertions", assertionScope},
		{"assertion_evidence", ""},
		{"assertion_relations", "t.assertion_id IN (SELECT assertion_id FROM kb.assertion_evidence WHERE input_record_id=$1)"},
		{"semantic_assertion_redirects", "t.source_assertion_id IN (SELECT assertion_id FROM kb.assertion_evidence WHERE input_record_id=$1)"},
	}},
	"associate_semantics": {[]string{"normalize_assertions"}, []contractStep{
		{"semantic_decision_candidates", "t.input_record_id=$1 AND t.source_artifact_type IS DISTINCT FROM 'product_structure'"},
		{"semantic_processing_outcomes", ""},
		{"unresolved_semantic_occurrences", ""},
		{"semantic_processing_findings", "t.outcome_id IN (SELECT id FROM kb.semantic_processing_outcomes WHERE input_record_id=$1)"},
	}},
	"project_semantics": {dependencies: []string{"associate_semantics"}},
}

func steps(tables ...string) []contractStep {
	result := make([]contractStep, 0, len(tables))
	for _, table := range tables {
		result = append(result, contractStep{table: table})
	}
	return result
}

func metricSteps() []contractStep {
	// These variants are mutually exclusive; either writer replaces all of
	// the extraction's rows. The planner selects the last successful writer.
	return []contractStep{
		{"metrics", "t.input_record_id=$1 AND t.ref_mention_id IS NULL"}, {"metrics_dropped", ""},
		{"artifact_objects", "t.source_record_id=$1 AND t.artifact_type='metric'"},
	}
}

func withObjects(table, artifact string) []contractStep {
	return []contractStep{{table, ""}, {"artifact_objects", "t.source_record_id=$1 AND t.artifact_type='" + artifact + "'"}}
}

var processorRegistries = map[string][]string{
	"extract_products":             {"product_names"},
	"extract_references":           {"ref_resources", "ref_resource_versions", "ref_resource_identifiers"},
	"extract_metrics":              {"object_nodes", "artifact_categories"},
	"extract_metrics_simple":       {"object_nodes", "artifact_categories"},
	"extract_provisions":           {"object_nodes", "artifact_categories"},
	"extract_entity":               {"entity_names", "object_nodes", "artifact_categories"},
	"extract_relation":             {"relation_predicates", "object_nodes", "artifact_categories"},
	"extract_inventory_items":      {"object_nodes", "artifact_categories"},
	"extract_semantic_projections": {"artifact_categories"},
	"extract_structured_knowledge": {"artifact_categories"},
	"generate_scene_blocks":        {"artifact_categories"},
	"generate_topics":              {"artifact_categories"},
	"generate_summaries":           {"artifact_categories"},
	"normalize_assertions":         {"object_nodes", "semantic_claim_identities"},
	"extract_product_structure":    {"object_nodes"},
}

func importSteps(processor string, contract processorContract) []string {
	names := append([]string{}, processorRegistries[processor]...)
	for _, step := range contract.steps {
		names = append(names, step.table)
	}
	return names
}

func isVocabularyTable(name string) bool {
	for _, table := range vocabularyTables {
		if name == table {
			return true
		}
	}
	return false
}

var governedTermFields = map[string]string{
	"metrics": "metric_definition_term_id",
	"semantic_assertions": "predicate_term_id assertion_kind_term_id unit_term_id quantity_kind_term_id instance_of_term_id " +
		"class_identity_state_term_id mapping_resolution_state_term_id value_state_term_id conformance_state_term_id",
	"semantic_processing_outcomes": "stage_term_id disposition_term_id highest_severity_term_id",
	"semantic_processing_findings": "dimension_term_id finding_term_id severity_term_id retry_state_term_id",
}
