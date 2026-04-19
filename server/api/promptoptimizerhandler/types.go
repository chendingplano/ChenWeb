package promptoptimizerhandler

import (
	"encoding/json"
	"time"
)

// ErrorResponse mirrors docgenhandler.ErrorResponse so frontend code can
// handle errors uniformly across ChenWeb API surfaces.
type ErrorResponse struct {
	Status   bool   `json:"status"`
	ErrorMsg string `json:"error_msg"`
}

// Model is the sanitized view of a row in prompt_optimizer_models. The
// encrypted API key is intentionally omitted — it never leaves the server.
type Model struct {
	ID            string          `json:"id"`
	UserID        string          `json:"user_id"`
	Name          string          `json:"name"`
	Provider      string          `json:"provider"`
	BaseURL       string          `json:"base_url,omitempty"`
	Model         string          `json:"model"`
	DefaultParams json.RawMessage `json:"default_params"`
	Enabled       bool            `json:"enabled"`
	HasAPIKey     bool            `json:"has_api_key"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// CreateModelRequest is the POST body for /api/v1/prompt-optimizer/models.
type CreateModelRequest struct {
	Name          string          `json:"name"`
	Provider      string          `json:"provider"`
	BaseURL       string          `json:"base_url"`
	Model         string          `json:"model"`
	APIKey        string          `json:"api_key"`
	DefaultParams json.RawMessage `json:"default_params"`
	Enabled       *bool           `json:"enabled"`
}

// UpdateModelRequest is the PUT body. An empty APIKey keeps the existing
// ciphertext untouched; a non-empty value replaces it.
type UpdateModelRequest struct {
	Name          string          `json:"name"`
	Provider      string          `json:"provider"`
	BaseURL       string          `json:"base_url"`
	Model         string          `json:"model"`
	APIKey        string          `json:"api_key"`
	DefaultParams json.RawMessage `json:"default_params"`
	Enabled       *bool           `json:"enabled"`
}

// ModelListResponse is the wire shape for GET /models.
type ModelListResponse struct {
	Status bool    `json:"status"`
	Models []Model `json:"models"`
}

// ModelResponse is the wire shape for single-model reads and writes.
type ModelResponse struct {
	Status bool   `json:"status"`
	Model  *Model `json:"model"`
}

// validProviders enumerates acceptable provider strings. Kept here so
// handler_models.go and future optimize_handler.go share the same set.
var validProviders = map[string]bool{
	"openai":            true,
	"openai_compatible": true,
	"anthropic":         true,
	"gemini":            true,
}

// validTemplateKinds enumerates kind values accepted by the templates
// table, matching the CHECK constraint in the migration.
var validTemplateKinds = map[string]bool{
	"optimize":       true,
	"iterate":        true,
	"test":           true,
	"analyze":        true,
	"user_optimize":  true,
	"image_optimize": true,
}

// validHistoryKinds enumerates kind values accepted by history rows
// (subset of template kinds; user_optimize / image_optimize are template
// categories only).
var validHistoryKinds = map[string]bool{
	"optimize": true,
	"iterate":  true,
	"test":     true,
	"analyze":  true,
}

// Template is a prompt template row. user_id NULL marks built-in seeds.
type Template struct {
	ID          string          `json:"id"`
	UserID      *string         `json:"user_id,omitempty"`
	Kind        string          `json:"kind"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Content     string          `json:"content"`
	Language    string          `json:"language"`
	Variables   json.RawMessage `json:"variables"`
	BuiltIn     bool            `json:"built_in"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type CreateTemplateRequest struct {
	Kind        string          `json:"kind"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Content     string          `json:"content"`
	Language    string          `json:"language"`
	Variables   json.RawMessage `json:"variables"`
}

type UpdateTemplateRequest struct {
	Kind        string          `json:"kind"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Content     string          `json:"content"`
	Language    string          `json:"language"`
	Variables   json.RawMessage `json:"variables"`
}

type TemplateListResponse struct {
	Status    bool       `json:"status"`
	Templates []Template `json:"templates"`
}

type TemplateResponse struct {
	Status   bool      `json:"status"`
	Template *Template `json:"template"`
}

// History is one row of prompt_optimizer_history.
type History struct {
	ID              string          `json:"id"`
	UserID          string          `json:"user_id"`
	Kind            string          `json:"kind"`
	OriginalPrompt  string          `json:"original_prompt,omitempty"`
	OptimizedPrompt string          `json:"optimized_prompt,omitempty"`
	TestInput       string          `json:"test_input,omitempty"`
	TestOutput      string          `json:"test_output,omitempty"`
	TemplateID      *string         `json:"template_id,omitempty"`
	ModelID         *string         `json:"model_id,omitempty"`
	Params          json.RawMessage `json:"params"`
	CreatedAt       time.Time       `json:"created_at"`
}

type HistoryListResponse struct {
	Status   bool      `json:"status"`
	History  []History `json:"history"`
	Total    int64     `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
}

// Favorite is a pin over a history row or template.
type Favorite struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	RefKind   string    `json:"ref_kind"`
	RefID     string    `json:"ref_id"`
	Note      string    `json:"note,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateFavoriteRequest struct {
	RefKind string `json:"ref_kind"`
	RefID   string `json:"ref_id"`
	Note    string `json:"note"`
}

type FavoriteListResponse struct {
	Status    bool       `json:"status"`
	Favorites []Favorite `json:"favorites"`
}

// Variable is a per-user named value substituted into templates at run
// time via the {{name}} placeholder syntax.
type Variable struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UpsertVariableRequest struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type VariableListResponse struct {
	Status    bool       `json:"status"`
	Variables []Variable `json:"variables"`
}

// OptimizeRequest is the body for the optimize/iterate/test/analyze
// endpoints. Kind picks which operation to run; not every field is used
// by every kind (iterate uses OptimizedPrompt + Feedback; test uses
// OptimizedPrompt + TestInput; analyze uses OriginalPrompt).
type OptimizeRequest struct {
	Kind            string          `json:"kind"`
	ModelID         string          `json:"model_id"`
	TemplateID      string          `json:"template_id"`
	OriginalPrompt  string          `json:"original_prompt"`
	OptimizedPrompt string          `json:"optimized_prompt"`
	Feedback        string          `json:"feedback"`
	TestInput       string          `json:"test_input"`
	Variables       json.RawMessage `json:"variables"`
	Stream          bool            `json:"stream"`
}
