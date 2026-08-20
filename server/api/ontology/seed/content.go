// Curated vocabulary for the core 4a ontology modules (ADR 2026072901 DR1,
// DR13; spec 2026072702). This is the platform-owned, governed baseline --
// authored as data (the 2026-07-31 storage decision), released and activated
// through the module compiler. Terms are the minimum the pilot domain and the
// facet vocabulary need; nothing here requires a code change to install.
package seed

type seedLabel struct {
	Label string
	Lang  string
	Role  string
}

type seedTerm struct {
	ID     string // namespaced term_id, e.g. core:assertion
	Kind   string // one of terms.AllowedTermKinds
	Def    string
	Labels []seedLabel
}

func enPref(label string) []seedLabel {
	return []seedLabel{{Label: label, Lang: "en", Role: "prefLabel"}}
}

// coreModule owns the contract between processors and the ontology: referent,
// information artifact, assertion, evidence, agent, role, and the semantic-role
// and part-hierarchy predicates (DR20).
var coreModule = moduleContent{
	ModuleID:  "core",
	Version:   "1.0.0",
	Title:     "Core semantic module",
	Owner:     "platform",
	DependsOn: []string{},
	Terms: []seedTerm{
		{ID: "core:referent", Kind: "class", Def: "A canonical real-world or conceptual entity that SemOS tracks under governed identity.", Labels: enPref("referent")},
		{ID: "core:information_artifact", Kind: "class", Def: "A durable carrier of information -- a document, clause, or block -- with line-level provenance.", Labels: enPref("information artifact")},
		{ID: "core:assertion", Kind: "class", Def: "A qualified claim made by a source about a referent, normalized into governed predicates.", Labels: enPref("assertion")},
		{ID: "core:evidence", Kind: "class", Def: "Provenance that supports or contradicts an assertion.", Labels: enPref("evidence")},
		{ID: "core:agent", Kind: "class", Def: "An actor -- manufacturer, authority, person -- that makes claims or performs actions.", Labels: enPref("agent")},
		{ID: "core:role", Kind: "class", Def: "A part an entity plays in a context, such as product or component.", Labels: enPref("role")},
		{ID: "core:occurrence", Kind: "class", Def: "An event, action, or state with participants.", Labels: enPref("occurrence")},
		{ID: "core:value", Kind: "class", Def: "A normalized value form attached to an assertion.", Labels: enPref("value")},
		{ID: "core:part", Kind: "class", Def: "A part or component in a product structure.", Labels: enPref("part")},
		{ID: "core:instance_of", Kind: "property", Def: "Relates a referent to the class it is an instance of.", Labels: enPref("instance of")},
		{ID: "core:plays_role", Kind: "property", Def: "Relates a referent to a role it plays in a scope.", Labels: enPref("plays role")},
		{ID: "core:part_of", Kind: "property", Def: "Transitive part-hood, scoped to a product configuration (DR20).", Labels: enPref("part of")},
		{ID: "core:component_of", Kind: "property", Def: "Immediate-parent part-hood for display, non-transitive (DR20).", Labels: enPref("component of")},
		{ID: "core:variant_of", Kind: "property", Def: "An alternative part fulfilling the same function (DR20).", Labels: enPref("variant of")},
		{ID: "core:about", Kind: "property", Def: "Relates an assertion to the referent it is about.", Labels: enPref("about")},
		{ID: "core:has_evidence", Kind: "property", Def: "Relates an assertion to its supporting evidence.", Labels: enPref("has evidence")},
		{ID: "core:asserted_by", Kind: "property", Def: "Relates an assertion to the agent or source that made it.", Labels: enPref("asserted by")},
		{ID: "core:has_polarity", Kind: "property", Def: "The polarity of a claim (positive or negative).", Labels: enPref("has polarity")},
		{ID: "core:has_confidence", Kind: "property", Def: "The confidence assigned to a claim.", Labels: enPref("has confidence")},
		{ID: "core:aligns_to_term", Kind: "property", Def: "Binds a keyword concept to the governed term it is an accepted alias of (spec 2026080403 §16.1/§16.2, REQ-2). The assignment side of the governed bridge: auto-proposed, auto-accepted above a threshold, with method/score/evidence recorded on the assertion.", Labels: enPref("aligns to term")},
	},
}

