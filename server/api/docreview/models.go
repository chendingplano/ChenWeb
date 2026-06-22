package docreview

// SubmitRequestInput is the request body for POST /api/v1/doc-review/requests.
type SubmitRequestInput struct {
	InputRecordID int64              `json:"input_record_id"`
	Tier          string             `json:"tier"`          // "must_review", "should_review", "custom"
	Aspects       []string           `json:"aspects"`       // selected aspect names (all when tier-based)
	ReferenceDocs []ReferenceDoc     `json:"reference_docs,omitempty"`
	Notes         string             `json:"notes,omitempty"`
	ModelOverrides map[string]ModelOverride `json:"model_overrides,omitempty"`
	RequesterName string             `json:"requester_name"`
	RequesterID   int64              `json:"requester_id"`
	ReportTemplate string            `json:"report_template,omitempty"`
	DocTemplate   string             `json:"doc_template,omitempty"`
}

type ReferenceDoc struct {
	RecordID int64  `json:"record_id"`
	DocNo    string `json:"doc_no"`
	Title    string `json:"title"`
}

type ModelOverride struct {
	ModelRef string `json:"model_ref"`
}

// RequestStatus represents a row from kb.doc_review_requests.
type RequestStatus struct {
	ID             int64               `json:"id"`
	InputRecordID  int64               `json:"input_record_id"`
	ReviewRunID    string              `json:"review_run_id,omitempty"`
	Tier           string              `json:"tier"`
	Aspects        []string            `json:"aspects"`
	ReferenceDocs  []ReferenceDoc      `json:"reference_docs,omitempty"`
	Notes          string              `json:"notes,omitempty"`
	ModelOverrides map[string]ModelOverride `json:"model_overrides,omitempty"`
	RequesterName  string              `json:"requester_name"`
	RequesterID    int64               `json:"requester_id"`
	ReportTemplate string              `json:"report_template,omitempty"`
	DocTemplate    string              `json:"doc_template,omitempty"`
	Status         string              `json:"status"`
	CreateTime     string              `json:"create_time"`
	StartTime      string              `json:"start_time,omitempty"`
	EndTime        string              `json:"end_time,omitempty"`
	ErrorMessage   string              `json:"error_message,omitempty"`
	ReportID       int64               `json:"report_id,omitempty"` // latest report for this request, if any (DR15)
}

// RequestWithFindings extends RequestStatus with findings.
type RequestWithFindings struct {
	Request  RequestStatus   `json:"request"`
	Findings []FindingItem   `json:"findings,omitempty"`
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

// ReportRow represents a row from kb.doc_review_reports (partial, for listing).
type ReportRow struct {
	ID                int64  `json:"id"`
	RequestID         int64  `json:"request_id"`
	InputRecordID     int64  `json:"input_record_id"`
	ReviewRunID       string `json:"review_run_id"`
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
	ExecutiveSummary string                 `json:"executive_summary"`
	ReportJSON       map[string]any         `json:"report_json"`
	ReportMarkdown   string                 `json:"report_markdown"`
}

// AspectInfo describes one review aspect.
type AspectInfo struct {
	Name        string `json:"name"`
	Group       string `json:"group"`       // "P1".."P6"
	Label       string `json:"label"`       // human-readable, e.g. "Grammar & Spelling"
	Priority    string `json:"priority"`    // "Must Review", "Should Review", etc.
	Description string `json:"description"`
	DefaultModel string `json:"default_model"`
	IsToolUse   bool   `json:"is_tool_use"`
}

// TierInfo describes one priority tier.
type TierInfo struct {
	Key         string   `json:"key"`         // "must_review", "should_review", etc.
	Label       string   `json:"label"`       // "Must Review"
	Description string   `json:"description"` // "Critical compliance aspects"
	AspectNames []string `json:"aspect_names"`
}

// SubmitResult is the response from POST /api/v1/doc-review/requests.
type SubmitResult struct {
	RequestID   int64  `json:"request_id"`
	Status      string `json:"status"`
	ReviewRunID string `json:"review_run_id,omitempty"`
}

// AspectStatus is one row of kb.doc_review_status (DR15) — the status of a single
// reviewed aspect within a run. An aspect is "finished" iff Status is
// "success" or "failed".
type AspectStatus struct {
	Aspect       string `json:"aspect"`
	Pass         string `json:"pass,omitempty"`
	Status       string `json:"status"` // pending | running | success | failed
	FindingCount int    `json:"finding_count"`
	ErrorMessage string `json:"error_message,omitempty"`
	StartTime    string `json:"start_time,omitempty"`
	EndTime      string `json:"end_time,omitempty"`
}

// ActiveJob is one entry in the live monitor (DR15): a review request that still
// has at least one unfinished aspect, with its full per-aspect status list.
type ActiveJob struct {
	RequestID     int64          `json:"request_id"`
	InputRecordID int64          `json:"input_record_id"`
	ReviewRunID   string         `json:"review_run_id"`
	Tier          string         `json:"tier"`
	Status        string         `json:"status"`
	RequesterName string         `json:"requester_name"`
	DocTitle      string         `json:"doc_title,omitempty"`
	CreateTime    string         `json:"create_time"`
	StartTime     string         `json:"start_time,omitempty"`
	Aspects       []AspectStatus `json:"aspects"`
}
