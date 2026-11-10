package productdrawings

import (
	"context"
	"fmt"
	"os"
	"strings"

	sharedllm "github.com/chendingplano/shared/go/api/llm"
)

type sharedImageProvider struct {
	client *sharedllm.ImageClient
	style  sharedllm.ImageStyle
}
type selectionProvider struct{ qwen, openAI ImageProvider }

func (p selectionProvider) Generate(ctx context.Context, model, prompt string) ([]byte, error) {
	if strings.EqualFold(model, "Qwen") {
		return p.qwen.Generate(ctx, qwenModelName(), prompt)
	}
	return p.openAI.Generate(ctx, openAIModelName(), prompt)
}

const qwenSelectionModel = "Qwen"
const openAISelectionModel = "OpenAI"

func qwenModelName() string {
	return envOr("QWEN_IMAGE_GEN_MODEL", strings.TrimSpace(os.Getenv("IMAGE_GEN_MODEL")))
}
func openAIModelName() string { return strings.TrimSpace(os.Getenv("OPENAI_IMAGE_GEN_MODEL")) }

func newOpenAIProvider(baseURL, apiKey string) ImageProvider {
	style := sharedllm.ImageStyleOpenAICompatible
	if strings.Contains(strings.ToLower(baseURL), "dashscope") || strings.EqualFold(strings.TrimSpace(os.Getenv("IMAGE_GEN_STYLE")), "dashscope") {
		style = sharedllm.ImageStyleDashScope
	}
	return &sharedImageProvider{client: sharedllm.NewImageClient(baseURL, apiKey, nil), style: style}
}

func (p *sharedImageProvider) Generate(ctx context.Context, model, prompt string) ([]byte, error) {
	if p == nil || p.client == nil {
		return nil, fmt.Errorf("image generation provider is not configured")
	}
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("image generation model is not configured (set QWEN_IMAGE_GEN_MODEL, IMAGE_GEN_MODEL, or OPENAI_IMAGE_GEN_MODEL)")
	}
	response, err := p.client.Generate(ctx, sharedllm.ImageRequest{Style: p.style, Model: model, Prompt: prompt})
	return response.Data, err
}
