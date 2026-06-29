package docreviews

// SubmitRequestInput is the request body for POST /api/v1/doc-review/requests.
type SubmitRequestInput struct {
	InputRecordID  int64                    `json:"input_record_id"`
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

// FindingMetadataEnvelope is stored in kb.doc_review_findings.metadata.
type FindingMetadataEnvelope struct {
	I18N FindingI18NMetadata `json:"i18n"`
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

// RequestListFilter holds the optional filters for ListRequests. Empty fields
// (or "all" for the enum-style fields) are ignored, broadening the search.
type RequestListFilter struct {
	RequestID     string // exact id match (parsed; ignored if not a valid int)
	DocTitle      string // ILIKE on the document's title / file_name
	RequesterName string // ILIKE on requester_name
	Tier          string // exact match (empty / "all" = any)
	Status        string // exact match (empty / "all" = any)
	CreateStart   string // create_time >= (datetime-local / RFC3339)
	CreateEnd     string // create_time <= (datetime-local / RFC3339)
	Limit         int    // capped to 200; defaults to 100
}

// RequestListItem is one row of the document-review request list.
type RequestListItem struct {
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
