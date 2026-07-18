package docreviews

import "encoding/json"

// SubmitRequestInput is the request body for POST /api/v1/doc-review/requests.
type SubmitRequestInput struct {
	InputRecordID  int64                    `json:"input_record_id"`
	ReviewDepth    int                      `json:"review_depth"`
	Tier           string                   `json:"tier"`    // "must_review", "should_review", "custom"
	Aspects        []string                 `json:"aspects"` // selected aspect names (all when tier-based)
	ReferenceDocs  []ReferenceDoc           `json:"reference_docs,omitempty"`
	Notes          string                   `json:"notes,omitempty"`
	ModelOverrides map[string]ModelOverride `json:"model_overrides,omitempty"`
	RequesterName  string                   `json:"requester_name"`
	RequesterID    int64                    `json:"requester_id"`
	ReportTemplate string                   `json:"report_template,omitempty"`
	DocTemplate    string                   `json:"doc_template,omitempty"`
}

type ReferenceDoc struct {
	RecordID int64  `json:"record_id"`
	DocNo    string `json:"doc_no"`
	Title    string `json:"title"`
}

type ModelOverride struct {
	ModelRef string `json:"model_ref"`
}

// RequestStatus represents a row from kb.doc_review_requests plus the latest run's timing.
type RequestStatus struct {
	ID             int64                    `json:"id"`
	InputRecordID  int64                    `json:"input_record_id"`
	ReviewDepth    int                      `json:"review_depth"`
	LatestRunID    int64                    `json:"latest_run_id,omitempty"`
	Tier           string                   `json:"tier"`
	Aspects        []string                 `json:"aspects"`
	ReferenceDocs  []ReferenceDoc           `json:"reference_docs,omitempty"`
	Notes          string                   `json:"notes,omitempty"`
	ModelOverrides map[string]ModelOverride `json:"model_overrides,omitempty"`
	RequesterName  string                   `json:"requester_name"`
	RequesterID    int64                    `json:"requester_id"`
	ReportTemplate string                   `json:"report_template,omitempty"`
	DocTemplate    string                   `json:"doc_template,omitempty"`
	Status         string                   `json:"status"`
	RunStatus      string                   `json:"run_status,omitempty"`
	CreateTime     string                   `json:"create_time"`
	StartTime      string                   `json:"start_time,omitempty"`    // from latest run
	EndTime        string                   `json:"end_time,omitempty"`      // from latest run
	ErrorMessage   string                   `json:"error_message,omitempty"` // from latest run
	ReportID       int64                    `json:"report_id,omitempty"`     // latest report for this request, if any
}

// RequestWithFindings extends RequestStatus with findings and per-aspect statuses.
type RequestWithFindings struct {
	Request        RequestStatus       `json:"request"`
	Findings       []FindingItem       `json:"findings,omitempty"`
	AspectStatuses []AspectStatus      `json:"aspect_statuses,omitempty"`
	Packages       []ReviewPackageInfo `json:"packages,omitempty"`
}

type RequestFindingsOptions struct {
	Language string
	Pass     string
	Aspect   string
	RunID    int64
}

// FindingItem is a finding row from kb.doc_review_findings.
type FindingItem struct {
	ID           int64   `json:"id"`
	Pass         string  `json:"pass"`
	Aspect       string  `json:"aspect"`
	Severity     string  `json:"severity"`
	FindingType  string  `json:"finding_type"`
	Title        string  `json:"title"`
	Description  string  `json:"description"`
	Evidence     string  `json:"evidence,omitempty"`
	Location     string  `json:"location,omitempty"`
	Suggestion   string  `json:"suggestion,omitempty"`
	Confidence   float64 `json:"confidence"`
	ReviewStatus string  `json:"review_status"`
	// ArtifactID identifies the artifact-under-review (metric_id / prov_id /
	// inventory_item_id) this finding is about, for the per-artifact reviewers.
	ArtifactID string `json:"artifact_id,omitempty"`
	// RelatedArtifactID/RelatedRecordID identify a cross-document artifact
	// referenced by this finding, when present in finding metadata.
	RelatedArtifactID string `json:"related_artifact_id,omitempty"`
	RelatedRecordID   int64  `json:"related_record_id,omitempty"`
	// ArtifactFields/RelatedArtifactFields are the name-value snapshot of the
	// artifact under review and the matched/referenced artifact (ADR
	// 2026071603), captured at finding-creation time and read back verbatim
	// by the report renderer.
	ArtifactFields        json.RawMessage `json:"artifact_fields,omitempty"`
	RelatedArtifactFields json.RawMessage `json:"related_artifact_fields,omitempty"`
	// ModelName is the LLM model that generated this finding (ADR 2026070201
	// change log), read back from finding metadata.
	ModelName string `json:"model_name,omitempty"`
	// RunID is the review run that produced this finding, read back from
	// finding metadata (mirrors the row's real run_id column).
	RunID int64 `json:"run_id,omitempty"`
	// Metadata is the raw kb.doc_review_findings.metadata JSONB blob, exposed
	// verbatim for consumers (e.g. the Finding Details panel) that want the
	// full record rather than the specific subfields decoded above.
	Metadata json.RawMessage `json:"metadata,omitempty"`
	// ReferenceDoc is the raw kb.doc_review_findings.reference_doc JSONB blob.
	ReferenceDoc json.RawMessage `json:"reference_doc,omitempty"`
}

