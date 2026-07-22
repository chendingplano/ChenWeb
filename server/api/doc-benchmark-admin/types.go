package docbenchmarkadmin

import "time"

const (
	DefaultScope         = "default"
	StepRuntimeConfig    = "runtime-config"
	StepRoots            = "roots"
	StepWorkingCopy      = "working-copy"
	StepValidate         = "validate"
	StepRun              = "run"
	StepReport           = "report"
	StepCompare          = "compare"
	StatusCompleted      = "completed"
	StatusReady          = "ready"
	StatusBlocked        = "blocked"
	StatusRunning        = "running"
	StatusFailed         = "failed"
	StatusUnknown        = "unknown"
	JobQueued            = "queued"
	JobRunning           = "running"
	JobSucceeded         = "succeeded"
	JobFailed            = "failed"
)

type Config struct {
	Scope             string `json:"scope"`
	ExperimentPath    string `json:"experiment_path"`
	DatasetRoot       string `json:"dataset_root"`
	ArtifactRoot      string `json:"artifact_root"`
	WorkRoot          string `json:"work_root"`
	EvidenceRoot      string `json:"evidence_root"`
	StoreID           int64  `json:"store_id"`
	Owner             string `json:"owner"`
	TenantID          string `json:"tenant_id"`
	MetricsModelName  string `json:"metrics_model_name"`
	AllowDirty        bool   `json:"allow_dirty"`
	ReportFormat      string `json:"report_format"`
	ReportOutputPath  string `json:"report_output_path"`
	MetricsBaseline   string `json:"metrics_baseline"`
	MetricsCandidate  string `json:"metrics_candidate"`
	ChunkBaseline     string `json:"chunk_baseline"`
	ChunkCandidate    string `json:"chunk_candidate"`
	CreatedAt         string `json:"created_at,omitempty"`
	UpdatedAt         string `json:"updated_at,omitempty"`
}

type StepState struct {
	ID           string         `json:"id"`
	Title        string         `json:"title"`
	Description  string         `json:"description"`
	Status       string         `json:"status"`
	Message      string         `json:"message"`
	Detected     map[string]any `json:"detected,omitempty"`
	CompletedAt  string         `json:"completed_at,omitempty"`
	FailedAt     string         `json:"failed_at,omitempty"`
	RunningJobID int64          `json:"running_job_id,omitempty"`
}

type SetupState struct {
	Config          Config      `json:"config"`
	Steps           []StepState `json:"steps"`
	ActiveJobs      []Job       `json:"active_jobs"`
	RecentJobs      []Job       `json:"recent_jobs"`
	LastExperimentID string     `json:"last_experiment_id,omitempty"`
}

type Job struct {
	ID         int64          `json:"id"`
	Scope      string         `json:"scope"`
	StepID     string         `json:"step_id"`
	JobType    string         `json:"job_type"`
	Status     string         `json:"status"`
	Message    string         `json:"message"`
	Request    map[string]any `json:"request,omitempty"`
	Result     map[string]any `json:"result,omitempty"`
	ErrorText  string         `json:"error_text,omitempty"`
	CreatedBy  string         `json:"created_by,omitempty"`
	CreatedAt  string         `json:"created_at,omitempty"`
	StartedAt  string         `json:"started_at,omitempty"`
	FinishedAt string         `json:"finished_at,omitempty"`
	UpdatedAt  string         `json:"updated_at,omitempty"`
}

type runRequest struct {
	StepID string `json:"step_id"`
}

func terminalJobStatus(status string) bool {
	return status == JobSucceeded || status == JobFailed
}

func formatTS(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.UTC().Format(time.RFC3339)
}