// documentAuthorityModule owns document kind, issuer, jurisdiction, edition,
// normative status, supersession, and the DR4 document-facet vocabulary.
var documentAuthorityModule = moduleContent{
	ModuleID:  "document-authority",
	Version:   "1.0.0",
	Title:     "Document authority module",
	Owner:     "platform",
	DependsOn: []string{"core"},
	Terms: []seedTerm{
		{ID: "da:document_kind", Kind: "class", Def: "The kind of a document -- standard, specification, regulation, report, manual.", Labels: enPref("document kind")},
		{ID: "da:issuer", Kind: "class", Def: "The organization that issues a document.", Labels: enPref("issuer")},
		{ID: "da:jurisdiction", Kind: "class", Def: "The jurisdiction a document applies in.", Labels: enPref("jurisdiction")},
		{ID: "da:edition", Kind: "class", Def: "A specific edition or version of a document.", Labels: enPref("edition")},
		{ID: "da:normative_status", Kind: "class", Def: "Whether a document or clause is normative or informative.", Labels: enPref("normative status")},
		{ID: "da:supersedes", Kind: "property", Def: "Relates an edition to the edition it supersedes.", Labels: enPref("supersedes")},
		{ID: "da:amends", Kind: "property", Def: "Relates an edition to the edition it amends.", Labels: enPref("amends")},
		{ID: "da:cites", Kind: "property", Def: "Relates a document to another document it cites.", Labels: enPref("cites")},
		{ID: "da:is_normative", Kind: "property", Def: "Whether a document or clause is normative.", Labels: enPref("is normative")},
		{ID: "da:effective_interval", Kind: "property", Def: "The interval during which an edition is effective.", Labels: enPref("effective interval")},
		// DR4 facet vocabulary: facet keys and their permitted values are
		// governed terms, so routing/profile rules never use free text.
		{ID: "da:doc_kind", Kind: "concept", Def: "Facet key: the document kind.", Labels: enPref("facet: document kind")},
		{ID: "da:domain", Kind: "concept", Def: "Facet key: the domain of the document.", Labels: enPref("facet: domain")},
		{ID: "da:normative", Kind: "concept", Def: "Facet key: whether the document is normative.", Labels: enPref("facet: normative status")},
		{ID: "da:jurisdiction_facet", Kind: "concept", Def: "Facet key: the jurisdiction.", Labels: enPref("facet: jurisdiction")},
		{ID: "da:language", Kind: "concept", Def: "Facet key: the source language.", Labels: enPref("facet: language")},
		{ID: "da:doc_kind_standard", Kind: "concept", Def: "Facet value: the document is a standard.", Labels: enPref("facet value: standard")},
		{ID: "da:doc_kind_specification", Kind: "concept", Def: "Facet value: the document is a specification.", Labels: enPref("facet value: specification")},
		{ID: "da:doc_kind_regulation", Kind: "concept", Def: "Facet value: the document is a regulation.", Labels: enPref("facet value: regulation")},
		{ID: "da:doc_kind_report", Kind: "concept", Def: "Facet value: the document is a report.", Labels: enPref("facet value: report")},
		{ID: "da:doc_kind_manual", Kind: "concept", Def: "Facet value: the document is a manual.", Labels: enPref("facet value: manual")},
		// Tier-1 facet keys (spec ADR 2026072901 §3.5 DR4; §16.1 "Facet
		// tiers 1-2"): deterministic, computed once from the static
		// analyzer's line file, no doc_metadata or LLM dependency. Value
		// kinds are numeric/boolean, not enumerated -- unlike doc_kind,
		// these are measurements, not a fixed vocabulary, so no permitted-
		// value terms are seeded for them.
		{ID: "da:page_count", Kind: "concept", Def: "Facet key: the document's page count.", Labels: enPref("facet: page count")},
		{ID: "da:toc_presence", Kind: "concept", Def: "Facet key: whether the document has a detected table of contents.", Labels: enPref("facet: table-of-contents presence")},
		{ID: "da:heading_count", Kind: "concept", Def: "Facet key: the number of lines classified as a heading.", Labels: enPref("facet: heading count")},
		{ID: "da:language_mix", Kind: "concept", Def: "Facet key: the CJK-to-Latin character ratio across the document's lines.", Labels: enPref("facet: language mix")},
		{ID: "da:table_line_ratio", Kind: "concept", Def: "Facet key: the fraction of lines classified as tabular.", Labels: enPref("facet: table-line ratio")},
		{ID: "da:numeric_unit_density", Kind: "concept", Def: "Facet key: the fraction of lines containing a number-with-unit pattern.", Labels: enPref("facet: numeric-with-unit density")},
		{ID: "da:modal_verb_density", Kind: "concept", Def: "Facet key: the fraction of lines containing a normative modal verb (shall/must/应/必须).", Labels: enPref("facet: modal-verb density")},
		{ID: "da:figure_density", Kind: "concept", Def: "Facet key: the fraction of lines that look like a figure or table caption.", Labels: enPref("facet: figure density")},
		// Tier-2 facet keys: derived from extract_doc_metadata's
		// already-extracted output (doc_no, publish_date), no new LLM call.
		{ID: "da:publish_date_facet", Kind: "concept", Def: "Facet key: the document's publish date, from extract_doc_metadata.", Labels: enPref("facet: publish date")},
		{ID: "da:authority_hint", Kind: "concept", Def: "Facet key: the issuing-body hint derived from the document number's prefix pattern (GB/ISO/IEC/ANSI/...).", Labels: enPref("facet: authority hint")},
		{ID: "da:authority_hint_gb", Kind: "concept", Def: "Facet value: document number matches a Chinese national/industry standard prefix (GB/GB-T/HG/JB/...).", Labels: enPref("facet value: GB/China")},
		{ID: "da:authority_hint_iso", Kind: "concept", Def: "Facet value: document number matches an ISO prefix.", Labels: enPref("facet value: ISO")},
		{ID: "da:authority_hint_iec", Kind: "concept", Def: "Facet value: document number matches an IEC prefix.", Labels: enPref("facet value: IEC")},
		{ID: "da:authority_hint_ansi", Kind: "concept", Def: "Facet value: document number matches an ANSI prefix.", Labels: enPref("facet value: ANSI")},
		{ID: "da:authority_hint_astm", Kind: "concept", Def: "Facet value: document number matches an ASTM prefix.", Labels: enPref("facet value: ASTM")},
		{ID: "da:authority_hint_unknown", Kind: "concept", Def: "Facet value: document number present but its prefix matches no known authority pattern.", Labels: enPref("facet value: unknown authority")},
		{ID: "da:normative_value", Kind: "concept", Def: "Facet value: the document is normative.", Labels: enPref("facet value: normative")},
		{ID: "da:informative_value", Kind: "concept", Def: "Facet value: the document is informative.", Labels: enPref("facet value: informative")},
	},
}

