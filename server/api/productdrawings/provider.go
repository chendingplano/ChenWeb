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
	return envOr("QWEN_IMAGE_GEN_MODEL", envOr("IMAGE_GEN_MODEL", "wan2.2-t2i-flash"))
}
func openAIModelName() string { return envOr("OPENAI_IMAGE_GEN_MODEL", "gpt-image-2.5-sunburst") }

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
	response, err := p.client.Generate(ctx, sharedllm.ImageRequest{Style: p.style, Model: model, Prompt: prompt})
	return response.Data, err
}
