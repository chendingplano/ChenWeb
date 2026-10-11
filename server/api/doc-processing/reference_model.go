package docprocessing

// Reference model of extract_references (openspec change extract-references-processor, devdoc
// 2026101002 §3): the governed vocabularies of every label a reference carries, and the in-memory
// shape of one reference (RefMention). The LLM proposes labels; normalizeRefMention keeps only
// governed values so search and joins stay stable.

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// refTargetSubtypes is the governed subtype vocabulary per target type (devdoc §3.1). Every type
// also accepts refSubtypeOthers; an unknown subtype becomes "others" and the proposed label is
// kept in TargetSubtypeLabel for curators.
var refTargetSubtypes = map[string][]string{
	"document":            {"report", "specification", "proposal", "manual", "file", "internal_document", "document_section", "document_element", "document_version", "document_collection"},
	"web_resource":        {"website", "web_page", "blog", "blog_post", "online_article", "wiki_page", "online_forum", "forum_post", "social_media_post", "online_documentation", "web_archive"},
	"publication":         {"journal_article", "conference_paper", "preprint", "working_paper", "technical_report", "thesis", "book", "book_chapter", "review_article", "dataset_paper", "poster_or_abstract", "research_protocol"},
	"standard":            {"international_standard", "national_standard", "industry_standard", "technical_specification", "internet_standard", "draft_standard", "standard_amendment", "standard_corrigendum", "standard_part", "standard_clause", "technical_guideline", "conformance_profile"},
	"legal":               {"law", "regulation", "directive", "administrative_rule", "legal_code_provision", "court_decision", "legal_case", "regulatory_guidance", "regulatory_notice", "government_policy", "treaty", "legislative_document"},
	"general_publication": {"book", "encyclopedia_entry", "dictionary_entry", "handbook", "magazine_article", "newspaper_article", "white_paper", "newsletter", "press_release", "brochure_or_catalog"},
	"software":            {"software_project", "source_code_file", "source_code_element", "commit", "branch_or_tag", "pull_request", "issue_or_ticket", "software_release", "software_package", "api", "api_endpoint", "software_dependency", "configuration", "container_image", "build_artifact"},
	"data":                {"dataset", "database", "database_record", "database_table", "database_row", "data_field", "statistical_series", "spreadsheet", "spreadsheet_element", "data_repository", "knowledge_graph_entity", "ontology_concept"},
	"media":               {"image", "diagram", "chart", "audio_recording", "video", "presentation", "slide", "webinar", "transcript", "map", "geospatial_layer"},
	"communication":       {"email", "email_thread", "chat_message", "discussion_thread", "meeting_record", "decision_record", "memo", "announcement", "shared_workspace_item"},
	"business_record":     {"organization", "product", "product_model", "product_specification", "patent", "trademark", "contract", "procurement_document", "certification", "audit_report", "safety_document", "test_report"},
	"other":               {"person", "event", "location", "concept_or_term", "definition", "measurement_or_metric", "mathematical_formula", "procedure", "experiment", "physical_sample", "clinical_trial"},
}

const refSubtypeOthers = "others"

var refMechanisms = refSetOf("url", "hyperlink", "doi", "isbn", "issn", "arxiv_id", "pmid", "standard_number", "rfc_number",
	"legal_citation", "patent_number", "repository_path", "file_path", "relative_path", "anchor", "numbered_citation",
	"author_year", "citation_key", "footnote", "named_reference", "structural_reference", "indirect_reference",
	"identifier", "cross_reference_field")

// refAssertedRelationships are the relationships a source text can assert. contradicts, same_as,
// version_of and part_of are derived by the system and are never accepted from extraction.
var refAssertedRelationships = refSetOf("cites", "references", "quotes", "defines_in", "based_on", "implements",
	"complies_with", "requires", "supersedes", "amends", "supports", "refutes", "extends", "uses", "depends_on",
	"derived_from", "see_also")

var refScopes = refSetOf("collection", "document", "section", "requirement", "data_element")
var refContents = refSetOf("criterion", "criteria_set", "method", "term", "requirements", "data", "general")

// Defaults that replace a value outside its vocabulary.
const (
	refDefaultTargetType   = "document"
	refDefaultMechanism    = "named_reference"
	refDefaultRelationship = "references"
	refDefaultScope        = "document"
	refDefaultContent      = "general"
)

// Values of ref_form, list_kind and normativity.
const (
	refFormListed        = "listed"
	refFormInText        = "in_text"
	refListNormative     = "normative"
	refListInformative   = "informative"
	refNormativityListed = "normative"
	refNormativityInform = "informative"
	refNormativityNone   = "unlisted"
)

