package docreviews

import (
	"os"
	"sort"
	"strings"

	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
)

// ListAspects returns all review aspects with their metadata, using the default
// (English) labels and descriptions. Callers that render for a specific UI
// locale should use ListAspectsForLocale.
// The Checked field is populated from [reviewers.<aspect>].checked in doc-review.local.toml.
// Source: Document Review Checklist spec (doc-repo/specs/202606/2026061102-spec-document-review-checklist.md)
func ListAspects() []AspectInfo {
	return ListAspectsForLocale("")
}

// ListAspectsForLocale returns all review aspects with their Label/Description
// resolved for locale. The built-in labels in listAspectsBase are the ultimate
// fallback; a [reviewers.<aspect>].name_<locale>/desc_<locale> (falling back to
// name_en/desc_en) overrides them when configured. An empty locale resolves to
// the English variant.
func ListAspectsForLocale(locale string) []AspectInfo {
	base := listAspectsBase()
	for i := range base {
		base[i].DefaultModel = strings.TrimSpace(os.Getenv("DOC_REVIEW_" + strings.ToUpper(base[i].Name) + "_MODEL_NAME"))
		if base[i].DefaultModel == "" {
			base[i].DefaultModel = strings.TrimSpace(os.Getenv("DOC_REVIEW_MODEL_NAME"))
		}
	}
	cfg, _ := GetDocReviewConfig()
	if cfg == nil {
		return base
	}
	for i, a := range base {
		rev, ok := cfg.Reviewers[a.Name]
		if !ok {
			continue
		}
		if rev.Checked != nil {
			base[i].Checked = *rev.Checked
		}
		if name := rev.LocalizedName(locale); name != "" {
			base[i].Label = name
		}
		if desc := rev.LocalizedDesc(locale); desc != "" {
			base[i].Description = desc
		}
	}
	return base
}