// measurementModule owns the metric-definition contract and the metric
// assertion kinds (research §5.4): metric definition vs assertion, observable
// property, feature of interest, procedure, condition, aggregation.
var measurementModule = moduleContent{
	ModuleID:  "measurement",
	Version:   "1.0.0",
	Title:     "Measurement module",
	Owner:     "platform",
	DependsOn: []string{"core", "quantity"},
	Terms: []seedTerm{
		{ID: "mea:metric_definition", Kind: "metric_definition", Def: "The governed definition of a metric: canonical name, alternative names, value type, range type, quantity kind, permitted units (DR23).", Labels: enPref("metric definition")},
		{ID: "mea:metric_assertion", Kind: "class", Def: "A normalized assertion assigning a value to a metric for a feature of interest.", Labels: enPref("metric assertion")},
		{ID: "mea:observable_property", Kind: "class", Def: "The property being measured, such as luminance or response time.", Labels: enPref("observable property")},
		{ID: "mea:feature_of_interest", Kind: "class", Def: "The entity a metric is asserted about, such as a display module.", Labels: enPref("feature of interest")},
		{ID: "mea:procedure", Kind: "class", Def: "The test method or procedure that produces the value.", Labels: enPref("procedure")},
		{ID: "mea:condition", Kind: "class", Def: "The operating or test conditions under which an assertion applies.", Labels: enPref("condition")},
		{ID: "mea:aggregation_window", Kind: "class", Def: "The time or count window an aggregated metric covers.", Labels: enPref("aggregation window")},
		// Metric assertion kinds -- the governed vocabulary for what a metric
		// claim means (spec §16.3 item 4, CQ-M04).
		{ID: "mea:lower_bound_requirement", Kind: "property", Def: "Assertion kind: the value must be at least the stated limit.", Labels: enPref("lower bound requirement")},
		{ID: "mea:upper_bound_requirement", Kind: "property", Def: "Assertion kind: the value must be at most the stated limit.", Labels: enPref("upper bound requirement")},
		{ID: "mea:interval_requirement", Kind: "property", Def: "Assertion kind: the value must fall within the stated interval.", Labels: enPref("interval requirement")},
		{ID: "mea:observed_value", Kind: "property", Def: "Assertion kind: a measured test result.", Labels: enPref("observed value")},
		{ID: "mea:target", Kind: "property", Def: "Assertion kind: a design target value.", Labels: enPref("target")},
		{ID: "mea:reference", Kind: "property", Def: "Assertion kind: a reference value for comparison.", Labels: enPref("reference")},
		{ID: "mea:capability", Kind: "property", Def: "Assertion kind: a capability claim of the product.", Labels: enPref("capability")},
		{ID: "mea:has_quantity_kind", Kind: "property", Def: "Binds a metric or value to its quantity kind.", Labels: enPref("has quantity kind")},
		{ID: "mea:has_unit", Kind: "property", Def: "Binds a value to its unit term.", Labels: enPref("has unit")},
		{ID: "mea:measured_by", Kind: "property", Def: "Binds a metric to the procedure that measures it.", Labels: enPref("measured by")},
	},
}