// FindingLocalizedContent stores localized prose for one language.
type FindingLocalizedContent struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Suggestion  string `json:"suggestion"`
	Provenance  string `json:"provenance,omitempty"`
}

// FindingTranslation is the localized subset applied to a FindingItem when a
// caller requests a specific language. It mirrors FindingLocalizedContent.
type FindingTranslation = FindingLocalizedContent

// FindingI18NMetadata stores the canonical/source language bookkeeping and all
// precomputed translations for a finding.
type FindingI18NMetadata struct {
	SchemaVersion            int                                `json:"schema_version"`
	SourceLanguage           string                             `json:"source_language,omitempty"`
	SourceLanguageConfidence float64                            `json:"source_language_confidence,omitempty"`
	CanonicalLanguage        string                             `json:"canonical_language,omitempty"`
	CanonicalOrigin          string                             `json:"canonical_origin,omitempty"`
	Translations             map[string]FindingLocalizedContent `json:"translations,omitempty"`
}

// findingMetadataReservedKeys are the top-level scalar keys in the flat metadata format.
// Any key not in this set is treated as a language code whose value is a FindingLocalizedContent.
var findingMetadataReservedKeys = map[string]bool{
	"schema_version":             true,
	"source_language":            true,
	"source_language_confidence": true,
	"canonical_language":         true,
	"canonical_origin":           true,
	"related_artifact_id":        true,
	"related_record_id":          true,
	"result_kind":                true,
	"analysis_relationship":      true,
	"artifact_fields":            true,
	"related_artifact_fields":    true,
	"model_name":                 true,
	"run_id":                     true,
}

// FindingMetadataEnvelope is stored in kb.doc_review_findings.metadata.
// JSON format: flat — metadata fields and language codes at the top level (see ADR 2026063001).
// Legacy format: {"i18n": {...}} — still readable but never written.
// RelatedArtifactID/RelatedRecordID are the structured cross-document
// reference of ADR 2026070201 AR5 §6 (zero values are omitted).
type FindingMetadataEnvelope struct {
	I18N                  FindingI18NMetadata
	RelatedArtifactID     string
	RelatedRecordID       int64
	ResultKind            string
	AnalysisRelationship  string
	ArtifactFields        json.RawMessage
	RelatedArtifactFields json.RawMessage
	ModelName             string
	RunID                 int64
}

func (e FindingMetadataEnvelope) MarshalJSON() ([]byte, error) {
	m := make(map[string]any, len(e.I18N.Translations)+9)
	m["schema_version"] = e.I18N.SchemaVersion
	if e.I18N.SourceLanguage != "" {
		m["source_language"] = e.I18N.SourceLanguage
	}
	if e.I18N.SourceLanguageConfidence != 0 {
		m["source_language_confidence"] = e.I18N.SourceLanguageConfidence
	}
	if e.I18N.CanonicalLanguage != "" {
		m["canonical_language"] = e.I18N.CanonicalLanguage
	}
	if e.I18N.CanonicalOrigin != "" {
		m["canonical_origin"] = e.I18N.CanonicalOrigin
	}
	if e.RelatedArtifactID != "" {
		m["related_artifact_id"] = e.RelatedArtifactID
	}
	if e.RelatedRecordID != 0 {
		m["related_record_id"] = e.RelatedRecordID
	}
	if e.ResultKind != "" {
		m["result_kind"] = e.ResultKind
	}
	if e.AnalysisRelationship != "" {
		m["analysis_relationship"] = e.AnalysisRelationship
	}
	if len(e.ArtifactFields) > 0 {
		m["artifact_fields"] = e.ArtifactFields
	}
	if len(e.RelatedArtifactFields) > 0 {
		m["related_artifact_fields"] = e.RelatedArtifactFields
	}
	if e.ModelName != "" {
		m["model_name"] = e.ModelName
	}
	if e.RunID != 0 {
		m["run_id"] = e.RunID
	}
	for lang, content := range e.I18N.Translations {
		m[lang] = content
	}
	return json.Marshal(m)
}

