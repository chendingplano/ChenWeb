package agentplatformhandler

import (
	"time"

	"github.com/chendingplano/deepdoc/server/api/agenttrace"
)

// ErrorResponse mirrors promptoptimizerhandler.ErrorResponse so the frontend
// can handle errors uniformly across ChenWeb API surfaces.
type ErrorResponse struct {
	Status   bool   `json:"status"`
	ErrorMsg string `json:"error_msg"`
}

// -----------------------------------------------------------------------------
// Workspace
// -----------------------------------------------------------------------------

type Workspace struct {
	ID          string    `json:"id"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	OwnerUserID string    `json:"owner_user_id"`
	MyRole      string    `json:"my_role"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CreateWorkspaceRequest struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// -----------------------------------------------------------------------------
// Agent
// -----------------------------------------------------------------------------

type Agent struct {
	ID           string     `json:"id"`
	WorkspaceID  string     `json:"workspace_id"`
	Name         string     `json:"name"`
	AvatarEmoji  string     `json:"avatar_emoji"`
	RuntimeKind  string     `json:"runtime_kind"`
	Model        string     `json:"model"`
	Instructions string     `json:"instructions"`
	Enabled      bool       `json:"enabled"`
	ArchivedAt   *time.Time `json:"archived_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type CreateAgentRequest struct {
	Name         string `json:"name"`
	AvatarEmoji  string `json:"avatar_emoji"`
	RuntimeKind  string `json:"runtime_kind"`
	Model        string `json:"model"`
	Instructions string `json:"instructions"`
	Enabled      *bool  `json:"enabled"`
}

type UpdateAgentRequest struct {
	Name         *string `json:"name"`
	AvatarEmoji  *string `json:"avatar_emoji"`
	Model        *string `json:"model"`
	Instructions *string `json:"instructions"`
	Enabled      *bool   `json:"enabled"`
}

// -----------------------------------------------------------------------------
// Project
// -----------------------------------------------------------------------------

type Project struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CreateProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type UpdateProjectRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

// -----------------------------------------------------------------------------
// Issue
// -----------------------------------------------------------------------------

type Issue struct {
	ID              string    `json:"id"`
	WorkspaceID     string    `json:"workspace_id"`
	ProjectID       *string   `json:"project_id,omitempty"`
	IssueNumber     int       `json:"issue_number"`
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	Status          string    `json:"status"`
	Priority        int       `json:"priority"`
	BoardOrder      float64   `json:"board_order"`
	AssigneeUserID  *string   `json:"assignee_user_id,omitempty"`
	AssigneeAgentID *string   `json:"assignee_agent_id,omitempty"`
	CreatedBy       string    `json:"created_by"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type CreateIssueRequest struct {
	ProjectID       string `json:"project_id"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	Status          string `json:"status"`
	Priority        int    `json:"priority"`
	AssigneeUserID  string `json:"assignee_user_id"`
	AssigneeAgentID string `json:"assignee_agent_id"`
}

type UpdateIssueRequest struct {
	ProjectID       *string  `json:"project_id"`
	Title           *string  `json:"title"`
	Description     *string  `json:"description"`
	Status          *string  `json:"status"`
	Priority        *int     `json:"priority"`
	BoardOrder      *float64 `json:"board_order"`
	AssigneeUserID  *string  `json:"assignee_user_id"`
	AssigneeAgentID *string  `json:"assignee_agent_id"`
}

// BulkMoveItem applies a (status, board_order) tuple to a single issue.
// It's addressed by issue_number since the URL already scopes to a workspace.
type BulkMoveItem struct {
	IssueNumber int     `json:"issue_number"`
	Status      string  `json:"status"`
	BoardOrder  float64 `json:"board_order"`
}

type BulkMoveRequest struct {
	Moves []BulkMoveItem `json:"moves"`
}

// -----------------------------------------------------------------------------
// Comment
// -----------------------------------------------------------------------------

type Comment struct {
	ID            string    `json:"id"`
	IssueID       string    `json:"issue_id"`
	AuthorUserID  *string   `json:"author_user_id,omitempty"`
	AuthorAgentID *string   `json:"author_agent_id,omitempty"`
	Body          string    `json:"body"`
	CreatedAt     time.Time `json:"created_at"`
}

type CreateCommentRequest struct {
	Body string `json:"body"`
}

// -----------------------------------------------------------------------------
// Task run / events / artifacts (M1b)
// -----------------------------------------------------------------------------

type TaskRun struct {
	ID             string     `json:"id"`
	WorkspaceID    string     `json:"workspace_id"`
	IssueID        string     `json:"issue_id"`
	IssueNumber    int        `json:"issue_number"`
	AgentID        string     `json:"agent_id"`
	Status         string     `json:"status"` // queued|claimed|running|succeeded|failed|canceled
	QueuedAt       time.Time  `json:"queued_at"`
	ClaimedAt      *time.Time `json:"claimed_at,omitempty"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	ExitCode       *int       `json:"exit_code,omitempty"`
	ErrorMessage   *string    `json:"error_message,omitempty"`
	RunnerVersion  *string    `json:"runner_version,omitempty"`
	WorkdirPath    *string    `json:"workdir_path,omitempty"`
	LeaseExpiresAt *time.Time `json:"lease_expires_at,omitempty"`
}

type TaskEvent struct {
	ID        int64     `json:"id"`
	TaskRunID string    `json:"task_run_id"`
	Kind      string    `json:"kind"`
	Payload   string    `json:"payload"`
	At        time.Time `json:"at"`
}

type Artifact struct {
	ID        string    `json:"id"`
	TaskRunID string    `json:"task_run_id"`
	Path      string    `json:"path"`
	Kind      string    `json:"kind"`
	SizeBytes int64     `json:"size_bytes"`
	CreatedAt time.Time `json:"created_at"`
}

type AgentTraceRecord struct {
	ID              string                `json:"id"`
	WorkspaceID     string                `json:"workspace_id"`
	TaskRunID       string                `json:"task_run_id"`
	AgentKind       string                `json:"agent_kind"`
	ProviderTraceID string                `json:"provider_trace_id"`
	InputText       string                `json:"input_text"`
	OutputText      string                `json:"output_text"`
	ToolCallCount   int64                 `json:"tool_call_count"`
	Usage           agenttrace.TokenUsage `json:"usage"`
	TotalLatencyMS  *int64                `json:"total_latency_ms,omitempty"`
	TotalCostUSD    *float64              `json:"total_cost_usd,omitempty"`
	Trace           agenttrace.Trace      `json:"trace"`
	CreatedAt       time.Time             `json:"created_at"`
	UpdatedAt       time.Time             `json:"updated_at"`
}

type AgentTraceSummary struct {
	ID              string                `json:"id"`
	WorkspaceID     string                `json:"workspace_id"`
	TaskRunID       string                `json:"task_run_id"`
	AgentKind       string                `json:"agent_kind"`
	ProviderTraceID string                `json:"provider_trace_id"`
	OutputText      string                `json:"output_text"`
	ToolCallCount   int64                 `json:"tool_call_count"`
	Usage           agenttrace.TokenUsage `json:"usage"`
	TotalLatencyMS  *int64                `json:"total_latency_ms,omitempty"`
	TotalCostUSD    *float64              `json:"total_cost_usd,omitempty"`
	CreatedAt       time.Time             `json:"created_at"`
	UpdatedAt       time.Time             `json:"updated_at"`
}

type TraceListFilter struct {
	AgentKind string
	Limit     int
}

type TraceEvaluationRequest struct {
	ContainsAnswer []string `json:"contains_answer"`
	UsedTools      []string `json:"used_tools"`
	AvoidedTools   []string `json:"avoided_tools"`
	MaxTokens      *int     `json:"max_tokens"`
}