// provisionModule owns the provision-family assertion contract: the deontic
// predicate binding a subject to a governing provision, and the deontic
// assertion kinds (required/prohibited/permitted) a provision's modality
// resolves to (ADR 2026081801 task 7.6). Mirrors measurementModule's shape --
// a family-owned module for family-specific assertion kinds, distinct from
// coreModule's cross-family predicates -- but has no external dependency
// like measurement's on quantity/QUDT, so it joins the strict startup batch
// directly.
var provisionModule = moduleContent{
	ModuleID:  "provision",
	Version:   "1.0.0",
	Title:     "Provision module",
	Owner:     "platform",
	DependsOn: []string{"core"},
	Terms: []seedTerm{
		{ID: "prov:has_provision", Kind: "property", Def: "Binds a subject to a normative provision governing it.", Labels: enPref("has provision")},
		// Deontic assertion kinds -- what a provision's modality means
		// (mirrors mea:*'s assertion-kind terms; provisionAssertionKind maps
		// modality "recommended" onto kind "permitted" rather than a fourth
		// term, since a recommendation is a weaker permission, not a distinct
		// deontic category).
		{ID: "prov:required", Kind: "property", Def: "Assertion kind: the subject is required to satisfy the stated provision.", Labels: enPref("required")},
		{ID: "prov:prohibited", Kind: "property", Def: "Assertion kind: the subject is prohibited from the stated provision.", Labels: enPref("prohibited")},
		{ID: "prov:permitted", Kind: "property", Def: "Assertion kind: the subject is permitted the stated provision.", Labels: enPref("permitted")},
	},
}

