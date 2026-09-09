package productdrawings

import "context"

type ProductDrawing struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Prompt      string `json:"prompt"`
	Keywords    string `json:"keywords"`
	Notes       string `json:"notes"`
	Filename    string `json:"filename"`
	Model       string `json:"model"`
	ModelName   string `json:"model_name"`
	ImageURL    string `json:"image_url"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type ProductDrawingPage struct {
	Drawings []ProductDrawing `json:"drawings"`
	Total    int              `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
}

type PendingResponse struct {
	Token       string `json:"token"`
	ImageURL    string `json:"image_url"`
	Prompt      string `json:"prompt"`
	Model       string `json:"model"`
	ModelName   string `json:"model_name"`
	ExpiresAt   string `json:"expires_at"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Keywords    string `json:"keywords"`
	Notes       string `json:"notes"`
}

type KeepResponse struct {
	Status   bool   `json:"status"`
	Filename string `json:"filename"`
	Path     string `json:"path"`
}

const (
	promptFileName = "prompt-ventilator-exploded-view-v1.md"
	defaultModel   = "openai-image-2.5"
)

type Request struct {
	Subject     string `json:"subject"`
	Model       string `json:"model"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Prompt      string `json:"prompt"`
	Keywords    string `json:"keywords"`
	Notes       string `json:"notes"`
}

type ImageProvider interface {
	Generate(ctx context.Context, model, prompt string) ([]byte, error)
}

type Config struct {
	OutputDir   string
	PromptDir   string
	Model       string
	BaseURL     string
	APIKey      string
	Provider    ImageProvider
	PendingDir  string
	QwenModel   string
	OpenAIModel string
}