func (e *FindingMetadataEnvelope) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	// Legacy format: {"i18n": {"translations": {...}, ...}}
	if i18nRaw, ok := raw["i18n"]; ok {
		return json.Unmarshal(i18nRaw, &e.I18N)
	}
	// Flat format: metadata fields + language codes at top level.
	if sv, ok := raw["schema_version"]; ok {
		_ = json.Unmarshal(sv, &e.I18N.SchemaVersion)
	}
	if sl, ok := raw["source_language"]; ok {
		_ = json.Unmarshal(sl, &e.I18N.SourceLanguage)
	}
	if slc, ok := raw["source_language_confidence"]; ok {
		_ = json.Unmarshal(slc, &e.I18N.SourceLanguageConfidence)
	}
	if cl, ok := raw["canonical_language"]; ok {
		_ = json.Unmarshal(cl, &e.I18N.CanonicalLanguage)
	}
	if co, ok := raw["canonical_origin"]; ok {
		_ = json.Unmarshal(co, &e.I18N.CanonicalOrigin)
	}
	if ra, ok := raw["related_artifact_id"]; ok {
		_ = json.Unmarshal(ra, &e.RelatedArtifactID)
	}
	if rr, ok := raw["related_record_id"]; ok {
		_ = json.Unmarshal(rr, &e.RelatedRecordID)
	}
	if rk, ok := raw["result_kind"]; ok {
		_ = json.Unmarshal(rk, &e.ResultKind)
	}
	if ar, ok := raw["analysis_relationship"]; ok {
		_ = json.Unmarshal(ar, &e.AnalysisRelationship)
	}
	if af, ok := raw["artifact_fields"]; ok {
		e.ArtifactFields = append(json.RawMessage(nil), af...)
	}
	if raf, ok := raw["related_artifact_fields"]; ok {
		e.RelatedArtifactFields = append(json.RawMessage(nil), raf...)
	}
	if mn, ok := raw["model_name"]; ok {
		_ = json.Unmarshal(mn, &e.ModelName)
	}
	if rid, ok := raw["run_id"]; ok {
		_ = json.Unmarshal(rid, &e.RunID)
	}
	for key, val := range raw {
		if findingMetadataReservedKeys[key] {
			continue
		}
		var content FindingLocalizedContent
		if err := json.Unmarshal(val, &content); err != nil {
			continue
		}
		if content.Title == "" && content.Description == "" && content.Suggestion == "" {
			continue
		}
		if e.I18N.Translations == nil {
			e.I18N.Translations = make(map[string]FindingLocalizedContent)
		}
		e.I18N.Translations[key] = content
	}
	return nil
}

func applyFindingMetadata(f *FindingItem, data []byte) {
	if f == nil || len(data) == 0 {
		return
	}
	var metadata FindingMetadataEnvelope
	if err := json.Unmarshal(data, &metadata); err != nil {
		return
	}
	f.RelatedArtifactID = metadata.RelatedArtifactID
	f.RelatedRecordID = metadata.RelatedRecordID
	f.ArtifactFields = metadata.ArtifactFields
	f.RelatedArtifactFields = metadata.RelatedArtifactFields
	f.ModelName = metadata.ModelName
	f.RunID = metadata.RunID
}

// FindingNormalization is the result of converting reviewer output into a
// canonical storage form.
type FindingNormalization struct {
	SourceLanguage           string                             `json:"source_language"`
	SourceLanguageConfidence float64                            `json:"source_language_confidence"`
	CanonicalLanguage        string                             `json:"canonical_language"`
	CanonicalOrigin          string                             `json:"canonical_origin"`
	Canonical                FindingLocalizedContent            `json:"canonical"`
	SourceTranslation        FindingLocalizedContent            `json:"source_translation"`
	Translations             map[string]FindingLocalizedContent `json:"translations,omitempty"`
}

// ReviewPackageInfo describes a configured package group in display order.
type ReviewPackageInfo struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// ReportRow represents a row from kb.doc_review_reports (partial, for listing).
type ReportRow struct {
	ID                int64  `json:"id"`
	RequestID         int64  `json:"request_id"`
	InputRecordID     int64  `json:"input_record_id"`
	RunID             int64  `json:"run_id"`
	TotalFindings     int    `json:"total_findings"`
	HighCount         int    `json:"high_count"`
	MediumCount       int    `json:"medium_count"`
	LowCount          int    `json:"low_count"`
	OverallAssessment string `json:"overall_assessment"`
	CreateTime        string `json:"create_time"`
}