func listAspectsBase() []AspectInfo {
	return []AspectInfo{
		// P1 — Language & Style
		{Name: "grammar_spelling", Group: "P1", Label: "Grammar & Spelling", Priority: "Should Review", Description: "Checks grammar, spelling, punctuation, and capitalization."},
		{Name: "tone_voice", Group: "P1", Label: "Tone & Voice", Priority: "Should Review", Description: "Evaluates consistency of tone and voice throughout."},
		{Name: "formatting_consistency", Group: "P1", Label: "Formatting Consistency", Priority: "Should Review", Description: "Checks for consistent use of formatting (bold, italic, lists)."},
		{Name: "readability", Group: "P1", Label: "Readability", Priority: "Should Review", Description: "Measures sentence complexity, paragraph length, and clarity."},
		{Name: "localization", Group: "P1", Label: "Localization", Priority: "Review for External/Public", Description: "Checks if content is suitable for target locale."},

		// P2 — Structure & Organization
		{Name: "logical_flow", Group: "P2", Label: "Logical Flow", Priority: "Must Review", Description: "Evaluates whether content follows a logical sequence."},
		{Name: "heading_hierarchy", Group: "P2", Label: "Heading Hierarchy", Priority: "Should Review", Description: "Validates heading nesting and consistency."},
		{Name: "toc_accuracy", Group: "P2", Label: "Table of Contents Accuracy", Priority: "Should Review", Description: "Verifies ToC matches actual headings."},
		{Name: "navigability", Group: "P2", Label: "Navigability", Priority: "Review for Regulated", Description: "Assesses how easily a reader can find relevant sections."},
		{Name: "section_balance", Group: "P2", Label: "Section Balance", Priority: "Review for External/Public", Description: "Checks if sections are proportional in length."},
		{Name: "modularity", Group: "P2", Label: "Modularity", Priority: "Review for Regulated", Description: "Evaluates whether content is self-contained and reusable."},

		// P3 — Content Quality
		{Name: "completeness", Group: "P3", Label: "Completeness", Priority: "Must Review", Description: "Checks if all required topics are covered.", IsToolUse: true},
		{Name: "correctness", Group: "P3", Label: "Correctness", Priority: "Must Review", Description: "Verifies factual accuracy of content.", IsToolUse: true},
		{Name: "clarity", Group: "P3", Label: "Clarity", Priority: "Must Review", Description: "Evaluates how clearly concepts are explained.", IsToolUse: true},
		{Name: "conciseness", Group: "P3", Label: "Conciseness", Priority: "Should Review", Description: "Checks for unnecessary verbosity."},
		{Name: "relevance", Group: "P3", Label: "Relevance", Priority: "Should Review", Description: "Ensures content is relevant to the document's purpose.", IsToolUse: true},
		{Name: "currency", Group: "P3", Label: "Currency", Priority: "Should Review", Description: "Verifies information is up-to-date."},
		{Name: "examples", Group: "P3", Label: "Examples", Priority: "Review for External/Public", Description: "Checks for adequate examples.", IsToolUse: true},
		{Name: "diagrams", Group: "P3", Label: "Diagrams", Priority: "Review for External/Public", Description: "Evaluates quality and relevance of diagrams.", IsToolUse: true},
		{Name: "testable_claims", Group: "P3", Label: "Testable Claims", Priority: "Must Review", Description: "Flags claims that should be testable.", IsToolUse: true},
		{Name: "evidence_rationale", Group: "P3", Label: "Evidence & Rationale", Priority: "Must Review", Description: "Checks if claims are supported by evidence.", IsToolUse: true},

		// P4 — Consistency
		{Name: "internal_contradictions", Group: "P4", Label: "Internal Contradictions", Priority: "Must Review", Description: "Finds contradictory statements across the document.", IsToolUse: true},
		{Name: "terminology_consistency", Group: "P4", Label: "Terminology Consistency", Priority: "Must Review", Description: "Checks for consistent use of terms.", IsToolUse: true},
		{Name: "cross_reference_correctness", Group: "P4", Label: "Cross-Reference Correctness", Priority: "Must Review", Description: "Validates cross-references point to correct targets.", IsToolUse: true},
		{Name: "requirement_traceability", Group: "P4", Label: "Requirement Traceability", Priority: "Review for Regulated", Description: "Traces requirements to their implementation.", IsToolUse: true},

		// P5 — Technical & Compliance
		{Name: "technical_accuracy", Group: "P5", Label: "Technical Accuracy", Priority: "Must Review", Description: "Verifies technical details, code snippets, formulas.", IsToolUse: true},
		{Name: "assumptions", Group: "P5", Label: "Assumptions", Priority: "Must Review", Description: "Identifies unstated or invalid assumptions.", IsToolUse: true},
		{Name: "prerequisites", Group: "P5", Label: "Prerequisites", Priority: "Should Review", Description: "Checks if prerequisites are documented.", IsToolUse: true},
		{Name: "standards_compliance", Group: "P5", Label: "Standards Compliance", Priority: "Must Review", Description: "Checks compliance with relevant standards (ISO, IEC, GB).", IsToolUse: true},
		{Name: "compliance", Group: "P5", Label: "Compliance Review", Priority: "Review for Regulated", Description: "Detects the activities the document describes (including what the document itself is) and searches the knowledge base for artifacts that could govern each one. Reports grounding candidates per activity; \"none found\" is relative to the searched knowledge base, not a compliance verdict."},
		{Name: "legal_compliance", Group: "P5", Label: "Legal Compliance", Priority: "Review for Regulated", Description: "Checks compliance with legal requirements.", IsToolUse: true},
		{Name: "regulatory_compliance", Group: "P5", Label: "Regulatory Compliance", Priority: "Review for Regulated", Description: "Checks compliance with regulatory requirements.", IsToolUse: true},
		{Name: "internal_policy", Group: "P5", Label: "Internal Policy", Priority: "Review for Regulated", Description: "Checks adherence to internal policies.", IsToolUse: true},
		{Name: "security", Group: "P5", Label: "Security", Priority: "Must Review", Description: "Identifies security vulnerabilities and concerns.", IsToolUse: true},
		{Name: "performance", Group: "P5", Label: "Performance", Priority: "Should Review", Description: "Evaluates performance implications.", IsToolUse: true},
		{Name: "error_handling", Group: "P5", Label: "Error Handling", Priority: "Should Review", Description: "Checks if error scenarios are addressed.", IsToolUse: true},
		{Name: "limitations", Group: "P5", Label: "Limitations", Priority: "Should Review", Description: "Identifies undocumented limitations.", IsToolUse: true},
		{Name: "metrics", Group: "P5", Label: "Metric Consistency", Priority: "Review for Regulated", Description: "Cross-document check: flags conflicting metric values, units, or thresholds against related metrics in other documents."},
		{Name: "provisions", Group: "P5", Label: "Provision Consistency", Priority: "Review for Regulated", Description: "Cross-document check: flags contradictory or conflicting provisions (requirements, prohibitions) against related provisions in other documents."},
		{Name: "entities", Group: "P5", Label: "Entity Consistency", Priority: "Review for Regulated", Description: "Cross-document check: flags conflicting entity types, definitions, or attributes for the same real-world entity against related entities in other documents."},
		{Name: "inventory_items", Group: "P5", Label: "Inventory Item Consistency", Priority: "Review for Regulated", Description: "Cross-document check: flags conflicting manufacturer, brand, model/part numbers, specifications, or standards for the same catalog item against related inventory items in other documents."},

		// P6 — Meta & Process
		{Name: "version_history", Group: "P6", Label: "Version History", Priority: "Should Review", Description: "Checks if version history is complete."},
		{Name: "review_status", Group: "P6", Label: "Review Status", Priority: "Should Review", Description: "Verifies review status is documented."},
		{Name: "ownership", Group: "P6", Label: "Ownership", Priority: "Should Review", Description: "Checks document ownership is identified."},
		{Name: "references", Group: "P6", Label: "References", Priority: "Must Review", Description: "Verifies all references are properly cited."},
		{Name: "related_documents", Group: "P6", Label: "Related Documents", Priority: "Should Review", Description: "Checks if related documents are referenced."},
		{Name: "confidentiality", Group: "P6", Label: "Confidentiality", Priority: "Must Review", Description: "Checks for proper confidentiality markings."},
		{Name: "sensitive_data", Group: "P6", Label: "Sensitive Data", Priority: "Must Review", Description: "Flags potentially sensitive information."},
		{Name: "pii", Group: "P6", Label: "Personally Identifiable Info", Priority: "Must Review", Description: "Detects PII exposure risks."},
		{Name: "data_retention", Group: "P6", Label: "Data Retention", Priority: "Review for Regulated", Description: "Checks if data retention is addressed."},
		{Name: "license_ip", Group: "P6", Label: "License & IP", Priority: "Review for Regulated", Description: "Checks license and intellectual property issues."},
	}
}