// semanticProcessingModule owns the cross-family state vocabulary decided by
// ADR 2026081801 DR4 and DR9: outcome dispositions, finding terms, finding
// dimensions, severities, retry states, required processing stages, and the
// independent state axes (class identity, mapping resolution, value,
// conformance, evidence role).
//
// These are governed ontology terms rather than database enums on purpose
// (DR4): a new finding term must be installable without a schema migration.
// The local names are the exact normative machine identifiers from DR9 --
// persisted state, API payloads, dependency fingerprints, and canonical
// serialization use them verbatim, and hyphenated or natural-language variants
// are rejected. User interfaces may render friendlier labels.
//
// term_kind is "concept" throughout: these are controlled-vocabulary values,
// not classes of real-world referents (core:*) and not properties.
var semanticProcessingModule = moduleContent{
	ModuleID:  "semantic-processing",
	Version:   "1.0.0",
	Title:     "Semantic processing state module",
	Owner:     "platform",
	DependsOn: []string{"core"},
	Terms: []seedTerm{
		// Outcome dispositions (DR4). Disposition describes what the stage
		// managed to produce; it never selects the canonical identity branch.
		{ID: "semantic:normalized", Kind: "concept", Def: "The stage produced an authoritative normalized representation.", Labels: enPref("normalized")},
		{ID: "semantic:raw_preserved", Kind: "concept", Def: "The stage preserved the source claim without a complete authoritative normalization.", Labels: enPref("raw preserved")},
		{ID: "semantic:not_applicable", Kind: "concept", Def: "The stage does not apply to this artifact.", Labels: enPref("not applicable")},
		{ID: "semantic:no_result", Kind: "concept", Def: "The stage ran but could produce no result, and recorded why.", Labels: enPref("no result")},

		// Finding terms (DR4). Each names one content-level problem; several
		// may coexist under one outcome envelope.
		{ID: "semantic:mapping_unresolved", Kind: "concept", Def: "A governed mapping for the observed raw value is missing or only proposed.", Labels: enPref("mapping unresolved")},
		{ID: "semantic:mapping_ambiguous", Kind: "concept", Def: "A governed mapping exists but no single canonical target can be selected.", Labels: enPref("mapping ambiguous")},
		{ID: "semantic:unparsed", Kind: "concept", Def: "The source literal itself could not be parsed into a semantic value.", Labels: enPref("unparsed")},
		{ID: "semantic:value_missing", Kind: "concept", Def: "The source mentions the artifact but supplies no expected value.", Labels: enPref("value missing")},
		{ID: "semantic:value_unknown", Kind: "concept", Def: "Raw evidence exists but no other value state can yet be determined.", Labels: enPref("value unknown")},
		{ID: "semantic:datatype_mismatch", Kind: "concept", Def: "The observed datatype conflicts with the selected class contract.", Labels: enPref("datatype mismatch")},
		{ID: "semantic:contract_violation", Kind: "concept", Def: "The claim violates a rule in its class contract.", Labels: enPref("contract violation")},
		{ID: "semantic:class_provisional", Kind: "concept", Def: "No existing class resolved, so a provisional class was created.", Labels: enPref("class provisional")},
		{ID: "semantic:class_ambiguous", Kind: "concept", Def: "Several classes remain plausible for this occurrence.", Labels: enPref("class ambiguous")},
		{ID: "semantic:identity_evidence_conflict", Kind: "concept", Def: "Candidate identity evidence conflicts; recorded as a finding, never stored in the class-identity state field.", Labels: enPref("identity evidence conflict")},
		{ID: "semantic:source_conflict", Kind: "concept", Def: "This source claim conflicts with another source claim.", Labels: enPref("source conflict")},
		{ID: "semantic:no_verdict", Kind: "concept", Def: "A comparison or evaluation could produce no verdict because a required capability is absent.", Labels: enPref("no verdict")},

		// Finding dimensions (DR4): which axis a finding belongs to. Findings
		// are keyed within a stage's declared decision scopes, and the
		// dimension is what makes several simultaneous findings legible.
		{ID: "semantic:dimension_mapping", Kind: "concept", Def: "Findings about governed vocabulary mapping.", Labels: enPref("mapping dimension")},
		{ID: "semantic:dimension_value", Kind: "concept", Def: "Findings about the value literal and its datatype.", Labels: enPref("value dimension")},
		{ID: "semantic:dimension_class", Kind: "concept", Def: "Findings about class identity and resolution.", Labels: enPref("class dimension")},
		{ID: "semantic:dimension_conformance", Kind: "concept", Def: "Findings about conformance with a class contract.", Labels: enPref("conformance dimension")},
		{ID: "semantic:dimension_identity", Kind: "concept", Def: "Findings about canonical claim identity.", Labels: enPref("identity dimension")},
		{ID: "semantic:dimension_conflict", Kind: "concept", Def: "Findings about conflicts between sources or instances.", Labels: enPref("conflict dimension")},

		// Finding severities. Severity is a user-facing signal only: DR3 is
		// explicit that an error-severity finding does not become an
		// execution failure.
		{ID: "semantic:severity_info", Kind: "concept", Def: "Informational finding.", Labels: enPref("info")},
		{ID: "semantic:severity_warning", Kind: "concept", Def: "Finding a consumer should be warned about.", Labels: enPref("warning")},
		{ID: "semantic:severity_error", Kind: "concept", Def: "Finding presented to users as an error; it does not fail processor execution.", Labels: enPref("error")},

		// Retry states (DR10).
		{ID: "semantic:retry_pending", Kind: "concept", Def: "The finding is retryable and awaiting a dependency change.", Labels: enPref("retry pending")},
		{ID: "semantic:retry_scheduled", Kind: "concept", Def: "A targeted retry has been scheduled for this finding.", Labels: enPref("retry scheduled")},
		{ID: "semantic:retry_not_retryable", Kind: "concept", Def: "No dependency change can resolve this finding.", Labels: enPref("not retryable")},
		{ID: "semantic:retry_stale", Kind: "concept", Def: "The retry target no longer matches the current source or dependency.", Labels: enPref("retry stale")},

		// Required semantic stages for the metric adapter (DR5, DR13). The
		// adapter declares which of these it requires; the completeness
		// projection reports against that declaration.
		{ID: "semantic:stage_normalize", Kind: "concept", Def: "Normalization of a raw artifact occurrence into semantic values.", Labels: enPref("normalize stage")},
		{ID: "semantic:stage_class_resolution", Kind: "concept", Def: "Resolution of the ontology class an occurrence instantiates.", Labels: enPref("class resolution stage")},
		{ID: "semantic:stage_associate", Kind: "concept", Def: "Association of a normalized or raw-preserved occurrence with a semantic instance and evidence.", Labels: enPref("associate stage")},

		// Class identity states (DR9). candidate_evidence_conflict is the
		// state; semantic:identity_evidence_conflict above is the finding.
		{ID: "semantic:resolved_existing", Kind: "concept", Def: "The occurrence resolved to an existing class.", Labels: enPref("resolved existing")},
		{ID: "semantic:provisional_new", Kind: "concept", Def: "No existing class resolved, so a provisional class was created.", Labels: enPref("provisional new")},
		{ID: "semantic:ambiguous_candidates", Kind: "concept", Def: "Several classes remain plausible; the instance points at a deterministic provisional class.", Labels: enPref("ambiguous candidates")},
		{ID: "semantic:candidate_evidence_conflict", Kind: "concept", Def: "Candidate class evidence conflicts.", Labels: enPref("candidate evidence conflict")},

		// Mapping resolution states (DR9).
		{ID: "semantic:mapping_resolved", Kind: "concept", Def: "A governed mapping selected one authoritative canonical target.", Labels: enPref("mapping resolved")},
		{ID: "semantic:mapping_state_unresolved", Kind: "concept", Def: "No approved governed mapping is available.", Labels: enPref("mapping state unresolved")},
		{ID: "semantic:mapping_state_ambiguous", Kind: "concept", Def: "The governed mapping is recorded as ambiguous.", Labels: enPref("mapping state ambiguous")},
		{ID: "semantic:mapping_not_required", Kind: "concept", Def: "Mapping does not apply to this occurrence.", Labels: enPref("mapping not required")},

		// Value states (DR9, and ADR 2026081701 DR8's table).
		{ID: "semantic:value_present", Kind: "concept", Def: "A usable normalized value or interval exists.", Labels: enPref("value present")},
		{ID: "semantic:value_state_missing", Kind: "concept", Def: "The document mentions the artifact but supplies no expected value.", Labels: enPref("value state missing")},
		{ID: "semantic:value_state_unparsed", Kind: "concept", Def: "A source value exists but normalization cannot parse it.", Labels: enPref("value state unparsed")},
		{ID: "semantic:value_state_datatype_mismatch", Kind: "concept", Def: "The observed datatype conflicts with the selected class contract.", Labels: enPref("value state datatype mismatch")},
		{ID: "semantic:value_state_unknown", Kind: "concept", Def: "The occurrence and raw evidence exist, but no other value state can yet be determined.", Labels: enPref("value state unknown")},
		{ID: "semantic:value_state_not_applicable", Kind: "concept", Def: "The source explicitly says the artifact does not apply.", Labels: enPref("value state not applicable")},

		// Conformance states (DR9).
		{ID: "semantic:conforms", Kind: "concept", Def: "The claim conforms with its class contract.", Labels: enPref("conforms")},
		{ID: "semantic:conformance_contract_violation", Kind: "concept", Def: "The claim violates its class contract.", Labels: enPref("conformance contract violation")},
		{ID: "semantic:not_evaluated", Kind: "concept", Def: "Conformance has not been evaluated.", Labels: enPref("not evaluated")},

		// Evidence roles (DR9). The persisted/API identifiers remain the bare
		// strings "supports" and "contradicts"; these are their governed term
		// IDs.
		{ID: "semantic:evidence_supports", Kind: "concept", Def: "Evidence supporting an assertion; persisted and returned as the identifier \"supports\".", Labels: enPref("evidence supports")},
		{ID: "semantic:evidence_contradicts", Kind: "concept", Def: "Evidence contradicting an assertion; persisted and returned as the identifier \"contradicts\".", Labels: enPref("evidence contradicts")},
	},
}