// ReportDetail is the full report with JSON content for detail view.
type ReportDetail struct {
	ReportRow
	ExecutiveSummary string         `json:"executive_summary"`
	ReportJSON       map[string]any `json:"report_json"`
	ReportMarkdown   string         `json:"report_markdown"`
}

// AspectInfo describes one review aspect.
type AspectInfo struct {
	Name         string `json:"name"`
	Group        string `json:"group"`    // "P1".."P6"
	Label        string `json:"label"`    // human-readable, e.g. "Grammar & Spelling"
	Priority     string `json:"priority"` // "Must Review", "Should Review", etc.
	Description  string `json:"description"`
	DefaultModel string `json:"default_model"`
	IsToolUse    bool   `json:"is_tool_use"`
	Checked      bool   `json:"checked"` // initial checked state from doc-review.local.toml [reviewers.<aspect>].checked
}

// TierInfo describes one priority tier.
type TierInfo struct {
	Key         string   `json:"key"`         // "must_review", "should_review", etc.
	Label       string   `json:"label"`       // "Must Review"
	Description string   `json:"description"` // "Critical compliance aspects"
	AspectNames []string `json:"aspect_names"`
}

// ReviewRunListFilter holds the optional filters for ListRuns. Empty fields
// (or "all" for the enum-style fields) are ignored, broadening the search.
type ReviewRunListFilter struct {
	RunID         string // exact run-id match (parsed; ignored if not a valid int)
	DocTitle      string // ILIKE on the document's title / file_name
	RequesterName string // ILIKE on requester_name
	Tier          string // exact match (empty / "all" = any)
	Status        string // exact match (empty / "all" = any)
	CreateStart   string // create_time >= (datetime-local / RFC3339)
	CreateEnd     string // create_time <= (datetime-local / RFC3339)
	Limit         int    // capped to 200; defaults to 100
}

// ReviewRunListItem is one row of the document-review run list.
type ReviewRunListItem struct {
	RunID         int64  `json:"run_id"`
	RequestID     int64  `json:"request_id"`
	InputRecordID int64  `json:"input_record_id"`
	DocTitle      string `json:"doc_title"`
	Tier          string `json:"tier"`
	Status        string `json:"status"`
	RequesterName string `json:"requester_name"`
	AspectCount   int    `json:"aspect_count"`
	TotalFindings int    `json:"total_findings"`
	ReportID      int64  `json:"report_id,omitempty"`
	CreateTime    string `json:"create_time"`
	StartTime     string `json:"start_time,omitempty"`
	EndTime       string `json:"end_time,omitempty"`
}

// ReportPDFFile describes one review-report PDF file found on disk.
type ReportPDFFile struct {
	RequestID  int64  `json:"request_id"`
	ReportID   int64  `json:"report_id"`
	FileName   string `json:"file_name"`
	CreateTime string `json:"create_time"`
	IsCurrent  bool   `json:"is_current"`
}

// SubmitResult is the response from POST /api/v1/doc-review/requests.
type SubmitResult struct {
	RequestID int64  `json:"request_id"`
	RunID     int64  `json:"run_id"`
	Status    string `json:"status"`
}

// StalledRun identifies a review run that needs to be re-triggered after recovery.
type StalledRun struct {
	RequestID int64
	RunID     int64
}

// AspectStatus is one row of kb.doc_review_status (DR15) — the status of a single
// reviewed aspect within a run. An aspect is "finished" iff Status is
// "success" or "failed".
type AspectStatus struct {
	Aspect       string  `json:"aspect"`
	Pass         string  `json:"pass,omitempty"`
	Status       string  `json:"status"` // pending | running | success | failed
	Progress     float64 `json:"progress"`
	FindingCount int     `json:"finding_count"`
	ErrorMessage string  `json:"error_message,omitempty"`
	StartTime    string  `json:"start_time,omitempty"`
	EndTime      string  `json:"end_time,omitempty"`
}

// ActiveJob is one entry in the live monitor (DR15): a review request that still
// has at least one unfinished aspect, with its full per-aspect status list.
type ActiveJob struct {
	RequestID     int64          `json:"request_id"`
	InputRecordID int64          `json:"input_record_id"`
	RunID         int64          `json:"run_id"`
	Tier          string         `json:"tier"`
	Status        string         `json:"status"`
	RequesterName string         `json:"requester_name"`
	DocTitle      string         `json:"doc_title,omitempty"`
	CreateTime    string         `json:"create_time"`
	StartTime     string         `json:"start_time,omitempty"`
	Aspects       []AspectStatus `json:"aspects"`
}