// tierMeta holds the display label and description for a review tier.
type tierMeta struct {
	Label string
	Desc  string
}

// defaultTierOrder fixes the display order of the built-in tiers.
var defaultTierOrder = []string{"must_review", "should_review", "review_external", "review_regulated"}

// defaultTierMeta supplies labels/descriptions for the built-in tier keys.
var defaultTierMeta = map[string]tierMeta{
	"must_review":      {Label: "Must Review", Desc: "Critical compliance and quality aspects"},
	"should_review":    {Label: "Should Review", Desc: "Recommended aspects"},
	"review_external":  {Label: "Review for External/Public", Desc: "For documents intended for external audiences"},
	"review_regulated": {Label: "Review for Regulated", Desc: "For regulated industry documents"},
}

func normalizeTierKey(k string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(k)), "-", "_")
}

func tierLabelFor(key string) (string, string) {
	if m, ok := defaultTierMeta[key]; ok {
		return m.Label, m.Desc
	}
	words := strings.FieldsFunc(key, func(r rune) bool { return r == '_' || r == '-' })
	for i, w := range words {
		if w != "" {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " "), ""
}

func validAspectNames() map[string]bool {
	m := make(map[string]bool)
	for _, a := range ListAspects() {
		m[a.Name] = true
	}
	return m
}

// ListTiers returns the review tier definitions with their aspect mappings.
func ListTiers() []TierInfo {
	if cfg := appconfig.GetDocReviewsConfig(); len(cfg) > 0 {
		return tiersFromConfig(cfg)
	}
	return defaultTiers()
}

func tiersFromConfig(cfg map[string][]string) []TierInfo {
	valid := validAspectNames()
	norm := make(map[string][]string, len(cfg))
	for rawKey, items := range cfg {
		key := normalizeTierKey(rawKey)
		filtered := make([]string, 0, len(items))
		for _, it := range items {
			name := strings.TrimSpace(it)
			if valid[name] {
				filtered = append(filtered, name)
			}
		}
		norm[key] = filtered
	}

	seen := make(map[string]bool)
	keys := make([]string, 0, len(norm))
	for _, k := range defaultTierOrder {
		if _, ok := norm[k]; ok {
			keys = append(keys, k)
			seen[k] = true
		}
	}
	extra := make([]string, 0, len(norm))
	for k := range norm {
		if !seen[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	keys = append(keys, extra...)

	tiers := make([]TierInfo, 0, len(keys))
	for _, k := range keys {
		label, desc := tierLabelFor(k)
		tiers = append(tiers, TierInfo{Key: k, Label: label, Description: desc, AspectNames: norm[k]})
	}
	return tiers
}

func defaultTiers() []TierInfo {
	all := ListAspects()
	var mustReview, shouldReview, externalReview, regulatedReview []string
	for _, a := range all {
		switch a.Priority {
		case "Must Review":
			mustReview = append(mustReview, a.Name)
		case "Should Review":
			shouldReview = append(shouldReview, a.Name)
		case "Review for External/Public":
			externalReview = append(externalReview, a.Name)
		case "Review for Regulated":
			regulatedReview = append(regulatedReview, a.Name)
		}
	}
	return []TierInfo{
		{Key: "must_review", Label: "Must Review", Description: "Critical compliance and quality aspects", AspectNames: mustReview},
		{Key: "should_review", Label: "Should Review", Description: "Recommended aspects", AspectNames: shouldReview},
		{Key: "review_external", Label: "Review for External/Public", Description: "For documents intended for external audiences", AspectNames: externalReview},
		{Key: "review_regulated", Label: "Review for Regulated", Description: "For regulated industry documents", AspectNames: regulatedReview},
	}
}

// ResolveAspectsForTier returns the aspect names for a given tier key.
func ResolveAspectsForTier(tierKey string) []string {
	for _, t := range ListTiers() {
		if t.Key == tierKey {
			return t.AspectNames
		}
	}
	return nil
}