// RefMention is one reference occurrence in a source document (one kb.ref_mentions row).
type RefMention struct {
	ID                 int64 // kb.ref_mentions.id once stored
	InputRecordID      int64
	RefID              string
	Section            string
	SourceLineSpans    []string
	SourceTableRows    []TableRowRef
	Evidence           string
	RefForm            string
	ListKind           string
	ListLabel          string // the entry's label in its list ("[12]", "Smith 2024"); not stored
	ListEntryIdx       int    // index of the list entry in the record's mention slice, -1 if none
	Normativity        string
	InRequirement      bool
	ListMismatch       string
	Mechanism          string
	CitedRaw           string
	Identifiers        []RefIdentifier
	TargetType         string
	TargetSubtype      string
	TargetSubtypeLabel string
	TargetTitle        string
	TargetVersion      string
	Relationships      []string
	TargetScope        string
	TargetLocator      string
	TargetContent      string
	MetricName         string
	MetricNameEn       string
	MetricSubject      string
	MetricSubjectEn    string
	Condition          string
	Internal           bool
	Confidence         float64
	ResourceID         int64
	VersionID          int64
	R1State            string
	TargetInputID      int64
	R2State            string
	R3State            string
	MetricState        string
	MentionKey         string
	Notes              []string // normalization notes, stored in ext_info
}

// normalizeRefMention replaces every label outside its governed vocabulary and records why.
func normalizeRefMention(m *RefMention) {
	note := func(field, from, to string) {
		m.Notes = append(m.Notes, fmt.Sprintf("%s %q replaced by %q", field, from, to))
	}
	m.TargetType = strings.ToLower(strings.TrimSpace(m.TargetType))
	subtypes, ok := refTargetSubtypes[m.TargetType]
	if !ok {
		note("target_type", m.TargetType, refDefaultTargetType)
		m.TargetType = refDefaultTargetType
		subtypes = refTargetSubtypes[refDefaultTargetType]
	}
	proposed := strings.ToLower(strings.TrimSpace(m.TargetSubtype))
	m.TargetSubtype = refSubtypeOthers
	for _, s := range subtypes {
		if s == proposed {
			m.TargetSubtype = proposed
		}
	}
	if m.TargetSubtype == refSubtypeOthers && proposed != "" && proposed != refSubtypeOthers {
		m.TargetSubtypeLabel = proposed
	}
	m.Mechanism = refGoverned(m.Mechanism, refMechanisms, refDefaultMechanism, "mechanism", note)
	m.TargetScope = refGoverned(m.TargetScope, refScopes, refDefaultScope, "target_scope", note)
	m.TargetContent = refGoverned(m.TargetContent, refContents, refDefaultContent, "target_content", note)

	rels := make([]string, 0, len(m.Relationships))
	seen := map[string]bool{}
	for _, r := range m.Relationships {
		r = strings.ToLower(strings.TrimSpace(r))
		if !refAssertedRelationships[r] {
			if r != "" {
				m.Notes = append(m.Notes, fmt.Sprintf("relationship %q dropped", r))
			}
			continue
		}
		if !seen[r] {
			seen[r] = true
			rels = append(rels, r)
		}
	}
	if len(rels) == 0 {
		rels = []string{refDefaultRelationship}
	}
	sort.Strings(rels)
	m.Relationships = rels

	if m.RefForm != refFormListed {
		m.RefForm = refFormInText
		m.ListKind = ""
	} else if m.ListKind != refListNormative {
		m.ListKind = refListInformative
	}
	if m.Mechanism == "structural_reference" {
		m.Internal = true
	}
}

func refGoverned(v string, vocab map[string]bool, def, field string, note func(field, from, to string)) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if vocab[v] {
		return v
	}
	note(field, v, def)
	return def
}

func refSetOf(values ...string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, v := range values {
		out[v] = true
	}
	return out
}

// refMentionKey is the stable fingerprint of a mention (design D11): unchanged across re-runs of
// the same document, so later human decisions can reattach.
func refMentionKey(m RefMention) string {
	firstLine := ""
	if len(m.SourceLineSpans) > 0 {
		firstLine = m.SourceLineSpans[0]
	}
	firstLine += refTableRowLocation(m.SourceTableRows)
	return refFingerprint(fmt.Sprint(m.InputRecordID), refNormKey(m.Section), firstLine, refNormKey(m.CitedRaw),
		refNormKey(m.MetricName), refNormKey(m.TargetLocator))
}

// metricArtifactKey is the stable key of a metric row (design D11): positional metric ids change
// on every re-extraction, this does not.
func metricArtifactKey(recordID int64, section, name, subject, condition string) string {
	return refFingerprint(fmt.Sprint(recordID), refNormKey(section), refNormKey(name), refNormKey(subject), refNormKey(condition))
}

func refFingerprint(parts ...string) string {
	sum := sha1.Sum([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(sum[:])
}

func refNormKey(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}
